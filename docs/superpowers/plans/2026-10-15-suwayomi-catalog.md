# Suwayomi Catalogs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Nightly catalogs for every English Tachiyomi source, crawled through a Suwayomi-Server the GitHub job runs, published gzipped on `catalog-data`, searched offline; hits from sites the downloader can't download from say so.

**Architecture:** `catalog/suwayomi.go` is a small REST client plus the walk/union/merge; `catalog/sources.go` holds the published `sources.json` (every crawled source) and the store's union of the site table and the sources; `index.go` learns gzipped files and the index's `extra` files. `cmd/catalog.go` gains `crawl-suwayomi` and `index`. A sharded workflow runs the server and the crawl, a publish job merges and pushes. The app's picker, hits and status learn "search only".

**Tech Stack:** Go 1.26 (`net/http` only inside the Suwayomi client, through the project's `http` package where it fits; `compress/gzip`), Suwayomi-Server v2.4.2366 (Java 21), GitHub Actions matrix, React/TypeScript.

**Spec:** `docs/superpowers/specs/2026-10-15-suwayomi-catalog-design.md`

## Global Constraints

- Every CLI feature has its app counterpart (`AGENTS.md`).
- Catalog files on disk stay plain JSON, 0600, atomic; published files are `.json.gz`.
- A crawl never writes a file smaller than the one it replaces (union merge).
- The site key is the host of the source's `baseUrl` without `www.`; it must pass `validEntry` (bare domain) or the source is skipped with a `FAIL`.
- The Suwayomi store is added through GraphQL with `index.pb`; `server.conf` repos are not relied on.
- No new dependency in either `go.mod`.

## Review Focus

1. A source whose `baseUrl` is a bare host with a path (`https://x.test/en`) must give entries under `x.test` with the path kept in the URL join. → Task 1 `TestSuwayomiBaseURLWithPath`.
2. Two sources on the same host (mirrors, or a hand-written crawler's file plus a Suwayomi source) must end as one file with the union, never one overwriting the other. → Task 1 `TestSuwayomiMergesSameHost`.
3. A published `.json.gz` whose gunzipped content isn't the site's catalog must be rejected like a bad hash. → Task 3 `TestUpdateRejectsGzipOfAnotherSite`.
4. A `--shard 3/4` run must take exactly the sources with index ≡ 3 (mod 4) in id order, so four shards cover every source once. → Task 1 `TestSuwayomiShard`.
5. `--timeout` elapsed must still write the sources it finished and exit 2, never hang on Suwayomi. → Task 1 `TestSuwayomiTimeoutStopsNewSources`.

---

## File map

| File | Responsibility |
|---|---|
| `catalog/suwayomi.go` | client, `SuwayomiOptions`, `CrawlSuwayomi`, walks, union, merge, `SourceRow` |
| `catalog/suwayomi_test.go`, `catalog/testdata/suwayomi/*.json` | fake server, tests |
| `catalog/entry.go` | `SourceInfo` on `SiteCatalog` |
| `catalog/sources.go` | `SourceRow`, `MergeSources(dir)`, `Store.Sources()`, status union |
| `catalog/index.go` | `.json.gz` on publish and update, `Extra` files, `Gzip(dir)` |
| `cmd/catalog.go` | `crawl-suwayomi`, `index`, status columns |
| `cmd/search.go` | `DL` column, `downloadable` in JSON |
| `.github/workflows/catalog-suwayomi.yaml`, `catalog.yaml` | jobs |
| `tools/suwayomi/run.sh`, `stop.sh`, `makefile` | local server, smoke |
| `gui/catalog.go`, `gui/frontend/src/{types,api,mock,search,SearchPanel}.*` | search-only |
| `README.md`, `AGENTS.md`, `.agents/skills/gui/SKILL.md` | docs |

---

### Task 1: Suwayomi client and crawl

**Files:** Create `catalog/suwayomi.go`, `catalog/suwayomi_test.go`; modify `catalog/entry.go`.

**Interfaces (produces):**
```go
// entry.go
type SourceInfo struct { ID, Name, Lang string; NSFW bool; Pages int; Complete bool }   // json: id,name,lang,nsfw,pages,complete
type SiteCatalog struct { …; Source *SourceInfo `json:"source,omitempty"` }

// suwayomi.go
type SuwayomiOptions struct {
    Server   string        // http://127.0.0.1:4567
    Langs    []string      // "en" → en + all
    Out      string        // dir of the catalog files
    MaxPages int           // per listing, default 300
    Parallel int           // default 4
    Shard, Shards int      // 0,0 = everything
    Sources  []string      // names or ids to keep, empty = all
    Install  bool
    Timeout  time.Duration // 0 = none
    Pause    time.Duration // between pages, default 200ms, 0 in tests
}
type SourceRow struct { Site, Name, Lang string; NSFW bool; SourceID string; Entries int; Complete, Failed bool; Downloadable bool }
type SuwayomiReport struct { Sources []SourceRow; Written []string; Failed int }
func CrawlSuwayomi(ctx context.Context, w io.Writer, o SuwayomiOptions) (*SuwayomiReport, error)   // error only for a bad option or an unreachable server
```
Client calls (all GET, JSON): `/api/v1/extension/list` → `[]{pkgName, lang, installed}`; `/api/v1/extension/install/{pkg}` (120 s timeout); `/api/v1/source/list` → `[]{id, name, lang, baseUrl, supportsLatest, isNsfw}`; `/api/v1/source/{id}/popular/{n}`, `/latest/{n}`, `/search?searchTerm=&pageNum={n}` → `{mangaList:[{url,title}], hasNextPage}`. Walk rules as the spec: stop on `hasNextPage=false`, a page adding no new URL, `MaxPages`, or 3 errors in a row; `Pause` between pages; `ctx` checked each page. Site = host of `baseUrl` sans `www.`; skipped (`FAIL`) when it fails `siteNameRe`. Merge: `NewStore(o.Out)`, `Load`, existing entries ∪ new (existing first, `dedupe`), `sort.SliceStable` by `Fold(title)`, `Home` kept, `Language` = source lang unless the file has one, `Source` set, `Write`. The report's `Written` lists sites written; `w` gets one line per source like `runCrawl`. `Timeout`: a `time.Now()` check before starting each source. Rows go to `sources-<shard>.json` (`sources.json` without shards) in `Out`, written at the end even when sources failed.

- [ ] **Step 1: Failing tests** with a fake Suwayomi (`httptest.Server` whose handler serves canned pages from a map keyed by path, counting calls): `TestSuwayomiUnionOfListings` (popular pages 1–2 then `hasNextPage:false`; latest one page that repeats on page 2; search empty → union of 50 titles, latest stop after the repeat), `TestSuwayomiBaseURLWithPath`, `TestSuwayomiMergesSameHost` (an existing `x.test.json` with 3 entries of which 1 overlaps → merged file has 3 + new, existing first, `Source` set), `TestSuwayomiShard` (4 sources, `--shard 3/4` keeps the 4th), `TestSuwayomiErrorsInARowStopTheWalk` (page 2 returns 500 three times → the walk ends with what it has, source not failed), `TestSuwayomiEmptySourceFails` (no manga at all → `FAIL`, nothing written, `Failed` row), `TestSuwayomiInstallsMissingExtensions` (`installed:false` en extension gets `/extension/install/` called, an `all` one too, a `fr` one not), `TestSuwayomiSkipsBadHost` (`baseUrl` `https://../x`), `TestSuwayomiTimeoutStopsNewSources` (`Timeout: 1ns`, two sources → the first is attempted — timeout checked after the first start — rows say the second was skipped: `Failed:true`, log says "timeout"), `TestSuwayomiSourcesFile` (rows written, `Downloadable` true for `mangaread.org`, false for `x.test`).
- [ ] **Step 2:** run → FAIL. **Step 3:** implement. **Step 4:** run → PASS, `gofmt`, `go vet`. **Step 5:** commit `catalog: Suwayomi crawl`.

### Task 2: sources.json, status union

**Files:** Create `catalog/sources.go`, `catalog/sources_test.go`; modify `catalog/store.go` (`Load` reads `sources.json`; `Status` union; `SiteStatus` gains `Downloadable`, `NSFW`, `Source string`), `cmd/catalog.go` (`index` subcommand; status `DL`/`NSFW` columns), `cmd/search.go` (`DL` column, `downloadable` JSON), tests.

**Interfaces:** `func MergeSources(dir string) ([]SourceRow, error)` (reads `sources-*.json` and `sources.json`, keys by site, newest wins, adds table sites with no row as `{Site, Lang, Downloadable:true}`, writes `sources.json` sorted by site, removes the shard files); `func (s *Store) Sources() []SourceRow`; `Status()` rows for sources not in the table (`Downloadable:false`). `cmd`: `catalogIndexCmd` (`--out`, `--gzip`, Task 3) runs `MergeSources` then `BuildIndex`/`WriteIndex`; `runSearch` prints `DL` (`yes`/`-`) after `SITE` and `"downloadable"` in JSON, from `store.Downloadable(site)` (in the table).

- [ ] Tests: `TestMergeSourcesShards`, `TestMergeSourcesKeepsTableSites`, `TestStatusIncludesSources` (a source-only site shows `Downloadable:false`), `TestRunSearchShowsDownloadable`, `TestRunStatusDLColumn`, `TestCatalogIndexCommand`. RED → GREEN → commit `catalog: sources.json and search-only sites`.

### Task 3: gzip on publish and update

**Files:** modify `catalog/index.go` (`Gzip(dir) error` turns every `<site>.json` into `<site>.json.gz` and removes the plain file; `BuildIndex` lists `.json.gz` files with the gz hash and reads the site name from the gunzipped content; `Index.Extra []IndexEntry` for `sources.json`; `Update` fetches extras when changed and gunzips `.json.gz` after the hash check, rejecting content whose `Site` differs), `catalog/store.go` (`Load` reads `.json.gz` too, used by the crawl's merge on the checked-out branch), `cmd/catalog.go` (`index --gzip`), tests.

- [ ] Tests: `TestGzipDir`, `TestBuildIndexReadsGzip`, `TestUpdateGunzips`, `TestUpdateRejectsGzipOfAnotherSite`, `TestUpdateFetchesExtraSources`, `TestLoadReadsGzip`, `TestUpdateStillAcceptsPlainFiles`. RED → GREEN → commit `catalog: gzipped published files`.

### Task 4: workflow, local server scripts, smoke

**Files:** Create `.github/workflows/catalog-suwayomi.yaml`, `tools/suwayomi/run.sh`, `tools/suwayomi/stop.sh`, `tools/suwayomi/README.md`; modify `.github/workflows/catalog.yaml` (commit step runs `catalog index --gzip`), `makefile` (`catalog/smoke-suwayomi`).

- [ ] Write them as the spec says; run `make catalog/smoke-suwayomi` locally (server from `run.sh`, MangaPill + Weeb Central, `--max-pages 2`) and read the output. Commit `catalog: Suwayomi nightly job and local server`.

### Task 5: app

**Files:** modify `gui/catalog.go` (`HitDTO.Downloadable`, `.NSFW`; `SiteDTO.Downloadable`, `.NSFW`; `CatalogDTO.SearchOnly`), `gui/catalog_test.go`, frontend `types.ts`, `api.ts` (no new calls), `mock.ts` (two search-only sites, `downloadable` on hits), `search.ts` (`siteOptions` → two groups, no disabled rows; `catalogSummary` with search only), `SearchPanel.tsx` (optgroups; tag; the note with *Copy link* and *Open in browser*), tests; `App.tsx` passes `openURL`.

- [ ] Tests: Go `TestHitsCarryDownloadable`; vitest `siteOptions` groups, summary text, panel renders the tag and, with a picked search-only hit, the note. RED → GREEN; `pnpm typecheck`, `pnpm build`, gui Go tests; commit `gui: search-only sites`.

### Task 6: docs

- [ ] README (Search section paragraph on Tachiyomi sources and "search only"), AGENTS.md ("Suwayomi catalogs" bullets: the store mutation, the union of listings, the merge, shards, gzip, `index`), gui SKILL (search only). Commit `docs: Suwayomi catalogs`.

### Task 7: review pass

- [ ] Fresh reviewer on the branch range; fix Critical/Important with tests; ledger the rest.
