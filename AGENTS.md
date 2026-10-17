# AGENTS.md

Guidance for coding agents working in this repository.

## Project

Go CLI (cobra) that downloads manga chapters from supported websites and packs them into CBZ or EPUB files, plus a Wails desktop app in `gui/` (see the `gui` skill). Module: `github.com/elboletaire/manga-downloader`, targeting Go 1.26.0 (the `go` directive in `go.mod` and `gui/go.mod`, pinned by CI's test and smoke workflows).

## On-demand guides

These guides are not loaded with every session. Open the file when the task matches; agents that load skills by name can use the skill instead.

- **Sites and grabbers** (adding, fixing or investigating a site (new or supported), smoke-test failures, Cloudflare/browser problems): the `grabbers` skill, `.agents/skills/grabbers/SKILL.md`.
- **Desktop app** (`gui/`: Go backend, React frontend, bindings, packaging, theme, options): the `gui` skill, `.agents/skills/gui/SKILL.md`.
- **Release assets** (retaking the README and website screenshots and demo GIFs before a release; macOS only): the `pre-release` skill, `.agents/skills/pre-release/SKILL.md`.

## Commands

```bash
make install          # go mod download
make build            # clean + test + build unix binary (injects version via ldflags)
make build/all        # unix + windows binaries
make build/demo       # the VHS tapes' binary, served from demo/series (demo tag, see demo/README.md)
make test/demo        # the demo build's tests
GOOS=windows GOARCH=386 make build/release   # one release binary, built as release.yaml does
make test             # go test -v ./... (uses richgo if installed)
go test ./ranges -run TestParse   # run a single test
make clean            # remove built binaries and *.cbz and *.epub files
```

Unit test coverage is thin; real verification is the makefile smoke targets, which download from live sites — run them selectively. `make grabber` runs everything, `make grabber/html` all `PlainHTML` sites, `make grabber/<site>` just one; check the archives with `tools/verify-cbz`, since a run can succeed with empty or junk CBZs.

The makefile is tracked lowercase; on macOS `git add` needs the lowercase name. macOS also has no `timeout`.

Releases: the version lives only in the git tag (the makefile injects the short sha, the release workflow also injects `git describe`). There's no CHANGELOG and no hardcoded version. **`release.yaml` triggers on a *published GitHub Release*, not on a tag push** — pushing a tag alone releases nothing.

- **Website**: `website/` is the GitHub Pages site (Vite + TS, pnpm, deployed by `pages.yaml`). Its supported-sites list is parsed from this README's "Supported sites" section at build time, so keep that table's `| [domain](url) \* | notes | last verified |` row format and the "Show all N" count in sync. The "Last verified" column holds a date, `❌ failing since <date>` or `—` (never checked; the start for a new site): `tools/site-status` rewrites only that column from smoke results, run weekly by the last job of `smoke.yaml` (which commits the README) and by hand with `make site-status TARGETS="toonily"` for the browser sites CI can't run. It maps a smoke target to its row by the host of its first URL, and reports unmapped targets and rows (CI `::warning::`s); a row with no smoke target keeps `—` until one exists. The job commits through the GraphQL `createCommitOnBranch` (master requires signed commits), and the website shows the new dates at the next daily Pages run. It's published in English, Spanish and Catalan: page text lives in `website/locales/*.json` (i18next format, rendered at build time), so add or change a string in all three files. See `website/README.md`.
- **Homebrew**: `packaging/homebrew/tap/` holds the formula (`manga-downloader`, CLI) and cask (`manga-downloader-app`, desktop app; `Casks/manga-downloader.rb` is its deprecated old-name alias) for `elboletaire/homebrew-tap`. Each published release runs a `homebrew` job in `release.yaml` and `release-gui.yaml`, which copies them into the tap and sets the version and sha256s with `scripts/update-homebrew.sh`. Both jobs share one concurrency group. Keep the asset names in those files in step with the release workflows. See `packaging/homebrew/README.md`.

## Architecture

- **One HTTP transport** (`http/request.go`): every request goes through the
  shared `transport`, so a site's connection is kept and reused (the TLS
  handshake was most of a small page's time; a transport per request also
  leaked idle connections). `PageMemory` (off by default and in tests; the CLI
  and the app set 30 s) remembers plain document GETs, so identifying a site
  and then fetching its title and chapters asks for the series page once; a
  load and a library check start with `ResetPageMemory`, so they fetch.

The download flow lives in `core` (`core/download.go`, `Series.Download`), UI-agnostic so the CLI and the GUI share it. `cmd/root.go` (`Run`) is the CLI shell around it: it parses flags and renders the `core.Events` through `cmd/progress.go`.

The steps:

1. `grabber.NewSite(url, settings)` → `IdentifySite()` calls `Test()` on each registered grabber (`grabber/site.go`), matching by domain or by fetching the URL. **Order matters**: domain-matching grabbers first, then `PlainHTML`, then the remaining fetch-testing ones. After all grabbers decline, `PlainHTML` gets a final chance via `resolveSeriesURL()`, and only then `WordPressCategory` and `Blogger` are tried by markup (themesia chapter pages are WordPress posts too).
2. The matched `Site` fetches title and chapters; the range argument (`ranges.Parse`, `1-10,12,15-20`) filters them.
3. Each fetched chapter is passed through `Chapter.SkipPages` (`--skip-pages`, see below) before anything is downloaded, inside `core.Download`.
4. Each chapter's page list is fetched after `Events.Fetching` (renderers show the row as "fetching pages" until `Started`; `--bundle`'s totals pre-pass doesn't emit it). Chapters download concurrently (`downloader.FetchChapter`), bounded by `--concurrency` (max 5) and `--concurrency-pages` (max 10). Pages are plain GETs with a Referer.
5. `packer` writes CBZ, EPUB or raw output (`PackSingle`, or `PackBundle` with `--bundle`), named via a text/template (`--filename-template`, `packer/filename.go`; `{{.Title}}` is `grabber.ChapterLabel`, `{{.SiteTitle}}` the site's title alone); duplicate names get a `v{{.Version}}` suffix. Both paths go through `namePages` (`packer/pack.go`), which sniffs each page's format to name it `000.jpg`, `001.png`, … and transcodes per `--convert-images`.

**Chapter URL resolution**: both series and chapter links are accepted; see the grabbers skill for the full pattern.

**Design every feature for both the CLI and the desktop app.** They're meant to stay (almost) 1:1: a new option or behaviour lands in `core` where possible, then gets a flag in `cmd/` *and* a control in `gui/` (see the `gui` skill), with the same name, default and semantics, in the same change. The deliberate exceptions are few — `--concurrency`/`--concurrency-pages`/`--retry` aren't exposed in the app, and the app selects chapters from a list (with a range box that takes the same `1-10,12` syntax) instead of a positional range argument — so don't treat "the CLI already has it" as done, and say so explicitly when a feature is intentionally one-sided.

### Volumes (`--volumes`)

One file per volume, between the per-chapter default and `--bundle`. `core/volumes.go` groups the chapters (`GroupVolumes`): `auto` by `grabber.VolumeOf` (the listing's `Chapter.Volume`, 0 = unknown, filled only by grabbers whose listing already says: mangadex, mangalib, the mangahere family's `v98/c1195` URLs, Madara's "Volume N" wrappers in `tcb`), or by `ranges.ParseVolumes` groups (`"1-7/8-16"`, `/`-separated so it never clashes with `ranges.Parse`, volume N = Nth group, overlaps refused). A group can name its number (`"16:112-114"`, whole numbers only, as `Chapter.Volume` is an `int`); unnamed groups count on from the previous one, a number given twice is refused, and `core.SiteVolumes` names a volume only at a gap so the contiguous case stays `"1-7/8-16"`. `ranges/testdata/volumes.json` holds cases both the Go tests and `volumes.test.ts` run. Chapters outside every volume are packed one file each, never dropped.

- **No totals pre-pass**: unlike `--bundle`, each chapter is fetched once (`VolumeStart` comes first, then the chapter events, routed by `VolumeChapter`). A volume is packed by whichever of its chapters finishes last, so memory holds one volume at a time, not the run.
- **Progress is `VolumeProgress`, a 0–1 fraction core computes**, not page counts: with no pre-pass the page total is only known at the end, and growing it as chapters started pulled the bars back. Each chapter weighs the same in the download share, 80% of the fraction (by its pages once started, in full once done or failed), and the archive's pages make up the other 20%. Core emits it under the volume's lock, so it never decreases and arrives in order; renderers (`cmd/progress.go`, the controller) only set it, ignoring `Page` for a volume's chapters.
- **Naming**: `packer.VolumeTemplate` swaps in `FilenameVolumeTemplateDefault` only while `--filename-template` is the chapter default; a custom template names volumes too. It's checked before anything is fetched, since a volume failing to name would waste every chapter in it. The pre-`{{.Group}}` default (`filenameTemplateLegacyDefault`) still counts as the chapter default, and the GUI store migrates it with `packer.MigrateFilenameTemplate`.
- **App**: "Pack as" (`packAs` in `gui.json`, migrated from the old `bundle` key) maps onto `Bundle`/`Volumes`; the Volumes box is per load (prefilled from `core.SiteVolumes`), not an option, and replaces `Settings.Volumes` at `Start`. `frontend/src/volumes.ts` mirrors the parsing and grouping for the list; keep it in step with `ranges.ParseVolumes`/`core.GroupVolumes`/`core.SiteVolumes`.

### Scanlation groups (`--scanlator`)

A release's group is a field of the chapter, never part of its title: `Chapter.Group` is the display string and `Chapter.Groups` the names apart (a name can hold ", "), both set by `Chapter.SetGroups`, and `GetGroups` falls back to splitting `Group` when a grabber didn't set them. `core.Open` keeps every release by default (and with `all`), and `--scanlator <name>` keeps the group's releases, matched ignoring case (releases that name no group are left out then); the listing's own groups are what it picks from (`grabber.GroupOptions`, `grabber.SelectGroup`), so every site that sets its groups through `SetGroups` gets the same behaviour. A site that can only list one group per request implements `ScanlatorLister` and does its own filtering through `SelectGroup`, so the unknown-group error and the match are the same either way. The app keeps the unfiltered listings of its loads (`gui/listings.go`, the last 8, per URL and language; per group too for a `ScanlatorLister`), so a group change re-filters the cached listing without a request and switching language back is instant; an explicit reload always fetches. `core.Fetch` and `Listing.Series` are the two steps of `core.Open` for that. `grabber.Refilterable` is for the sites whose cached walk serves another language without a new walk. The group reaches the CLI and app chapter lists (`core.HasGroupDuplicates`), `{{.Group}}` in file names (empty for a volume or bundle whose chapters name different groups), and the EPUB identity key (`chapterKey`, which falls back to the group and title without an ID). A `PlainHTML` site whose rows list several uploads per chapter (zonatmo) sets `SiteSelector.Uploads`, one chapter per upload; see the grabbers skill for the fields and the default `ID`. CLI messages and app activity name a release via `core.ReleaseLabel`, with the group only when a number has several releases (`core.HasGroupDuplicates`), whereas default file names carry the group whenever the release has one.

### Page conversion (`--convert-images`, #155)

Some sites serve pages e-readers can't render — AVIF (atsu.moe, mistscans, mangalib) and WebP (unreliable on e-ink). `packer/convert.go` re-encodes to JPEG q90: AVIF by default, WebP opt-in (`avif,webp`), `none` disables both.

- **The decoder is `github.com/gen2brain/avif` with `-tags nodynamic`** — libavif/dav1d as WASM under wazero, so `CGO_ENABLED=0` and the 8-way cross-compile matrix keep working (any cgo decoder is disqualified by that). The tag forces the embedded WASM instead of `dlopen`ing a system libavif. **It's set in one place, `BUILD_TAGS` in the `makefile`**: the binaries built by `release.yaml` (via `make build/release`), `release-gui.yaml` (via `wails build` with tags from `make -s print-GUI_GO_TAGS`), the Dockerfiles (via `make build/unix`), and `gui-test.yaml` (tags from `make -s print-GUI_GO_TAGS`) all use it; the grabber smoke targets run plain `go run .` without the tag. Don't pass build flags from a workflow directly: without the tag the library silently tries a system libavif first, so a mismatch is easy to miss.
- **The seam is `namePages`, not `grabber.Page.Transform`**: a `Transform` error re-runs the download retry loop, so a decode failure would pointlessly refetch the page. `namePages` is also where extensions are assigned, so converting there makes the `.jpg` follow automatically.
- **`flattenAlpha` composites onto white first**, because `jpeg.Encode` drops alpha rather than compositing — transparent margins would otherwise come out as solid black bars. Already-opaque images pass through untouched to keep `jpeg.Encode`'s fast path.
- **A page that fails to convert keeps its original bytes *and* extension**, with a warning: `pack()` has no partial-success path, so erroring would lose the whole chapter (or bundle) over one page. The extension must never claim `.jpg` for non-JPEG bytes.
- **`tools/verify-cbz` is what actually proves conversion happened in CI**, precisely because that failure is non-fatal: it rejects `.avif` entries (`-allow-avif` for `--convert-images=none` archives) and checks entry names against sniffed content.
- The WebP test fixture is a real file inlined as bytes (`x/image/webp` is decode-only, and Go has no test-only dependency scope), which also pins the load-bearing blank import in `convert.go`.

### Page skipping (`--skip-pages`)

Scanlation groups reserve pages for credits/ads (tcbscans: always page 2, often the last one too), and a page that 404s on the site's end fails the *whole* chapter with no way around it — `https://tcbonepiecechapters.com/chapters/7855/one-piece-chapter-1146` was flatly undownloadable until this flag existed. `Chapter.SkipPages` (`grabber/chapter.go`) drops the selected pages, called from `core.Download` right after `FetchChapter`.

- **Positions are 1-based indices into `chapter.Pages`, never `Page.Number`.** That field is not a position: most grabbers set `i+1`, a couple (`mgeko`, `projectsuki`) set a bare `i`, and `tcb` sets the *reader page* number, which several images share. `SkipPages` deliberately leaves `Page.Number` alone afterwards, so a download error still names the page the site numbers; the archive stays contiguous anyway because `namePages` numbers by slice index.
- **Skipping happens before the download, not at packing time.** That's the entire point (a skipped page costs nothing and can't 404 the chapter), and `downloader.FetchChapter` aborts the chapter on the first page error, so a post-download filter could never rescue a broken page.
- **`core.Download` calls `FetchChapter` twice in `--bundle` mode** — once in the totals loop that sizes the progress bar, once in the download goroutine. Any per-chapter filtering has to be applied at *both*, the same way. The download's fetch can still find other pages than the totals one (or the totals one can fail), so `core.Download` counts `len(Pages)` on both sides and, when they differ, corrects the total and emits `BundleStart` again.
- **`ranges.Parse` rejects negatives; `ranges.ParseRelative` accepts them.** Two entry points rather than one, because counting from the end is meaningless for chapter *numbers* (and chapter 0 exists), so the chapter-range caller should fail loudly on a negative instead of filtering nothing. `ranges.Resolve(rngs, total)` is what turns `-1` into a position, per chapter.
- The parser is regex-based now: it used to `strings.Split(part, "-")`, which can't tell `-3--1` apart from anything. Reversed ranges still clamp to their first bound (`10-5` → just 10) as they always have, but only when both bounds share a sign — `2--1` ("page 2 to the last") is ordered, just not comparably so until `Resolve` knows the total.
- **pflag takes the argument after a long flag verbatim, dash and all**, so `--skip-pages -1` works and the `=` form isn't actually required (`parseLongArg` never inspects the value for a leading `-`). A *shorthand* would be a different story, which is why this flag has none. The README documents both forms.

### EPUB output (`--format epub`)

EPUB is another archive next to CBZ, grouped the same way (per chapter, `--volumes`, `--bundle`). `packer/epub.go` writes it with no new dependency.

- **`ArchiveEPUB` takes an `EPUBInfo`, not just the files.** The series, the volume or chapter number (`group-position`), the reading direction and the table of contents labels can't be recovered from the entry names, so the `Pack*` functions pass them in. Labels are keyed by folder (`chapterLabel`: "Chapter 265 - Title"), and a folder without one is named after the folder.
- **AVIF is always converted for EPUB**, whatever `--convert-images` says, since readers can't show it: `pageFormats` adds it for `epub`, and every Pack function goes through it.
- **A page EPUB can't show is left out with a warning**, rather than failing the book: anything but JPEG, PNG, GIF or WebP, or a page whose size can't be read. It still counts towards progress, since the archive's total is the pages downloaded. The book fails only when no page is left.
- **Reading direction** is `--reading-direction rtl|ltr` (default `rtl`) and a control in the app, shown only for EPUB. It sets the spine's `page-progression-direction`. Deriving it from the series' language is deferred.
- **`dc:language` is `und` on purpose.** `Settings.Language` is the site's filter (`en`, `es`, `all`, or a site's own code), not a BCP 47 tag, so it can't be written as the book's language.
- **The identifier is a UUID v5 of the series and what the book holds** (`chapter`, `volume N` or `bundle` plus each chapter's number, language and `Chapter.ID`, or its group and title without one), so a repack keeps it and two releases of one chapter don't share it. A grabber that lists several releases per number sets their groups and `Chapter.ID` as described in "Scanlation groups", and `FetchChapter` keeps both on what it returns.
- **Pages are fixed layout**: one XHTML per image, with a viewport at the image's size, and the first page is the cover. Calibre ignores fixed layout and stretches the image box to the screen, so the `<img>` carries `object-fit: scale-down` (a no-op in readers that honour the viewport).
- **epubcheck isn't in CI yet**: check a change by running epubcheck on a real download, the way `tools/verify-cbz` checks CBZs.

### Catalog and search (`search`, `catalog …`)

A catalog is one JSON per site (title, series URL, other names, and for
the Madara family the latest chapter number its list page showed, `latest`)
in `catalog/`; `Store.Search` scans them in memory after folding (`Fold`:
lowercase, NFKD, accents dropped, dakuten kept). Design:
`docs/superpowers/specs/2026-10-15-catalog-design.md`. The store is big (a
million and a half entries), so it doesn't keep `Entry` values: each site is
packed into a `block` (`catalog/pack.go`) as `Load` or `Write` reads it, the
folded names in one byte arena the scan walks (titles and other names parted
by a zero byte) and the titles, links and other names in deflated pages of
256 entries, inflated only for a hit (the site's common link prefix is kept
once). That is ~115 MB of heap for the full set, from ~400 MB as entries
plus an index. `Catalog(site)` rebuilds a site's entries from its block (a
copy, for the crawlers); inside the package, `has` and `count` answer
without rebuilding. `Load` and `Search` work on every core at once
(`inParallel`); a search scans the blocks without the lock (a block never
changes, a write swaps in a new one), ranks the matches as `key`s and keeps
the `limit` best through a heap, so a one-letter query that matches most of
the store costs about what any query does; only the kept hits inflate a
page. Measure before touching any of it (a load takes ~0.6 s, a search for
"one piece" ~25 ms).

- **The site table is `catalog/sites.json`** (embedded): every README site
  with its crawler `family`, list page, language, former domains (`aliases`)
  and `crawl` flag. `tools/site-check/` is the probe that filled it; rerun it
  before editing the table. A site with no crawler yet, an API site or a
  browser-gated one is `crawl: false` and shows "no catalog" in `status`.
- **FlareSolverr** (`http/flaresolverr.go`, off until `http.SetFlareSolverr`,
  `--flaresolverr` or the app's Advanced option): a GET answered with
  Cloudflare's challenge (`isChallenge`) has it earn the host's clearance once
  (per-host lock, not again for 10 min), then retries with its cookies and
  per-host user agent (`SetHostUserAgent`). If the retry still gets the
  challenge, those are dropped and the host is "proxied": its non-picture
  GETs are read through FlareSolverr (Chrome's JSON view unwrapped).
  `tools/site-try` reads `FLARESOLVERR`.
- **`tools/site-try`** checks sites end to end without saving: grabber,
  title, chapters, one free chapter's pages, the first page fetched and
  sniffed as a picture (AVIF included). A site is only added to the table
  once it passes there.
- **Crawlers are per family, not per site** (`crawl_*.go`, registered in
  `crawlers` by `init`): `themesia` (`/manga/list-mode/`, else the
  `/manga/?page=N` grid), `madara` (`/manga/page/N/?m_orderby=alphabet`: the default order is
  "latest update", which shifts between requests so pages overlap and the
  walk stops early; a site whose series live elsewhere, `/title/` or
  `/seri/`, has that in its `list`), `single` (one entry: the home page),
  `gigaviewer` (Hatena's GigaViewer hosts: one page, `/series` or `/comics`,
  whose episode links each stand for a series; the title comes from the
  link's `data-series-title`, a title element, or its picture's alt, with a
  paperback's "３巻"/"【…】" cut off), `sitemap` (sites with no list page
  but a sitemap: `list` is the sitemap or index, `sitemaps` picks an
  index's children, `series` picks the series URLs, its first group mapping
  a chapter URL to its series, or filling `seriesURL`'s `{1}`; titles come
  from the slug, long numeric or hex ids cut off). They run without a browser, through `http.GetText` with
  `DisableRetry` (a 404 is how a paged list ends), paced by `pagePause`, and
  stop when a page adds nothing new (a site that ignores `page=`). Tests use
  `withFakeFetch` and trimmed fixtures under `catalog/testdata/<family>/`.
- **Published on the orphan `catalog-data` branch** by
  `.github/workflows/catalog.yaml` nightly, like `star-tracker-data`:
  `<site>.json` files plus `index.json` (entries, crawl time, sha256).
  `Store.Update` refuses an index entry whose site isn't a bare domain or whose
  file isn't `<site>.json` (the name becomes a path under the catalog dir),
  downloads only files whose hash changed, verifies the hash,
  and keeps the local file and index entry of anything that fails, so the next
  update retries it. `catalog.BaseURL()` is the branch's raw URL, overridden by
  `MANGA_DOWNLOADER_CATALOG_URL`.
- **The half-size guard**: `catalog crawl` keeps a site's existing file when
  the new crawl has fewer than half its entries (exit 2, "kept" in the log), so
  a broken site never wipes a catalog. An empty crawl, or one that stopped on
  an error partway (a 503 on page 12), is a failure: nothing is written, so a
  catalog can't shrink run by run. The workflow commits what worked and then fails on exit 2, so a
  broken crawler is noticed.
- **Files are written atomically** (temp + rename, 0600) under
  `os.UserConfigDir()/manga-downloader/catalog/`, next to `gui.json`
  (`catalog.DefaultDir`).
- **The app** (`gui/catalog.go`): the controller owns the store, loads it
  once the window has asked for its first state (`App.shown`, or 3 s), so the
  window is up before the cores and the memory go to the catalogs, and
  refreshes in the background when the index is a day old
  (`RefreshCatalog`, cancelled at shutdown like the update check; a failure
  stays in `State.catalog.error`, never the window's log). `Search`, `Sites`,
  `CatalogStatus` and `UpdateCatalog` are bound; the window's results page is
  `ResultsPanel.tsx` + `useSearch.ts`, with the mock's own small catalog
  (`&nocatalog` starts without one). The library of followed series
  (`gui/library.go`, `library.json`) is described in the GUI skill.
- **Adding a family**: a `crawl_<family>.go` with its `init` registration and
  a fixture test, the family name on the sites in `sites.json` with `crawl:
  true` and their `list` URL, and a line in `make catalog/smoke`.

### Suwayomi catalogs (`catalog crawl-suwayomi`, `catalog index`)

Most catalogs come from Tachiyomi sources through a Suwayomi-Server
(`catalog/suwayomi.go`), run by `.github/workflows/catalog-suwayomi.yaml`
nightly, four shards. Design:
`docs/superpowers/specs/2026-10-15-suwayomi-catalog-design.md`.

- **The keiyoushi store is added at runtime through GraphQL**
  (`addExtensionStore(input:{indexUrl:".../keiyoushi/extensions/repo/index.pb"})`,
  in `tools/suwayomi/run.sh`); `server.conf`'s `extensionRepos` no longer
  does it. The REST API is `/api/v1/extension/list|install/{pkg}`,
  `/source/list`, `/source/{id}/popular|latest/{n}` and
  `/source/{id}/search?searchTerm=&pageNum={n}`; a series URL is the source's
  `baseUrl` plus the manga's `url` by plain concatenation (`joinBase`), as
  Tachiyomi's `HttpSource` does, never URL resolution.
- **A series link is base + path only for about half the sources** (measured
  2026-10-15: 273 same, 201 different, 101 unchecked of 585). `resolveLinks`
  fetches one title's details (`/api/v1/manga/{id}?onlineFetch=true`, which
  fills `realUrl`) and compares: equal → `links: joined`; else a template is
  learnt from the sample (the joined path's last segment under another
  prefix, mangadex's `/manga/x` → `/title/x`) and confirmed on a second
  title → `templated`; else every title's link is fetched, `--max-fetch`
  (200) per source and run, the titles not reached left out rather than
  stored with a wrong link → `fetched`; a check that fails keeps the joins →
  `unchecked`. A file whose links were joined is rebuilt, not unioned, once
  its source proves fetched or templated (`linksReplace`).
- **No listing is complete on every source** (MangaPill: popular is its top
  10, latest is 116 titles on one page, the empty search is empty), so
  `walkSource` unions popular, latest and the empty search, each until
  `hasNextPage` is false, a page adds nothing new, the page cap (`--max-pages`,
  300) or three errors in a row. `SourceInfo.Complete` is false when a cap
  cut a listing.
- **The site key is the host of `baseUrl`** without `www.` (`siteOfBase`), so
  a Suwayomi source for a site a hand-written crawler covers lands in the
  same file, as a **union** (`mergeSite`: existing entries first, sorted by
  folded title; a set that didn't change is not rewritten, so its published
  hash and `crawled` time stay put). A merged file is never smaller than the
  one it replaces; an empty crawl writes nothing. Writes to one site are
  serialised, and shards are split by a hash of the host (`shardOf`), so two
  sources on one host (a mirror, an `en` and an `all` source) always merge
  inside one process. The hand-written `catalog crawl` keeps a file's
  Suwayomi entries (after its own) when the file has a `source`, so the two
  nightly jobs don't undo each other.
- **A partial crawl is a failure** for the hand-written crawlers (nothing
  written) but not here: a source whose walk hit the cap or three errors
  still merges what it got (a union can't shrink a file) and is marked
  `complete: false`.
- **Shards**: `--shard i/n` keeps every source whose index in id order is
  `i` modulo `n`; each shard writes `sources-<i>.json`, and `catalog index`
  (the publish job) merges them into `sources.json` and removes them.
  `--timeout` stops starting sources (checked when a slot frees), so a shard
  ends inside the job's limit; the sources it skipped are rows with
  `failed: true`. A shard goes red only when more than half of its sources
  failed or none was written; the failed ones are listed in the run summary
  (about a hundred fail every night: Cloudflare walls, dead sites). Every request to the server has a deadline
  (`RequestTimeout`, 120 s), so a stuck extension can't hold a slot.
- **Published files are gzipped** (`catalog index --gzip` → `<site>.json.gz`,
  hash of the gzipped bytes in the index; `sources.json` is listed under the
  index's `extra`). `Store.Load` reads both forms (a plain file wins over a
  stale gz twin), `Update` gunzips after the hash check and keeps local files
  plain. Both catalog workflows run `catalog index --gzip` before committing.
- **Search only**: `Store.Status` unions the site table with `sources.json`;
  `SiteStatus.Downloadable` is "in the table" (a grabber exists). The CLI
  shows a `DL` column, JSON rows carry `downloadable`, the app's picker
  groups "Can download" / "Search only", a hit carries `downloadable` and
  `nsfw` (the source's flag, which means "may contain"), and picking a
  search-only hit shows a note with *Copy link* / *Open in browser* instead
  of loading.
- **Local**: `tools/suwayomi/run.sh` (Java 21+, data under
  `~/.cache/manga-downloader-suwayomi`), `make catalog/smoke-suwayomi`
  (MangaPill + Weeb Central, two pages, `--no-install`).

