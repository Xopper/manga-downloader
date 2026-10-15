# Library First Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the window around a library of followed series: home is the library with "new chapters" per series, one search box at the top finds or loads a series, the series page keeps the chapter list with the save settings on the right, and a strip at the bottom shows a running download.

**Architecture:** A new toolkit-free `Library` (Go, `gui/library.go`) persists followed series and the counts of every series seen in `library.json`; the controller updates it on loads and finished runs and can check every followed series for new chapters. `State.library` carries it to the window. The frontend's `App.tsx` becomes a page switch (home, results, series) under a top bar, reusing the existing chapter list, options controls, activity and log panels.

**Tech Stack:** Go 1.26 (`GOTOOLCHAIN=auto`), Wails v2, React 19 + TypeScript, vitest, pnpm. macOS builds need `CGO_LDFLAGS="-framework UniformTypeIdentifiers"` (the makefile's `GUI_CGO`).

**Spec:** `docs/superpowers/specs/2026-10-15-library-first-design.md`

## Global Constraints

- Colours, type and the existing controls stay (`styles.css` tokens, Inter, the chapter list, the option fields).
- `library.json` lives next to `gui.json`, 0600, written atomically (as `saveOptions`).
- Every Go command runs with `GOTOOLCHAIN=auto`; gui tests with `CGO_LDFLAGS="-framework UniformTypeIdentifiers"` and tags `desktop,production,nodynamic`.
- Keep `controller.go` toolkit-free; `app.go` stays the thin adapter.
- Change `types.ts`, `api.ts`, `mock.ts` and the Go DTOs together; regenerate the bindings with `wails generate module` and revert `frontend/wailsjs/runtime`.
- Keep the existing tests green; delete only the tests of components that go away (`SourceBar`, `SearchPanel`).

## Review Focus

1. A followed series whose URL the user pastes in a different form (trailing slash, `www.`, upper-case host) must still count as the same series: `urlKey` test in Task 1.
2. A finished run that saved nothing (every chapter failed or cancelled before the first) must not move `savedUpTo`: test in Task 3.
3. *Check for new chapters* while a series is loaded must leave that series, its selection and filters untouched, and a failing site must not stop the check of the others: tests in Task 4.
4. A `library.json` that doesn't parse must not lose the user's list on the next save (load keeps the file, logs a warning, starts empty but saves to a `.bak` first): test in Task 1.
5. Results for a query typed while a download runs must still render and follow must still work, but picking a row must not load (busy): covered by the `busy` prop test in Task 8.

---

### Task 1: The library on disk (`gui/library.go`)

**Files:**
- Create: `gui/library.go`, `gui/library_test.go`
- Modify: `gui/store.go` (extract `writeJSONAtomic(path string, v any) error` from `saveOptions`)

**Interfaces (produces):**

```go
type Followed struct {
    URL          string    `json:"url"`
    Title        string    `json:"title"`
    Site         string    `json:"site"`
    Downloadable bool      `json:"downloadable"`
    Chapters     int       `json:"chapters"`
    Latest       float64   `json:"latest"`
    SavedUpTo    float64   `json:"savedUpTo"`
    New          int       `json:"new"`
    FollowedAt   time.Time `json:"followedAt"`
    CheckedAt    time.Time `json:"checkedAt,omitempty"`
    LastSaved    time.Time `json:"lastSaved,omitempty"`
    Error        string    `json:"error,omitempty"`
}
type Seen struct { Title string; Chapters int; Latest float64; At time.Time }   // json: title, chapters, latest, at
type Library struct { mu sync.Mutex; path string; followed []Followed; seen map[string]Seen }

func NewLibrary(path string) *Library                 // path "" = memory only
func (l *Library) Load() error                        // missing file = empty; bad file = backed up to path+".bak", error returned, library empty
func (l *Library) List() []Followed                   // copies, in follow order
func (l *Library) Get(url string) (Followed, bool)
func (l *Library) Follow(f Followed) error            // sets FollowedAt now when zero; replaces an entry with the same key; saves
func (l *Library) Unfollow(url string) error
func (l *Library) Update(url string, fn func(*Followed)) error // no-op when not followed; saves
func (l *Library) MarkSeen(url, title string, numbers []float64) error // seen[key] = {title, len, max}; followed entry's Chapters/Latest/New/CheckedAt too; saves
func (l *Library) MarkSaved(url string, upTo float64, numbers []float64) error // SavedUpTo = max(old, upTo), LastSaved now, New recomputed from numbers; no-op when not followed
func (l *Library) SeenOf(url string) (Seen, bool)
func urlKey(raw string) string                         // trim, lower scheme+host, strip "www.", drop trailing "/"
func hostOf(raw string) string                         // host without "www.", "" when it doesn't parse
func newAbove(numbers []float64, savedUpTo float64) int
```

- [ ] Write `library_test.go`: `TestURLKeyNormalises` (`https://WWW.Example.com/manga/x/` == `https://example.com/manga/x`), `TestFollowUnfollowPersists` (new Library on the same path lists it, then not), `TestMarkSeenUpdatesCountsAndNew` (follow with SavedUpTo 3, MarkSeen with [1,2,3,4,5] → Chapters 5, Latest 5, New 2), `TestMarkSavedNeverGoesBack` (MarkSaved 5 then 4 keeps 5; New recomputed), `TestSeenIsCappedAt500`, `TestBadFileIsBackedUp` (write garbage, Load returns error, `.bak` exists, Follow then writes a fresh file).
- [ ] Run: `cd gui && GOTOOLCHAIN=auto CGO_LDFLAGS="-framework UniformTypeIdentifiers" go test -tags desktop,production,nodynamic -run 'Library|URLKey|MarkS|Follow|Seen|BadFile' ./...` → FAIL (undefined).
- [ ] Implement `library.go` and `writeJSONAtomic`; make `saveOptions` call it.
- [ ] Run the same → PASS. Run the whole gui Go suite → PASS.
- [ ] Commit: `gui: a library of followed series on disk`.

### Task 2: The controller knows the library and records loads

**Files:**
- Modify: `gui/controller.go` (field `library *Library`; `NewController` creates `NewLibrary("")`; `SetLibrary(l *Library)`; in `load()` after a successful series: `c.library.MarkSeen(url, series.Title, numbers)`), `gui/main.go` (`newSession` builds the library at `filepath.Join(dir, "manga-downloader", "library.json")`, loads it, warns on error, `ctrl.SetLibrary`), `gui/controller_test.go`.

**Interfaces (produces):** `func (c *Controller) Library() *Library`; `func chapterNumbers(chapters grabber.Filterables) []float64`.

- [ ] Test `TestLoadMarksTheSeriesSeen`: `newTestController` with 5 chapters, `c.Load(url)`, then `c.Library().SeenOf(url)` has Chapters 5 and the title; `TestLoadUpdatesAFollowedSeries`: follow the url with SavedUpTo 3 first, load, entry has New 2 and CheckedAt set.
- [ ] Run → FAIL. Implement. Run → PASS. Commit: `gui: loads record the series' chapter count in the library`.

### Task 3: Finished runs move the saved mark

**Files:**
- Modify: `gui/controller.go`: `runStats` gains `maxSaved float64`; `Packed` (success), `Downloaded` and `VolumePacked` (success → `v.LastNumber()`) raise it; the end of `run()` calls `c.library.MarkSaved(url, maxSaved, numbers)` when `maxSaved > 0` (outside the lock, after `c.emit()` ordering: compute under lock, call after unlock).
- Test in `controller_test.go`: `TestRunRaisesTheSavedMark` (follow, load 5 chapters, select 1–3, start, wait, entry SavedUpTo 3, New 2, LastSaved set); `TestCancelledRunBeforeAnythingSavedKeepsTheMark` (gate the site, start, cancel at once, SavedUpTo stays 0). Use the existing gated `fakeSite` pattern from `newTestController`.
- [ ] RED → implement → GREEN → commit: `gui: a finished run raises the library's saved mark`.

### Task 4: Checking the followed series for new chapters

**Files:**
- Modify: `gui/controller.go`:
  - fields `checking bool`, `checkDone, checkTotal int`, `checkCancel context.CancelFunc`; helper `busyLocked() bool { return c.loading || c.running || c.checking }` replacing every `c.loading || c.running` guard (BeginLoad, ConfigureLoad, Configure, Clear, SetSelected*, SetAll, SelectRange, SetVolumes, Start).
  - `func (c *Controller) CheckLibrary(ctx context.Context) error`: refused with `errBusy`; marks checking; for each followed entry with Downloadable (skip others): `c.fetchNumbers(url, settingsCopy)`; on success `library.MarkSeen` + clear Error; on failure `library.Update` sets Error; `checkDone++`, emit; stops early when `ctx.Err() != nil`. Ends with checking false, emit.
  - `func (c *Controller) fetchNumbers(url string, s *grabber.Settings) (title string, numbers []float64, err error)`: `c.closing.Wait()`, `browser.SetVisible`, `browser.Hold`, `c.identify`, `core.Fetch`, `listing.Series(s)`; never touches `c.series`, `c.listings` or the filters.
  - `func (c *Controller) CancelCheck()`; getters `Checking() (on bool, done, total int)`.
- Tests: `TestCheckLibraryUpdatesEveryFollowedSeries` (two followed urls served by the fake site, one of them loaded and with a selection: after the check both entries have Chapters set, the loaded series and its selection are unchanged); `TestCheckLibraryGoesOnPastAFailure` (identify returns nil for the first url: it gets an Error, the second is updated); `TestCheckIsRefusedWhileRunning` and `TestLoadIsRefusedWhileChecking` (gate).
- [ ] RED → implement → GREEN → commit: `gui: check every followed series for new chapters`.

### Task 5: Download new, one series or all

**Files:**
- Modify: `gui/controller.go`: `func (c *Controller) SelectAbove(number float64) int` (selects chapters with GetNumber() > number, returns the count; refused while busy as SetAll). Fields for the batch: `batch *batchInfo{active bool; done, total int; current string; cancel context.CancelFunc}`; getter `Batch() BatchInfo`.
- Modify: `gui/app.go`:
  - `func (a *App) DownloadNew(url string) error`: `BeginLoad(url)`, then a goroutine: `FinishLoad`; on error stop; `SelectAbove(entry.SavedUpTo)`; if 0 log "nothing new" and stop; `Start()`. Emits `chapters` as `finishLoad` does (factor `finishLoad` into `loadAndThen(url, func())`).
  - `func (a *App) DownloadAllNew() error`: refused while busy or a batch runs; goroutine over `Library().List()` entries with `New > 0 && Downloadable`: for each, the DownloadNew steps synchronously, then wait until `!Running()` (poll 50ms, stop on ctx); batch done/total/current emitted along; `CancelBatch()` cancels the ctx and the current run (`Cancel()`).
- Tests (`app_test.go`): `TestDownloadNewSelectsAboveTheMarkAndStarts` (follow with SavedUpTo 2 of 5, DownloadNew, wait for the summary: Saved 3, entry SavedUpTo 5, New 0); `TestDownloadAllNewRunsEachSeriesInTurn` (two followed, both new; after it the summary's series is the second and both entries have New 0); `TestDownloadNewWithNothingNewLogsAndStops`.
- [ ] RED → implement → GREEN → commit: `gui: download the new chapters of one series, or of every followed one`.

### Task 6: Bind it: DTOs, App methods, bindings

**Files:**
- Modify: `gui/app.go`: `State` gains `Library LibraryDTO json:"library"`; `HitDTO` gains `Chapters int json:"chapters"` and `Followed bool json:"followed"` (from `Library().SeenOf` / `Get`); new bound methods `Follow(f FollowedDTO) error` (maps to `Followed`, `Site` from `hostOf`, `Downloadable` as given, and when it's the loaded series fills Chapters/Latest/New from the loaded list), `Unfollow(url string) error`, `CheckLibrary() error` (goroutine with `a.updateCtx`), `CancelCheck()`, `DownloadNew(url) error`, `DownloadAllNew() error`, `CancelBatch()`.
- New file `gui/librarydto.go`:
  ```go
  type FollowedDTO struct { URL, Title, Site string; Downloadable bool; Chapters int; Latest, SavedUpTo float64; New int; FollowedAt, CheckedAt, LastSaved string; Error string } // json camelCase, times RFC 3339 or ""
  type LibraryDTO struct { Followed []FollowedDTO `json:"followed"`; Checking bool; CheckDone, CheckTotal int; Batch BatchDTO }
  type BatchDTO struct { Active bool; Done, Total int; Current string }
  func libraryDTO(l *Library, checking bool, done, total int, batch BatchInfo) LibraryDTO
  ```
- Tests: `TestStateCarriesTheLibrary`, `TestSearchHitsCarryKnownCounts` (load the fake url, then a Search over a store whose hit has that url → Chapters 5, Followed after Follow).
- [ ] RED → implement → GREEN.
- [ ] Regenerate bindings: `cd gui && GOTOOLCHAIN=auto PATH="$HOME/go/bin:$PATH" CGO_LDFLAGS=… wails generate module`, then `git checkout frontend/wailsjs/runtime`.
- [ ] Commit: `gui: the library, follow and download-new reach the window`.

### Task 7: Frontend types, api, mock, pure helpers

**Files:**
- Modify: `gui/frontend/src/types.ts` (`Followed`, `LibraryInfo`, `State.library`, `Hit.chapters`, `Hit.followed`), `api.ts` (`follow(f: Followed)`, `unfollow(url)`, `checkLibrary()`, `cancelCheck()`, `downloadNew(url)`, `downloadAllNew()`, `cancelBatch()`), `mock.ts` (an in-memory library seeded with three followed series — One Piece (the mock series, SavedUpTo 1182 so 12 are new), Berserk (up to date, search only false, chapters 379), Kingdom (search only) — `&nolibrary` starts empty; `checkLibrary` simulates 600 ms per series; `downloadNew` loads, selects above the mark and runs; hits carry `chapters` for the mock series).
- Create: `gui/frontend/src/library.ts` with pure helpers and `library.test.ts`:
  ```ts
  export function cardStatus(f: Followed): { text: string; tone: 'new' | 'ok' | 'muted' | 'warn' | 'danger' }
  export function cardAction(f: Followed): { label: string; kind: 'new' | 'none' | 'open' }   // "Download 2 new" | "Download all 379" | "Nothing new" | "Open on the site"
  export function sortFollowed(list: Followed[], by: 'new' | 'title' | 'saved'): Followed[]
  export function librarySummary(info: LibraryInfo, now: Date): string      // "5 series you follow · 3 new chapters · checked 5 min ago"
  export function newIndices(chapters: Chapter[], savedUpTo: number): number[]
  export function chapterMark(number: number, f: Followed | null): 'new' | 'saved' | ''
  export function isLinkQuery(text: string): boolean   // isSeriesUrl
  ```
- [ ] Tests for each helper (RED), implement (GREEN), `pnpm typecheck`. Commit: `gui frontend: library types, api and helpers`.

### Task 8: The top bar and the results page

**Files:**
- Create: `TopBar.tsx` (+ test): brand (Mascot + name, a button to go home), the search box (`query`, placeholder "Search 462 sites, or paste a link"; a link loads after `AUTO_LOAD_DELAY_MS` via `shouldAutoLoad`, or on Enter; a clear ×), *Check for new chapters* (hidden when the library is empty; shows "Checking 2 of 5…" + Cancel while checking), the Settings gear. Props: `state, query, onQuery, onLoad, onHome, onCheck, onCancelCheck, onSettings, busy`.
- Create: `ResultsPanel.tsx` (+ test, replacing `SearchPanel.test.ts`): toolbar (site select from `siteGroups`, "N results", catalog line, Update); a table of rows: title (+ "18+", "search only" tags), site, language, chapters (`—` with title "Open it once to see its chapters" when 0), a follow star (`aria-pressed`); the search-only note (from `SearchPanel`, same `BlockedHit` rule); empty states. Props as `SearchPanel` plus `onFollow(hit)`, `onUnfollow(url)`.
- Delete: `SourceBar.tsx`, `SourceBar.test.ts`, `SearchPanel.tsx`, `SearchPanel.test.ts`. Move `FilterField` into `FilterField.tsx`.
- Tests: TopBar shows the check button only with a library, shows progress while checking; ResultsPanel lists chapters, "—" for unknown, a pressed star for a followed hit, keeps the blocked note rule, `busy` disables picking but not the star.
- [ ] RED → implement → GREEN → commit: `gui frontend: top bar and results page`.

### Task 9: The library home

**Files:**
- Create: `LibraryPanel.tsx` (+ test): header, sort select, *Download all new · M* (disabled while busy or none new; "Downloading 2 of 3 · Title" + Cancel while a batch runs), the card grid (`cardStatus`, `cardAction`), the unfollow bookmark (`aria-label="Following X, click to unfollow"`), the empty state, the foot note. Props: `state, chapters?, busy, onOpen(url), onDownloadNew(url), onDownloadAllNew, onCancelBatch, onUnfollow(url), onOpenURL(url), sort, onSort`.
- Styles: `.library`, `.lib-head`, `.lib-grid`, `.card`, `.card.fresh`, `.cover`, `.pin`, `.card-status` in `styles.css` (tokens only).
- Tests: a card per followed series with its status and button label; the empty state; the batch line.
- [ ] RED → implement → GREEN → commit: `gui frontend: the library home`.

### Task 10: The series page

**Files:**
- Create: `SeriesPage.tsx`: breadcrumb; `ChapterPanel` (existing) in the middle; `SeriesAside.tsx` on the right: cover letter, title, site link (`openURL`), Follow/Following (`aria-pressed`), the facts list (chapters, new, saved up to, last downloaded; only for a followed series), `SaveOptions` and the dock.
- Modify: `ChapterPanel.tsx`: new props `followed: Followed | null`, `onSelectNew()`; the chip row (*New · K*, *All · N*) above the toolbar, the filter selectors (from `FilterField.tsx`, props `onPick`, `onOptions`, `onError`) in that row; rows get `chapterMark` tags (`<span class="tag new">new</span>` / `<span class="tag saved">saved</span>`).
- Modify: `OptionsPanel.tsx`: split into `SaveOptions` (folder, format, reading direction, pack as; exported) and the rest (`MoreOptions`: pages, advanced) so the aside and the settings drawer share them. `DownloadDock` is reused as the aside's foot.
- Tests: `SeriesAside.test.ts` (facts shown for a followed series, Follow button state, Download label); `ChapterPanel` chip test (New count and the mark on rows).
- [ ] RED → implement → GREEN → commit: `gui frontend: the series page with the aside`.

### Task 11: The strip, the drawers, and App.tsx

**Files:**
- Create: `ProgressStrip.tsx` (+ test): running → title, "chapter X · a of b done" (from `overallProgress`), bar, pct, Details, Cancel (uses the existing `DownloadDock` wording for cancelling); finished → `summaryHeadline`, Open folder, Details, dismiss; hidden otherwise.
- Create: `SettingsDrawer.tsx` (`MoreOptions` + the catalog line + Update) and `ActivityDrawer.tsx` (`ActivityPanel` in the `log-drawer` styling).
- Rewrite `App.tsx`: state `page: 'home' | 'series'`, `query`, `site`, `sort`, `settingsOpen`, `activityOpen`, `logOpen`, `blocked`, `copied`, `stripDismissed`; `useSearch` stays (driven by the top bar's query when it isn't a link); view = results when the query is a non-link, else page. Handlers: `openSeries(url)` (load unless loaded, `page = 'series'`), `onPick(hit)`, `follow`/`unfollow`, `downloadNew`, `downloadAllNew`, `check`. A run that starts shows the strip; the Activity tab logic goes.
- Styles: `.topbar`, `.page`, `.results`, `.series-page`, `.aside`, `.strip`, `.drawer`; remove `.source`, `.mode-switch`, `.tabs`, `.side` rules that nothing uses; keep `@media` narrow rules sane (the aside stacks under 1040px).
- Tests: an `App.test.ts` with the mock: home renders the mock's three cards; typing "one" shows results; the loaded scenario opens on the series page.
- [ ] RED → implement → GREEN; `pnpm typecheck && pnpm vitest run && pnpm build`. Commit: `gui frontend: library-first window`.

### Task 12: Docs, build, run, review

- [ ] Update `.agents/skills/gui/SKILL.md` ("Search mode" → "Pages", the library, `library.json`), `AGENTS.md` GUI pointers, README's GUI section and screenshot list note.
- [ ] `GOTOOLCHAIN=auto PATH="$HOME/go/bin:$PATH" make test/gui` → green; `make build/gui`; run `./manga-downloader-gui`, follow a series, check, download new, confirm the card updates.
- [ ] Review pass (fresh reviewer agent on the diff), fix, re-run tests, commit, push `catalog-search` and `master`.
