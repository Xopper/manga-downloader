# Catalog and Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Per-site catalogs (title + series URL) crawled nightly and published in the fork, searched offline from the CLI (`search`, `catalog …`) and the desktop app (a Search mode in the source bar).

**Architecture:** A new `catalog` package in the main module holds the site table (`sites.json`, embedded), four family crawlers (themesia, madara, wordpress, single), a `Store` for the files under the user config dir, an `Update` that syncs from a published `index.json` by sha256, and an in-memory folded search. `cmd/` adds `search` and `catalog crawl|update|status`. A workflow commits crawl output to the orphan `catalog-data` branch. The GUI controller owns a `Store`, refreshes it in the background, and `app.go` exposes `Search`/`Sites`/`CatalogStatus` DTOs to a Search mode in the window.

**Tech Stack:** Go 1.26, goquery, cobra, `golang.org/x/text` (folding), the project's `http` package; Wails v2 + React/TypeScript + vitest for the GUI; GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-15-catalog-design.md`

## Global Constraints

- Go `1.26.0` (both `go.mod` files); the local toolchain downloads it automatically.
- Every feature exists in both the CLI and the desktop app (project rule in `AGENTS.md`).
- All requests go through `http.GetText`/`http.Get` (`http/request.go`): never `net/http` directly in crawlers.
- The crawler never starts a browser; browser-gated sites are `crawl: false`.
- Catalog files are written atomically (temp file + rename), mode 0600, like `gui.json`.
- Published base URL: `https://raw.githubusercontent.com/Xopper/manga-downloader/catalog-data/catalog/`, overridable by `MANGA_DOWNLOADER_CATALOG_URL`.
- Half-size guard: a crawl returning fewer than half the entries of the existing file keeps the old file (warning, exit 2 at the end).
- No new dependency in `gui/go.mod` beyond the main module.
- GUI: logic in `controller.go`, DTOs and OS bits in `app.go`; the frontend talks only through `api.ts`; `mock.ts` mirrors every new method; 16px inputs on narrow windows.

## Review Focus

1. A query of only punctuation or spaces (e.g. `"  -  "`) must return no hits and no panic, not every entry. → Task 6 test `TestSearchEmptyQuery`.
2. A site whose list page returns the same entries on every page (a site that ignores `page=N`) must stop, not loop forever. → Task 3 test `TestThemesiaGridStopsWhenNothingNew`.
3. An `index.json` that lists a site file whose download fails halfway must leave the previous file and the previous index intact. → Task 7 test `TestUpdateKeepsOldFilesOnFailure`.
4. `catalog crawl` run with an `--out` that already holds a bigger file for that site must keep it and exit 2. → Task 9 test `TestCrawlHalfSizeGuard`.
5. Relative and protocol-relative series links (`/manga/x/`, `//site/manga/x/`) must become absolute `https` URLs in entries. → Task 2 test `TestAbsURL`.

---

## File map

| File | Responsibility |
|---|---|
| `catalog/entry.go` | `Entry`, `SiteCatalog` types, JSON shape |
| `catalog/sites.go`, `catalog/sites.json` | `SiteInfo`, embedded table, `Sites()`, `SiteByHost()` |
| `catalog/crawler.go` | `Crawler` interface, registry, `fetchDoc`, `absURL`, `dedupe`, pacing |
| `catalog/crawl_themesia.go` | list-mode page, grid fallback |
| `catalog/crawl_madara.go` | `/manga/page/N/` paging |
| `catalog/crawl_wordpress.go` | generic archive paging with per-site selectors |
| `catalog/crawl_single.go` | one entry: the home page |
| `catalog/fold.go` | `Fold(s string) string` |
| `catalog/search.go` | `Hit`, `(*Store).Search` |
| `catalog/store.go` | `Store`, `Dir`, `Load`, `Write`, `Status` |
| `catalog/index.go` | `Index`, `IndexEntry`, `BuildIndex`, `Update` |
| `catalog/testdata/<family>/*.html` | fixtures |
| `cmd/search.go`, `cmd/catalog.go` | subcommands |
| `.github/workflows/catalog.yaml` | nightly job |
| `makefile` | `catalog/smoke` target |
| `gui/controller.go`, `gui/app.go`, `gui/catalog.go` | store, refresh, bound methods, DTOs |
| `gui/frontend/src/{types,api,mock}.ts`, `SearchPanel.tsx`, `App.tsx`, `SourceBar.tsx` | Search mode |
| `README.md`, `AGENTS.md`, `.agents/skills/gui/SKILL.md` | docs |

---

### Task 1: Entry types and the site table

**Files:**
- Create: `catalog/entry.go`, `catalog/sites.go`, `catalog/sites.json`, `catalog/sites_test.go`

**Interfaces:**
- Produces: `type Entry struct{Title, URL string; AltNames []string}`; `type SiteCatalog struct{Site, Home, Language string; Crawled time.Time; Entries []Entry}`; `type SiteInfo struct{Site, Home string; Aliases []string; Family, List, Lang string; Crawl bool}`; `func Sites() []SiteInfo`; `func SiteByHost(host string) (SiteInfo, bool)` (matches site or alias, `www.` stripped, case-insensitive).

- [ ] **Step 1: Write the failing tests** (`catalog/sites_test.go`)

```go
package catalog

import "testing"

func TestSitesLoadsEmbeddedTable(t *testing.T) {
	sites := Sites()
	if len(sites) < 100 {
		t.Fatalf("expected the full site table, got %d", len(sites))
	}
	seen := map[string]bool{}
	for _, s := range sites {
		if s.Site == "" || s.Home == "" || s.Family == "" {
			t.Errorf("incomplete row: %+v", s)
		}
		if seen[s.Site] {
			t.Errorf("duplicate site %s", s.Site)
		}
		seen[s.Site] = true
		if s.Crawl && s.Family != "single" && s.List == "" {
			t.Errorf("%s crawls but has no list URL", s.Site)
		}
	}
}

func TestSiteByHost(t *testing.T) {
	for _, host := range []string{"kingofshojo.com", "www.kingofshojo.com", "WWW.KingOfShojo.com"} {
		if s, ok := SiteByHost(host); !ok || s.Site != "kingofshojo.com" {
			t.Errorf("%s: got %+v %v", host, s, ok)
		}
	}
	if s, ok := SiteByHost("genzupdates.com"); !ok || s.Site != "genztoons.org" {
		t.Errorf("alias: got %+v %v", s, ok)
	}
	if _, ok := SiteByHost("nope.example"); ok {
		t.Error("unknown host matched")
	}
}
```

- [ ] **Step 2: Run** `go test ./catalog -run 'TestSites|TestSiteByHost'` → FAIL (package missing).

- [ ] **Step 3: Implement**

`catalog/entry.go`:
```go
// Package catalog lists every series a site has, so a title can be found
// without knowing its URL, and searches those lists offline.
package catalog

import "time"

// Entry is one series a site lists
type Entry struct {
	Title string `json:"title"`
	// URL is the series page, what the downloader's Load/IdentifySite takes
	URL string `json:"url"`
	// AltNames are the other titles the site lists (romaji, native, …)
	AltNames []string `json:"alt,omitempty"`
}

// SiteCatalog is one site's list, the content of catalog/<site>.json
type SiteCatalog struct {
	Site     string    `json:"site"`
	Home     string    `json:"home"`
	Language string    `json:"lang"`
	Crawled  time.Time `json:"crawled"`
	Entries  []Entry   `json:"entries"`
}
```

`catalog/sites.go`: `//go:embed sites.json`, parse once in `sync.Once`, `Sites()` returns a copy, `SiteByHost` lowercases, strips `www.`, checks `Site` then `Aliases`.

`catalog/sites.json`: one row per README site, from `tools/site-check/results.json`. Rows: `site`, `home` (the final host from the report, e.g. `genztoons.org` with `aliases: ["genzupdates.com"]`; `violetmanga.com` with alias `violetscans.org`; `qimanga.com` alias `aurorascans.com`; `guya.cubari.moe` alias `guya.moe`), `family`, `list`, `lang`, `crawl`. Families and list URLs:
  - `themesia` (13 sites from the report): `list` = `<home>/manga/list-mode/`, `crawl: true`.
  - `madara` (11): `list` = `<home>/manga/page/{page}/`, `crawl: true`.
  - `wordpress` (hivetoons.org, rawkuma.net, silentquill.net, tvymanga3.com, utoon.us, animeallstar30.com): `list` = `<home>/manga/page/{page}/`, `crawl: true`; the implementer checks each with curl while writing the fixture and sets `crawl: false` with a `"note"` when the archive 404s.
  - `single` (blackcloveronline.com, chainsawman-chapter.online, chainsawmannmanga.com, dragonballmangaa.com, heavenly-delusion.com, jojolandsmanga.com, jujutsukaisenmanga.pro, the-apothecary-diaries.com, tcbonepiecechapters.com is NOT single — it lists several series, keep it `custom`): `crawl: true`, `list` empty.
  - everything else: `family` = the report's platform (`custom`, `nextjs`, `astro`, `nuxt`, `foolslide`, `blogger`, `api`), `crawl: false`. Browser-gated (`*`) sites are `crawl: false` whatever their family. `api`: mangadex.org, mangaplus.shueisha.co.jp, webtoons.com, mangalib.me.
  - `lang`: from the README label (Arabic → `ar`, Spanish → `es`, French → `fr`, Indonesian → `id`, Italian → `it`, Turkish sites `tr`, `twmanga.com` → `zh`, `rawkuma.net` → `ja`), else `en`.

- [ ] **Step 4: Run** the two tests → PASS. `gofmt -l catalog` → empty.

- [ ] **Step 5: Commit** `git add catalog && git commit -m "catalog: entry types and the embedded site table"`

---

### Task 2: Crawler contract, fetch helper, URL and dedupe helpers

**Files:**
- Create: `catalog/crawler.go`, `catalog/crawler_test.go`

**Interfaces:**
- Consumes: `http.GetText(http.RequestParams{URL, DisableRetry: true})` → `(string, error)`; `goquery.NewDocumentFromReader`.
- Produces:
```go
type Crawler interface {
	Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error)
}
var crawlers = map[string]Crawler{}            // family → crawler, filled by init() in each crawl_*.go
func CrawlerFor(site SiteInfo) (Crawler, bool)
// fetcher is how crawlers get a page; tests replace it with a map-backed one
type fetcher func(ctx context.Context, url string) (*goquery.Document, error)
var fetch fetcher = httpFetch
var pagePause = 300 * time.Millisecond        // between list pages, 0 in tests
func absURL(base, href string) string          // "" when href is empty or unparsable
func dedupe(entries []Entry) []Entry           // by URL path+query, first wins, order kept
func pageURL(list string, page int) string     // replaces {page}; a list without {page} is returned as is
```

- [ ] **Step 1: Failing tests**

```go
func TestAbsURL(t *testing.T) {
	cases := map[string]string{
		"/manga/x/":                 "https://a.test/manga/x/",
		"//a.test/manga/y/":         "https://a.test/manga/y/",
		"https://b.test/z":          "https://b.test/z",
		"manga/w":                   "https://a.test/manga/w",
		"":                          "",
	}
	for in, want := range cases {
		if got := absURL("https://a.test/manga/list-mode/", in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestDedupeByPath(t *testing.T) {
	in := []Entry{{Title: "A", URL: "https://a.test/manga/a/"}, {Title: "A again", URL: "https://www.a.test/manga/a/"}, {Title: "B", URL: "https://a.test/manga/b/"}}
	got := dedupe(in)
	if len(got) != 2 || got[0].Title != "A" || got[1].Title != "B" {
		t.Fatalf("got %+v", got)
	}
}

func TestPageURL(t *testing.T) {
	if got := pageURL("https://a.test/manga/page/{page}/", 3); got != "https://a.test/manga/page/3/" {
		t.Error(got)
	}
	if got := pageURL("https://a.test/manga/list-mode/", 3); got != "https://a.test/manga/list-mode/" {
		t.Error(got)
	}
}
```

- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement.** `httpFetch` calls `http.GetText` with `DisableRetry: true` and a `Referer` of the site home, wraps the body in `goquery.NewDocumentFromReader(strings.NewReader(text))`, checks `ctx.Err()` first. `dedupe` keys on `strings.TrimSuffix(u.Path, "/") + "?" + u.RawQuery` after `absURL`. A test helper `fakeFetch(pages map[string]string) fetcher` (in `crawler_test.go`) serves fixtures by URL and returns `errNotFound` (a `*http.StatusError{Code: 404}`) for unknown ones; `withFakeFetch(t, pages)` swaps `fetch` and `pagePause = 0` for the test via `t.Cleanup`.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `catalog: crawler contract and helpers`

---

### Task 3: Themesia crawler

**Files:**
- Create: `catalog/crawl_themesia.go`, `catalog/crawl_themesia_test.go`, `catalog/testdata/themesia/list-mode.html`, `catalog/testdata/themesia/grid-1.html`, `grid-2.html`, `grid-empty.html`

**Interfaces:** registers `crawlers["themesia"]`.

Fixtures: `list-mode.html` is a trimmed copy of `https://kingofshojo.com/manga/list-mode/` (keep `<div class="soralist">` with three `.blix` groups, ~10 `a.series` rows); `grid-N.html` trimmed from `https://en-thunderscans.com/manga/?page=N` keeping `.listupd .bs .bsx a[title]` rows; `grid-empty.html` the same page with no `.bsx`.

- [ ] **Step 1: Failing tests**

```go
func TestThemesiaListMode(t *testing.T) {
	withFakeFetch(t, map[string]string{
		"https://a.test/manga/list-mode/": read(t, "themesia/list-mode.html"),
	})
	site := SiteInfo{Site: "a.test", Home: "https://a.test", List: "https://a.test/manga/list-mode/"}
	got, err := crawlers["themesia"].Crawl(context.Background(), site, 0)
	if err != nil { t.Fatal(err) }
	if len(got) != 10 { t.Fatalf("got %d entries: %+v", len(got), got) }
	if got[0].Title != "??? Rank no Ura Soubi Musou" || got[0].URL != "https://kingofshojo.com/manga/rank-no-ura-soubi-musou/" {
		t.Errorf("first: %+v", got[0])
	}
}

func TestThemesiaGridFallback(t *testing.T) {
	withFakeFetch(t, map[string]string{
		// list-mode 404s, the grid pages serve, page 3 is empty
		"https://a.test/manga/?page=1": read(t, "themesia/grid-1.html"),
		"https://a.test/manga/?page=2": read(t, "themesia/grid-2.html"),
		"https://a.test/manga/?page=3": read(t, "themesia/grid-empty.html"),
	})
	site := SiteInfo{Site: "a.test", Home: "https://a.test", List: "https://a.test/manga/list-mode/"}
	got, err := crawlers["themesia"].Crawl(context.Background(), site, 0)
	if err != nil { t.Fatal(err) }
	if len(got) != 40 { t.Fatalf("got %d", len(got)) }
}

func TestThemesiaGridStopsWhenNothingNew(t *testing.T) {
	same := read(t, "themesia/grid-1.html")
	withFakeFetch(t, map[string]string{
		"https://a.test/manga/?page=1": same, "https://a.test/manga/?page=2": same, "https://a.test/manga/?page=3": same,
	})
	site := SiteInfo{Site: "a.test", Home: "https://a.test", List: "https://a.test/manga/list-mode/"}
	got, err := crawlers["themesia"].Crawl(context.Background(), site, 0)
	if err != nil { t.Fatal(err) }
	if len(got) != 20 { t.Fatalf("a site ignoring page= must stop after one repeat, got %d", len(got)) }
}

func TestThemesiaLimit(t *testing.T) { /* limit 1 on the grid → 20 entries, page 2 never fetched (fake fetch records calls) */ }
```

- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement.** `Crawl`: fetch `site.List`; if the doc has `.soralist a.series` rows → entries (title = text trimmed, URL = `absURL`), done. Otherwise (404 or no rows) walk `<home>/manga/?page=N` from 1: rows `.listupd .bsx a`, title = `title` attr or `.tt` text, stop when a page adds no new URL (compare count before/after `dedupe`) or has no rows, or `limit` pages reached; `pagePause` between pages; `ctx` checked each page. Return `dedupe(entries)`; an error on page 1 is returned, an error on a later page ends the walk with what was collected and that error.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `catalog: themesia crawler`

---

### Task 4: Madara crawler

**Files:** `catalog/crawl_madara.go`, `_test.go`, `testdata/madara/page-1.html`, `page-2.html` (trimmed from `https://www.mangaread.org/manga/page/N/`, rows `.page-item-detail .post-title h3 a` or `.post-title a`), and `page-404.html`.

**Interfaces:** registers `crawlers["madara"]`.

- [ ] **Step 1: Tests:** `TestMadaraPagesUntil404` (two pages then a 404 from the fake fetch → 2×N entries, titles trimmed, no `Read` suffix), `TestMadaraStopsWhenNothingNew`, `TestMadaraLimit`.
- [ ] **Step 2–4:** FAIL → implement (`pageURL(site.List, n)`, rows `.post-title a`, stop on `*http.StatusError` 404, empty page, or no new URLs) → PASS.
- [ ] **Step 5: Commit** `catalog: madara crawler`

---

### Task 5: WordPress generic and single crawlers

**Files:** `catalog/crawl_wordpress.go`, `catalog/crawl_single.go`, tests, `testdata/wordpress/page-1.html`, `page-2.html` (from rawkuma.net or hivetoons.org, whichever answers `/manga/page/2/`).

**Interfaces:** registers `crawlers["wordpress"]`, `crawlers["single"]`.

- [ ] **Step 1: Tests:** `TestWordPressArchive` (rows found by the first selector that matches among `article h2 a`, `.post-title a`, `.bsx a`, `h3.entry-title a`; stops as madara), `TestSingleEntryIsHome` (one entry, `Title` = the page `<title>` up to the first ` - `, ` | ` or ` – `, `URL` = `site.Home`; a fetch error still returns the entry with `Title` = the site name).
- [ ] **Step 2–4:** FAIL → implement → PASS.
- [ ] **Step 5: Commit** `catalog: wordpress and single-series crawlers`

---

### Task 6: Folding and search

**Files:** `catalog/fold.go`, `catalog/search.go`, `catalog/search_test.go`; `go get golang.org/x/text@latest` (direct dependency).

**Interfaces:**
```go
func Fold(s string) string                  // lowercase, NFKD, marks stripped, non-letter/digit → space, spaces collapsed
type Hit struct{ Site string; Entry Entry; Score int }
func (s *Store) Search(query string, sites []string, limit int) []Hit   // limit <= 0 → 50
```
Scores: exact folded title 400, title prefix 300, every word a word-prefix 200, every word a substring 100; +0 for alt-name matches but the same tiers minus 10. Ties: shorter title, then site.

- [ ] **Step 1: Tests:** `TestFold` (`"Café Ōtsuki!!"` → `"cafe otsuki"`, `"ｶﾌｪ"` → `"カフェ"` folded width), `TestSearchOrdering` (store with two sites in a temp dir, exact beats prefix beats word-prefix beats substring), `TestSearchSiteFilter`, `TestSearchLimit`, `TestSearchEmptyQuery` (`"  -  "` → nil, `""` → nil), `TestSearchAltNames`.
- [ ] **Step 2–4:** FAIL → implement (`Store` keeps `[]indexed{site string; e Entry; title string; alts []string}` built in `Load`) → PASS.
- [ ] **Step 5: Commit** `catalog: folding and search`

---

### Task 7: Store, index, update

**Files:** `catalog/store.go`, `catalog/index.go`, `catalog/store_test.go`, `catalog/index_test.go`

**Interfaces:**
```go
type Store struct { Dir string; mu sync.RWMutex; catalogs map[string]*SiteCatalog; entries []indexed }
func DefaultDir() (string, error)                          // os.UserConfigDir()/manga-downloader/catalog
func NewStore(dir string) *Store
func (s *Store) Load() []error                              // every *.json except index.json; bad files → warnings
func (s *Store) Write(c *SiteCatalog) error                 // atomic, 0600; updates memory too
func (s *Store) Catalog(site string) (*SiteCatalog, bool)
type SiteStatus struct { Site, Label, Language string; HasCatalog bool; Entries int; Crawled time.Time; FileAge time.Duration }
func (s *Store) Status() []SiteStatus                       // every site in Sites(), in table order
type IndexEntry struct { Site string `json:"site"`; File string `json:"file"`; Entries int `json:"entries"`; Crawled time.Time `json:"crawled"`; SHA256 string `json:"sha256"` }
type Index struct { Generated time.Time `json:"generated"`; Sites []IndexEntry `json:"sites"` }
func BuildIndex(dir string) (*Index, error)                 // hashes every <site>.json in dir
func WriteIndex(dir string, idx *Index) error
func ReadIndex(dir string) (*Index, error)                  // os.ErrNotExist when none
func BaseURL() string                                        // env override or the constant
func (s *Store) Update(ctx context.Context, baseURL string) (changed []string, err error)
```
`Update`: GET `baseURL + "index.json"` (through `http.GetText`), for each site whose sha differs from the local index (or missing locally) GET `baseURL + file`, verify sha256, `Write`; after all, write the new index with the entries that succeeded (keeping local entries for failed ones); return the first error with the changed list. A `ctx` cancel stops between files.

- [ ] **Step 1: Tests:** `TestStoreWriteLoadRoundTrip`, `TestLoadSkipsBadFile` (one warning, others loaded), `TestStatusListsEverySite`, `TestBuildIndexHashes`, `TestUpdateDownloadsOnlyChanged` (an `httptest.Server`; counts requests), `TestUpdateKeepsOldFilesOnFailure` (server returns 500 for one file: old file and old index entry remain, other site updated, err non-nil, changed lists the good one), `TestUpdateRejectsBadHash`, `TestBaseURLEnvOverride`.
- [ ] **Step 2–4:** FAIL → implement → PASS.
- [ ] **Step 5: Commit** `catalog: store, index and update`

---

### Task 8: `search` command

**Files:** `cmd/search.go`, `cmd/search_test.go`

**Interfaces:** `searchCmd` (`Use: "search <query>"`, flags `--site`, `--limit` (50), `--json`); helper `openStore() (*catalog.Store, error)` shared with Task 9 (uses `catalog.DefaultDir()`, `Load`, prints warnings in yellow to stderr); `runSearch(w io.Writer, store *catalog.Store, query, site string, limit int, asJSON bool) (exitCode int)`.

Output: aligned columns `SITE  TITLE  URL` with `text/tabwriter`, title truncated to 60 runes with `truncateString` (`cmd/progress.go`); JSON: `[{"site","title","url"}]`. No hits → `No results` in yellow, exit 1. No local index: run `Update` first (spinner-less, one line `Downloading the catalog…`), and if that fails print `No catalog yet: run manga-downloader catalog update` and exit 1.

- [ ] **Step 1: Tests:** `TestRunSearchTable` (temp store with two sites; checks columns and order), `TestRunSearchJSON`, `TestRunSearchNoResultsExit1`, `TestRunSearchSiteFilter`.
- [ ] **Step 2–4:** FAIL → implement, `rootCmd.AddCommand(searchCmd)` in `init()` → PASS.
- [ ] **Step 5: Commit** `cmd: search`

---

### Task 9: `catalog` commands

**Files:** `cmd/catalog.go`, `cmd/catalog_test.go`

**Interfaces:** `catalogCmd` with subcommands `update`, `status` (`--json`), `crawl` (`--site`, `--limit`, `--out` default `./catalog`). `runCrawl(ctx, w io.Writer, out string, site string, limit int) (exit int)`: for each `Sites()` row with `Crawl` (or the one `--site`), `CrawlerFor`, crawl, apply the half-size guard against an existing `<out>/<site>.json`, write, print `site  N entries (Δ)`; two sites at a time (`errgroup`-free: a `sync.WaitGroup` + semaphore of 2); exit 2 if any site failed or was guarded, 0 otherwise; writes `index.json` at the end with `BuildIndex`/`WriteIndex`.

- [ ] **Step 1: Tests:** `TestCrawlHalfSizeGuard` (fake crawler registered under family `fake` for a `SiteInfo` passed through a test-only `sitesForCrawl` var; existing file has 100 entries, crawl returns 40 → file untouched, exit 2), `TestCrawlWritesIndex`, `TestCrawlUnknownSiteFails`, `TestStatusJSON`.
- [ ] **Step 2–4:** FAIL → implement → PASS.
- [ ] **Step 5: Commit** `cmd: catalog crawl, update, status`

---

### Task 10: Nightly workflow and smoke target

**Files:** `.github/workflows/catalog.yaml`, `makefile` (`catalog/smoke`), `tools/site-check/README.md` (one paragraph: what the probe is for).

- [ ] **Step 1:** Write `catalog.yaml`:
```yaml
name: Catalog
on:
  schedule: [{ cron: "17 3 * * *" }]
  workflow_dispatch:
permissions: { contents: write }
concurrency: catalog-data
jobs:
  crawl:
    runs-on: ubuntu-latest
    timeout-minutes: 120
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.26" }
      - name: Build
        run: go build -o bin/manga-downloader .
      - name: Check out the data branch
        run: |
          git fetch origin catalog-data:catalog-data 2>/dev/null || true
          if git show-ref --quiet refs/heads/catalog-data; then
            git worktree add data catalog-data
          else
            git worktree add --detach data && (cd data && git checkout --orphan catalog-data && git rm -rfq . && mkdir -p catalog)
          fi
      - name: Crawl
        id: crawl
        run: |
          set +e
          bin/manga-downloader catalog crawl --out data/catalog | tee crawl.log
          echo "exit=$?" >> "$GITHUB_OUTPUT"
      - name: Commit
        run: |
          cd data
          git config user.name github-actions[bot]
          git config user.email github-actions[bot]@users.noreply.github.com
          git add catalog
          git diff --cached --quiet && echo "no change" && exit 0
          total=$(python3 -c 'import json;print(sum(s["entries"] for s in json.load(open("catalog/index.json"))["sites"]))')
          git commit -m "Update catalog: $total entries"
          git push origin HEAD:catalog-data
      - name: Fail when the crawl failed
        if: steps.crawl.outputs.exit != '0'
        run: grep -E 'FAIL|kept' crawl.log; exit 1
```
  The crawl exit 2 (a site failed or was guarded) still commits what worked and then marks the job red, so a broken site is noticed without losing the others.
- [ ] **Step 2:** makefile: `catalog/smoke:` runs `go run . catalog crawl --limit 1 --out /tmp/catalog-smoke --site kingofshojo.com`, then `--site mangaread.org`, then `--site rawkuma.net`, then `--site heavenly-delusion.com`, and `test -s /tmp/catalog-smoke/index.json`.
- [ ] **Step 3:** Run `make catalog/smoke` locally (live network) → the four files exist and have > 0 entries (single: exactly 1).
- [ ] **Step 4: Commit** `catalog: nightly workflow and smoke target`

---

### Task 11: GUI backend — controller and bound methods

**Files:**
- Create: `gui/catalog.go` (DTOs + App methods), `gui/catalog_test.go`
- Modify: `gui/controller.go` (store field, `catalogRefresh`, `Search`, `Sites`, `CatalogStatus`), `gui/app.go` (startup/shutdown hooks, `State.catalog` summary), `gui/controller_test.go`

**Interfaces:**
```go
// controller.go
type Controller struct { …; catalog *catalog.Store; catalogGen uint64; catalogUpdating bool; catalogErr string; catalogAt time.Time }
func (c *Controller) SetCatalog(s *catalog.Store)                       // tests pass a temp-dir store
func (c *Controller) RefreshCatalog(ctx context.Context, force bool)    // no-op when index younger than 24h unless force; runs Update; bumps catalogGen; notify()
func (c *Controller) Search(query, site string) []catalog.Hit
func (c *Controller) Sites() []catalog.SiteStatus
func (c *Controller) CatalogInfo() (sites, entries int, updatedAt time.Time, updating bool, errMsg string)

// catalog.go (bound)
type HitDTO struct { Site, Title, URL, Language string `json:"…"` }
type SiteDTO struct { Site, Label, Language string; HasCatalog bool; Entries int }
type CatalogDTO struct { Sites, Entries int; UpdatedAt string; Updating bool; Error string; Generation uint64 }
func (a *App) Search(query, site string) []HitDTO
func (a *App) Sites() []SiteDTO
func (a *App) CatalogStatus() CatalogDTO
func (a *App) UpdateCatalog() error         // force refresh in the background
```
`State` gets `Catalog CatalogDTO` (`json:"catalog"`) so the window learns of a finished refresh through the existing `state` push.

- [ ] **Step 1: Tests:** controller: `TestSearchUsesStore`, `TestRefreshSkipsFreshIndex` (index written now, fake base URL server counts 0 requests), `TestRefreshForceDownloads`, `TestRefreshErrorKeptInState`; app: `TestStateCarriesCatalog`.
- [ ] **Step 2–4:** FAIL → implement; `startup` loads `catalog.DefaultDir()` store, then `go c.RefreshCatalog(ctx, false)` with the shutdown-cancelled context, as the update check does → PASS with `cd gui && go test -tags desktop,production,nodynamic ./...` (frontend built first: `make build/gui/frontend`).
- [ ] **Step 5: Commit** `gui: catalog store, refresh and bound search`

---

### Task 12: GUI frontend — Search mode

**Files:**
- Modify: `gui/frontend/src/types.ts` (`Hit`, `SiteInfo`, `CatalogInfo`, `State.catalog`), `api.ts` (`search`, `sites`, `catalogStatus`, `updateCatalog`), `mock.ts` (fixed 30-entry catalog over 3 sites; `?scenario=search`), `App.tsx` (mode state, renders `SearchPanel` in place of `ChapterPanel` while in search mode), `SourceBar.tsx` (a two-way toggle *URL* / *Search* at the left of the bar; in Search mode the URL field is replaced by the search field and the site selector), `styles.css`
- Create: `SearchPanel.tsx` (results list, virtualised with the existing `virtual.ts`; row: title, site chip, language; empty states: "Type to search", "No results", "No catalog yet" with *Update now*), `SearchPanel.test.ts`, `search.ts` (debounce + the pure `resultRows(hits)` mapping), `search.test.ts`
- Regenerate bindings: `cd gui && wails generate module` (install with `go install github.com/wailsapp/wails/v2/cmd/wails@latest` if missing); commit `frontend/wailsjs/`.

- [ ] **Step 1: Tests (vitest):** `search.test.ts`: `resultRows` maps hits and sorts nothing (order is the backend's); debounce fires once after the pause. `SearchPanel.test.ts`: mock api, `?scenario=search` — typing `one` lists the mock's matches; clicking a row calls `load(url)` and the mode returns to URL; a catalog with 0 sites shows "No catalog yet" and the button calls `updateCatalog`.
- [ ] **Step 2–4:** FAIL → implement → `cd gui/frontend && pnpm test` PASS; `pnpm dev` with `?mock&scenario=search` checked by eye at 1100px and 420px width (16px input).
- [ ] **Step 5: Commit** `gui: search mode`

---

### Task 13: Docs

**Files:** `README.md` (features bullet "Search the supported sites' catalogs offline"; a "Search" section after "Supported sites" with the four commands and the app's Search mode), `AGENTS.md` ("Catalog" section: the package, the orphan branch, the half-size guard, no browser in the crawler, where the site table lives, how to add a family), `.agents/skills/gui/SKILL.md` (Search mode, the DTOs, the refresh gate), `tools/site-check/README.md`.

- [ ] **Step 1:** Write them. **Step 2:** `make test` green, `cd gui/frontend && pnpm test` green. **Step 3: Commit** `docs: catalog and search`

---

### Task 14: Review pass

- [ ] Run `go vet ./... && make test && make test/gui`.
- [ ] Read the whole diff with `git diff master` and the spec side by side; list every spec line without code; fix inline.
- [ ] `make catalog/smoke` once more.
