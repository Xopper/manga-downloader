# Catalog and search — design

Date: 2026-10-15. Status: approved in conversation, awaiting implementation plan.

## Goal

Let a user find a manga without knowing its URL. Today every grabber takes a
series URL and nothing else. This adds a per-site **catalog** (every title a
site lists, with its link), a **search** over those catalogs, in the CLI and
the desktop app alike, and a nightly job that keeps the catalogs fresh.

Non-goals for version 1: covers, live search on API sites (MangaDex, Manga
Plus, Webtoons, MangaLib), web-wide "search everywhere" through a search
engine. The interfaces leave room for both.

## Overview

```
nightly GitHub Action ──crawl──▶ catalog-data branch (one JSON per site + index.json)
                                          │
                               app / CLI ──update──▶ <config dir>/catalog/*.json
                                          │
                                 search (in memory) ──▶ site, title, URL ──▶ normal download flow
```

## Package `catalog/` (main module)

### Data

```go
type Entry struct {
    Title    string   `json:"title"`
    URL      string   `json:"url"`                 // the series page, what Load/IdentifySite takes
    AltNames []string `json:"alt,omitempty"`       // other titles the site lists (romaji, native, …)
    Latest   float64  `json:"latest,omitempty"`    // highest chapter number the list page showed (Madara), 0 when it doesn't say
}

type SiteCatalog struct {
    Site     string    `json:"site"`               // domain without www, the key everywhere
    Home     string    `json:"home"`
    Language string    `json:"lang"`               // site's main language, BCP-47-ish, "" when mixed
    Crawled  time.Time `json:"crawled"`
    Entries  []Entry   `json:"entries"`
}
```

JSON files are `catalog/<site>.json` (site = domain, dots kept). Sizing: about
100 bytes per entry; ~350k entries across all sites is ~35 MB on disk, ~8 MB
gzipped.

### Site table `catalog/sites.json`

The cleaned list from `tools/site-check/report-2026-10-15.md`, embedded with
`go:embed`. One row per supported site:

```json
{ "site": "asurascans.com", "home": "https://asurascans.com", "family": "astro",
  "list": "https://asurascans.com/series?page={page}", "lang": "en",
  "crawl": true }
```

- `family` names the crawler: `themesia`, `madara`, `wordpress`, `single`, or
  a site-specific name for custom sites once they get one.
- `crawl: false` for sites with no crawler yet, and for the API sites. They
  still appear in `catalog status` and the site picker, marked "no catalog".
- Moved domains from the report are recorded under their new host; the old one
  stays in `aliases` so a pasted old URL still matches the site.

### Crawlers

```go
type Crawler interface {
    // Crawl lists every series the site has. limit > 0 stops after that many
    // list pages (smoke tests). It returns what it got plus the first error.
    Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error)
}
```

Registered by family name in a map. Day-one families and what they read:

| family    | sites | how |
|-----------|-------|-----|
| themesia  | 13    | `/manga/list-mode/` lists every series on one page (`.soralist a`); fallback: `/manga/?page=N` grid until a page lists nothing new |
| madara    | 11    | `/manga/page/N/` (`.post-title a`) until empty; sites with the AJAX list use `admin-ajax.php` `madara_load_more` with `template=madara-core/content/content-archive` |
| wordpress | 14    | generic WordPress manga archive: `/manga/page/N/` or the site's `list` URL, rows `article h2 a` / `.post-title a`; per-site `list` from sites.json |
| single    | 10    | one entry: the site's home page (chainsawman, heavenly-delusion, jojolands, …) |

That is 48 sites on day one. The 58 "custom" plus Next.js/Astro/Nuxt sites get
a crawler each when wanted; until then `crawl: false`. Browser-gated sites are
out of scope for the crawler (it runs on CI without a browser).

Requests go through the project's `http` package (same UA and headers as the
grabbers), 1 request at a time per site, 2 sites in parallel in the nightly
job, with a 300 ms pause between pages. Entries are deduped by URL path.

### Store

`Store` reads and writes `<os.UserConfigDir()>/manga-downloader/catalog/`
(next to `gui.json`): `index.json` plus one file per site, written atomically
(temp file + rename, 0600 like `gui.json`).

- `Load()` reads every file into memory once; a file that does not parse is
  skipped with a warning, never a failure.
- `Update(ctx)` fetches the published `index.json`, compares each site's
  `sha256` with the local one, downloads only changed files, writes the new
  index last. Network errors leave the local files untouched.
- `Status()` returns per site: entries, crawled time, local file age, and
  whether the site has a catalog at all.

Published location: `https://raw.githubusercontent.com/Xopper/manga-downloader/catalog-data/catalog/`.
The base URL is one variable, overridable with `MANGA_DOWNLOADER_CATALOG_URL`
for tests and forks.

### Search

```go
type Hit struct { Site string; Entry Entry; Score int }
func (s *Store) Search(query string, sites []string, limit int) []Hit
```

- `sites` empty means all. `limit` default 50.
- Query and titles are folded: lowercase, Unicode NFKD with marks stripped,
  punctuation collapsed to spaces. Each query word must appear in the folded
  title or an alt name (any order).
- Score: exact folded title match > prefix > all words as word prefixes > all
  words as substrings. Ties by title length (shorter first), then site name.
- Folding is done once at `Load()` and kept beside each entry; search is a
  linear scan, fine for a few hundred thousand entries (well under 100 ms).

## CLI (`cmd/`)

```
manga-downloader search <query> [--site <domain>] [--limit N] [--json]
manga-downloader catalog update            # download changed files from the published index
manga-downloader catalog status [--json]   # per site: entries, crawled, file age
manga-downloader catalog crawl [--site <domain>] [--limit N] [--out <dir>]
```

- `search` loads the store, runs `Update` first only when there is no local
  index at all (first run), prints `site  title  url` as aligned columns, or
  JSON. Exit 1 when nothing matched. Pasting a printed URL into the normal
  command is the download path; `search` never downloads.
- `catalog crawl` is what the nightly job runs. `--out` defaults to `./catalog`.
  Each site writes its file only on success; a crawl that returns fewer than
  half the entries of the existing file (when one exists) keeps the old file
  and prints a warning, exit code 2 at the end if any site did that or failed.
  `--limit` crawls that many list pages, for smoke checks.

## Nightly job (`.github/workflows/catalog.yaml`)

- `schedule: "17 3 * * *"` and `workflow_dispatch`, `permissions: contents: write`.
- Checks out `master`, builds the CLI, checks out the orphan `catalog-data`
  branch into `data/` (creates it the first time), copies its `catalog/` in as
  the "existing files" for the half-size guard, runs `catalog crawl --out data/catalog`,
  writes `index.json` (site, file, entries, crawled, sha256), commits to
  `catalog-data` with the counts in the message. Default `GITHUB_TOKEN` is
  enough: it writes to its own repository.
- The job fails (red) when every site of a family failed, which is how a
  broken crawler is noticed. One site failing is a warning in the log only.
- The branch is orphan for the same reason as `star-tracker-data`: generated
  data must not bury real commits on `master`.

## Desktop app (`gui/`)

Follows the controller/app split from the `gui` skill.

- **Controller**: owns a `catalog.Store`. On startup, loads it, then in the
  background calls `Update` when the local index is missing or older than 24 h
  (same shape as the update check: cancelled at shutdown, failures to stderr
  only). `Search(query, site)` and `Sites()` are plain methods. The store's
  refresh flips a generation counter so the window can show "catalog updated".
- **App (bound)**: `Search(query, site string) []HitDTO`, `Sites() []SiteDTO`
  (domain, label, language, hasCatalog, entries), `CatalogStatus() CatalogDTO`
  (sites with a catalog, total entries, last update, updating bool). DTOs in
  `app.go`, types mirrored in `frontend/src/types.ts`, bindings regenerated,
  `api.ts` and `mock.ts` extended (mock gets a small fixed catalog and a
  `?scenario=search` entry).
- **Window**: the source bar gets a mode switch, *URL* / *Search*. Search mode
  shows a text field, a site selector (All sites + each site with a catalog;
  sites without one listed disabled with "no catalog") and a results list
  below the bar (virtualised like the chapter list; title, site, language).
  Typing searches after the same pause the URL box uses. Clicking a result
  fills the URL and calls `Load`, switching the bar back to URL mode. The
  status bar shows the catalog's age and a spinner while it updates.
- The Search field's input is 16px on narrow windows (existing field rules).

## Error handling

- No catalog yet (first run, offline): search mode shows "No catalog yet"
  with an *Update now* button; CLI prints the same and exits 1.
- Update fails: local files stay; the app shows the last update time in
  yellow; CLI `catalog status` shows the age. Nothing else changes.
- A result URL whose site no grabber matches (site table drift): `Load`
  fails exactly as a pasted URL would, with the existing error.

## Testing

- `catalog/`: fixture HTML per family under `catalog/testdata/<family>/`
  (list page, last page), table tests for each crawler's parsing, pagination
  stop, dedupe. Store tests on a temp dir: load, atomic write, update with a
  local `httptest` server serving an index (changed / unchanged / failing
  file). Search tests: folding (accents, kana width, punctuation), ordering,
  site filter, limit.
- `cmd/`: `search` output and exit codes against a temp config dir
  (`XDG_CONFIG_HOME`/`HOME` override as the existing tests do).
- `gui/`: controller tests for search and the refresh gate; vitest for the
  mode switch, result click → load, and the empty states, against `mock.ts`.
- Live: `make catalog/smoke` runs `catalog crawl --limit 1` for one site per
  family; `tools/site-check/` stays the liveness probe.

## Docs to update

README: a "Search" section with the commands and a line in the features list;
`AGENTS.md`: a "Catalog" section pointing here and listing the invariants
(orphan branch, half-size guard, no browser in the crawler); the `gui` skill:
the search mode and the new DTOs; the website's supported-sites list is
unaffected.
