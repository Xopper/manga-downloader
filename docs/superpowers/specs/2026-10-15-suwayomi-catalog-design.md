# Suwayomi catalogs — design

Date: 2026-10-15. Status: approved in conversation. Builds on
`2026-10-15-catalog-design.md` (the catalog files, the store, search, the
nightly job, the app's Search mode).

## Goal

Catalogs for every English-language Tachiyomi source (the keiyoushi list,
adult sources included), built nightly on GitHub by a Suwayomi-Server the
job runs itself, published like the hand-written crawlers' files, so users
search them offline. The downloader can't download from most of these
sites yet; a hit says so.

Not in scope: downloading from the new sites (needs grabbers, or Suwayomi on
the user's machine), other languages (a flag away), covers.

## What the spike showed (2026-10-15, Suwayomi-Server v2.4.2366)

- The keiyoushi store is added at runtime through GraphQL:
  `mutation { addExtensionStore(input:{indexUrl:"https://raw.githubusercontent.com/keiyoushi/extensions/repo/index.pb"}) { clientMutationId } }`.
  `server.conf`'s `extensionRepos` is not enough any more. `GET /api/v1/extension/list`
  then lists 1,430 extensions; 545 have `lang` `en` or `all`.
- `GET /api/v1/extension/install/{pkgName}` installs one (a JAR download, a
  few seconds). `GET /api/v1/source/list` lists the installed sources:
  `id`, `name`, `lang`, `baseUrl`, `supportsLatest`, `isNsfw`; source `0` is
  the local source and is skipped.
- Listings: `GET /api/v1/source/{id}/popular/{n}`, `/latest/{n}`,
  `/search?searchTerm=&pageNum={n}`, each `{mangaList: [{url, title, …}], hasNextPage}`.
  `url` is the site-relative path; the series page is `baseUrl + url`
  (`realUrl` needs a per-manga fetch, too costly). **No listing is complete on
  every source**: MangaPill's popular is its top 10 and its empty search is
  empty, while its latest is 116 titles on one page; Weeb Central pages
  through everything on popular and on an empty search. The crawl unions
  all three.
- A page takes 0.5–1.5 s through Suwayomi.

## The crawl: `manga-downloader catalog crawl-suwayomi`

```
catalog crawl-suwayomi --server http://127.0.0.1:4567 --lang en [--lang all]
    [--out ./catalog] [--max-pages 300] [--parallel 4] [--shard 0/4]
    [--source <name|id>]… [--no-install] [--timeout 4h]
```

1. `GET /extension/list`; every extension with `lang` in `--lang` (and
   `all`, which carries English sources too) and `installed: false` is
   installed, one at a time, failures logged (`FAIL install <pkg>`) and
   skipped. `--no-install` skips this (local runs against a prepared server).
2. `GET /source/list`, keep `lang` in `--lang`/`all`, drop id `0`, drop
   sources whose `baseUrl` is empty (nothing to link to), sort by id, and
   with `--shard i/n` keep every source whose index mod n is i.
3. Per source, `--parallel` at a time (Suwayomi's own
   `server.maxSourcesInParallel` is set to match): walk **popular**, then
   **latest** when `supportsLatest`, then the **empty search**. Each walk
   goes from page 1 while `hasNextPage` is true, stops when a page adds no
   new URL (sources that ignore the page number, like MangaPill's latest),
   after `--max-pages` pages, or after 3 errors in a row (a 5xx from Suwayomi
   is one error; the page is not retried), with 200 ms between pages. Every
   manga becomes `Entry{Title, URL: absURL(baseUrl, url)}`; an absolute
   `url` is kept as is. Entries are deduped by `urlKey` and sorted by folded
   title, so a file only changes when its set of titles changes.
4. The site key is the host of `baseUrl` without `www.` (so a Suwayomi
   source for a site the hand-written crawlers already cover lands in the
   same file). The file is **merged**: the existing file's entries union the
   crawl's (the existing ones first), `Home` is kept when present. A crawl
   that got nothing is a `FAIL` and writes nothing. The half-size guard is
   not needed for a union, but a merged file is still never smaller than
   the file it replaces.
5. `SiteCatalog` gains `Source *SourceInfo` (`json:"source,omitempty"`):
   `{ID string, Name, Lang string, NSFW bool, Pages int, Complete bool}` —
   `Complete` is false when any walk hit `--max-pages`.
6. The run writes `sources-<shard>.json` (or `sources.json` without a
   shard) into `--out`: one row per source attempted:
   `{site, name, lang, nsfw, sourceId, entries, complete, failed}`.
7. Exit codes as `catalog crawl`: 0, 2 when a source failed, 1 for a bad
   flag or an unreachable server. The `--timeout` stops starting new sources
   past it (running walks finish), so a shard ends inside the job's limit.

`catalog index --out <dir>` (new, also used by the existing nightly job):
merges every `sources-*.json` in the dir into `sources.json` (keyed by
site; a site from the hand-written crawlers that no source covers is listed
with `sourceId: ""`), then builds `index.json`. The published index lists
`sources.json` by hash too, under `"extra"`, so `Update` fetches it when it
changed.

## Publishing: `.github/workflows/catalog-suwayomi.yaml`

- `schedule: "7 1 * * *"` and `workflow_dispatch` (with an input for the
  languages, default `en`); `concurrency: catalog-data` shared with the
  other catalog job so they never push at once.
- Job `crawl`, `strategy.matrix.shard: [0, 1, 2, 3]`, `timeout-minutes: 330`:
  `actions/setup-java@v4` (Temurin 21), download the pinned
  `Suwayomi-Server-v2.4.2366.jar` (cached by URL with `actions/cache`),
  write `server.conf` (`server.ip = "127.0.0.1"`, port 4567, web UI off,
  browser open off, `maxSourcesInParallel = 4`), start it in the background,
  wait for `/api/v1/source/list`, add the store through GraphQL, check out
  `catalog-data` into `data/` (so the merge sees the existing files), run
  `catalog crawl-suwayomi --lang en --shard $i/4 --out data/catalog --timeout 4h30m`,
  upload `data/catalog` as artifact `shard-$i` (only the files this shard
  wrote: the CLI writes `written-<shard>.txt`, the job uploads those plus
  `sources-<shard>.json`).
- Job `publish` (`needs: crawl`, runs even if a shard failed): checks out
  `catalog-data`, downloads the artifacts over it, runs
  `catalog index --out catalog` from a built CLI, gzips nothing (see
  Compression), commits `Update Suwayomi catalogs: N entries, M sources`
  and pushes. Fails red afterwards when any shard failed.
- The existing `catalog.yaml` keeps running the hand-written crawlers; its
  commit step also runs `catalog index` so `sources.json` stays consistent.

### Compression

Published files are gzipped: the publish job writes `<site>.json.gz` and the
index lists `file: "<site>.json.gz"` with the hash of the gzipped bytes.
`Update` gunzips after the hash check (`compress/gzip`, standard library);
a plain `.json` file in an index is still accepted. Local files stay plain.
Expected: ~5× smaller downloads.

## Store, CLI and app changes

- `Store.Sources()` returns `sources.json`'s rows (loaded by `Load`, kept
  by `Update`). `Status()` lists the union of the site table and the sources,
  each row gaining `Downloadable bool` (the site is in the table: a grabber
  exists) and `NSFW bool` (from the source). `catalog status` prints a
  `DL` column (`yes`/`-`) and `nsfw`.
- `Search` hits gain nothing; the CLI's table gets a `DL` column; `--json`
  rows gain `"downloadable": bool`. `HitDTO` gains `downloadable` and `nsfw`.
- The app's site picker lists only sites with a catalog (several hundred),
  sorted: downloadable ones first under "Can download", then "Search only";
  no disabled rows any more (the status command has them). A hit from a
  search-only site shows a muted "search only" tag; picking it does not
  load: the panel shows a note "This site isn't supported for download yet"
  with *Copy link* (clipboard) and *Open in browser* (`openURL`).
  `useSearch` is unchanged.
- `CatalogInfo`/`CatalogDTO` gain `sources` (the count of search-only
  sites), shown in the panel head as "N sites, M titles (K search only)".

## Local run (for development)

`tools/suwayomi/run.sh` starts the server from a downloaded JAR into
`~/.cache/manga-downloader-suwayomi/`, adds the store, and prints the
server URL; `tools/suwayomi/stop.sh` stops it. The README's Search section
gets a "Catalogs from Tachiyomi sources" paragraph.

## Testing

- `catalog/suwayomi_test.go`: an `httptest` fake of the four endpoints with
  three sources (one paging on popular only, one whose latest ignores the
  page number, one failing on page 2), checks the union, the stop
  conditions, the shard filter, the merge with an existing file, the
  `sources-*.json` row, exit codes, and that `--timeout` stops new sources.
- `catalog/index_test.go`: gzipped files in an index (good hash, bad hash,
  plain file still accepted), `sources.json` fetched and merged.
- `cmd/`: `catalog index` merging shards; `status` and `search` columns.
- `gui/`: `Sources` in status, the downloadable flag on hits; vitest for the
  grouped picker, the search-only tag and the note.
- Live: `make catalog/smoke-suwayomi` runs `run.sh`, installs MangaPill and
  Weeb Central, crawls them with `--max-pages 2`, checks two files and
  `sources.json` exist.
