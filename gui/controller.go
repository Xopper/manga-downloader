// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/elboletaire/manga-downloader/catalog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	mdhttp "github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/elboletaire/manga-downloader/ranges"
)

// maxLogLines caps the log so a long download can't grow it without bound
const maxLogLines = 2000

// errBusy is returned when an action is refused because a series is loading
// or the library is being checked. A download running refuses nothing but
// the library check (see queue.go): it works on its own copies.
var errBusy = errors.New("a series is loading or the library is being checked")

// LogLevel is the severity of a log line
type LogLevel int

const (
	LogInfo LogLevel = iota
	LogWarn
	LogError
)

// LogLine is one entry of the log
type LogLine struct {
	Level LogLevel
	Text  string
}

// JobState is where a chapter's download is
type JobState int

const (
	// JobDownloading is a chapter whose pages are being fetched
	JobDownloading JobState = iota
	// JobArchiving is a chapter whose pages are all downloaded and being packed
	JobArchiving
	// JobDone is a chapter that was packed (or, in bundle mode, the bundle)
	JobDone
	// JobFailed is a chapter that couldn't be fetched, downloaded or packed
	JobFailed
	// JobSkipped is a chapter whose pages were all excluded by --skip-pages
	JobSkipped
	// JobFetching is a chapter whose page list is being fetched, before any
	// page is known
	JobFetching
)

// Job is the progress of one chapter. In bundle mode there's a single Job for
// the whole bundle, whose Total counts each page's download and archive steps
// (core.DownloadWeight and core.ArchiveWeight, as the CLI's bar does), and with
// volumes one per volume, whose Done is core.VolumeProgress out of a fixed
// Total of volumeScale (its pages aren't known until it's packed).
type Job struct {
	Title string
	// Tags is the release's language and group (" [en] [Alpha]"), kept apart
	// from Title so a narrow row truncates the title and keeps them
	Tags   string
	Number float64
	Total  int64
	Done   int64
	State  JobState
	Reason string
	// Status is the bundle's or the volume's phase ("Downloading
	// <chapter>", "Creating bundle..."); unused for single chapters
	Status string
	// Volume is set on the row of a volume
	Volume bool
}

// RunInfo describes the run in progress, or the last one
type RunInfo struct {
	// ID numbers the job (see queue.go), 0 before the first run
	ID int
	// URL and Title are the series the run downloads from
	URL   string
	Title string
	// Fresh is a job of a followed series' new chapters, whose list is
	// fetched when it starts
	Fresh bool
	// Chapters is how many chapters the run was started with
	Chapters int
	Bundle   bool
	// Volumes is set when the run packs one file per volume
	Volumes bool
	// Rows is how many progress rows the run ends up with: one per chapter,
	// one for a bundle, or one per volume plus one per chapter outside them
	Rows int
}

// Summary is what a finished run produced
type Summary struct {
	Dir       string
	Cancelled bool
	// Saved counts chapters written to disk (or, in bundle mode, kept for the
	// bundle, and with volumes, kept for their volume)
	Saved int
	// Volumes counts the volume files written
	Volumes int
	// Bundled is set in bundle mode, where Bundle is the bundle's filename
	// (empty when none was written)
	Bundled  bool
	Bundle   string
	Skipped  int
	Failures []string
	// Err is a bundle packing error
	Err error
}

// runStats collects what the events report during a run, for the summary
type runStats struct {
	saved    int
	volumes  int
	skipped  int
	failures []string
	bundle   string
	err      error
	// savedNumbers is the numbers of the chapters that reached the disk;
	// downloaded is the bundle's chapters kept in memory, which count only
	// once the bundle is written
	savedNumbers []float64
	downloaded   []float64
	// savedFiles is the file each saved chapter went to, Dir and Format
	// filled in when the run ends
	savedFiles []SavedFile
}

// Controller holds the state of the app and runs the core flow. It knows
// nothing about the window toolkit: the window reads its state through the
// getters and is told about changes through notify, which is called from any
// goroutine and must not call back into the controller.
//
// Every mutation happens under mu; getters return copies, so the view never
// shares memory with the event goroutines.
type Controller struct {
	notify func()
	// onDisk answers whether a saved chapter's file is still there
	onDisk presence
	// identify finds the site for a URL; core.Identify, replaced in tests
	identify func(url string, s *grabber.Settings) (grabber.Site, []error)
	// closeBrowser is browser.Close, replaced in tests
	closeBrowser func()
	// needsVisible is browser.NeedsVisible, and bringToFront
	// browser.BringToFront, replaced in tests
	needsVisible func(url string) bool
	bringToFront func()
	// closing tracks a browser close running in the background (see Clear),
	// which loads wait for rather than meet a browser going away
	closing sync.WaitGroup
	// filterChange is set by BeginLoad when a load changes only the filters of
	// the URL loaded last; load() reads and clears it (see listingFor)
	filterChange bool

	mu sync.Mutex
	// settings is the one pointer the site and core read their settings
	// through, so it's replaced (not mutated) by Load, and mutated by
	// Configure only while nothing is running
	settings *grabber.Settings

	url      string
	series   *core.Series
	loading  bool
	selected []bool
	// hasDuplicates is core.HasDuplicateChapterNumbers for the loaded list, and
	// hasGroupDuplicates is core.HasGroupDuplicates
	hasDuplicates      bool
	hasGroupDuplicates bool
	// loadedLanguage and loadedScanlator are the filters the list was fetched
	// with, which Configure can make stale
	loadedLanguage  string
	loadedScanlator string
	// listings are the unfiltered listings of the session's loads, which a
	// filter change is served from (see listingFor)
	listings listingCache
	// languages and scanlators are the filter options of the site of the last
	// load, nil for each one it doesn't list. They outlive a failed load, so
	// its filters can still be changed.
	languages  *grabber.FilterOptions
	scanlators *grabber.FilterOptions

	// loadErr is the last load's error, which the view shows under the URL
	loadErr error
	// volumesText is the Volumes box, the volumes a run in volume mode packs:
	// --volumes' syntax, "" meaning the site's own. siteVolumes is the site's
	// volumes in the same syntax, what the box starts with on each load.
	volumesText string
	siteVolumes string
	// update is the newer release the update check found, nil when none
	update *Update
	// catalog is the store Search looks in, nil until SetCatalog; catalogURL
	// is where RefreshCatalog downloads from (catalog.BaseURL, replaced in
	// tests); catalogUpdating, catalogErr and catalogGen are its standing
	catalog         *catalog.Store
	catalogURL      string
	catalogUpdating bool
	catalogErr      string
	catalogGen      uint64
	// catalogInfo is CatalogInfo's answer for catalogGen, nil until asked
	catalogInfo *CatalogInfo
	// attention is the browser window waiting for the user (a check to pass
	// there), nil when none is or the user dismissed it
	attention *browser.Attention
	// library is the followed series and what the app saw of every series it
	// loaded (see library.go); memory-only until SetLibrary
	library *Library
	// covers keeps the followed series' covers (see covers.go); nil until
	// SetCovers, and no cover is fetched meanwhile
	covers *coverStore
	// lists keeps the chapter list last seen of every series loaded (see
	// lists.go), what a load shows until the site answers; nil until
	// SetLists, and nothing is saved or shown meanwhile
	lists *listStore
	// listGen counts the times the loaded list changed (a load's result, a
	// saved list shown, a clear), so the view knows when to send it again
	listGen uint64
	// checking is set while CheckLibrary fetches the followed series, with
	// checkDone of checkTotal done; checkCancel ends it early
	checking    bool
	checkDone   int
	checkTotal  int
	checkCancel context.CancelFunc
	// selGen and logGen count changes to the selection and the log, so a view
	// that pushes state can skip re-sending either when it hasn't moved
	selGen uint64
	logGen uint64

	// active is the download going, nil when none; last the one that ended
	// last, what the window shows until the next starts; queue the jobs
	// waiting their turn, first to start first (see queue.go); nextID
	// numbers them
	active *run
	last   *run
	queue  []*run
	nextID int
	log    []LogLine
}

// NewController returns a controller that works on its own copy of settings
// and routes the library's warnings into its log.
func NewController(settings *grabber.Settings, notifyFn func()) *Controller {
	if notifyFn == nil {
		notifyFn = func() {}
	}
	c := &Controller{
		notify:       notifyFn,
		identify:     core.Identify,
		closeBrowser: browser.Close,
		needsVisible: browser.NeedsVisible,
		bringToFront: browser.BringToFront,
		settings:     settings,
		catalogURL:   catalog.BaseURL(),
		library:      NewLibrary(""),
	}
	notice.SetHandler(func(level notice.Level, msg string) {
		lvl := LogInfo
		if level == notice.LevelWarn {
			lvl = LogWarn
		}
		c.addLog(lvl, msg)
	})
	browser.SetAttentionHandler(c.onAttention)
	return c
}

// onAttention keeps the browser's attention requests, which the view shows
// over the window until the page renders or its load ends
func (c *Controller) onAttention(a browser.Attention) {
	c.mu.Lock()
	if a.Active {
		c.attention = &a
		c.appendLogLocked(LogWarn, a.Message())
	} else {
		c.attention = nil
	}
	c.mu.Unlock()
	c.emit()
}

// BrowserAlert is the message of the browser window waiting for the user, ""
// when none is
func (c *Controller) BrowserAlert() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attention == nil {
		return ""
	}
	return c.attention.Message()
}

// DismissBrowserAlert hides the alert; the window stays open, and the load
// goes on waiting for it
func (c *Controller) DismissBrowserAlert() {
	c.mu.Lock()
	had := c.attention != nil
	c.attention = nil
	c.mu.Unlock()
	if had {
		c.emit()
	}
}

// ShowBrowser brings the browser window to the front again
func (c *Controller) ShowBrowser() {
	c.bringToFront()
}

// BrowserVisibleAuto reports whether the browser opens visible on its own for
// the site of the URL loaded: the site needs it (declared by its grabber, or
// escalated earlier in this session), whatever the option says
func (c *Controller) BrowserVisibleAuto() bool {
	c.mu.Lock()
	url := c.url
	c.mu.Unlock()
	return url != "" && c.needsVisible(url)
}

// emit tells the view something changed. Callers must not hold mu.
func (c *Controller) emit() { c.notify() }

// addLog appends a line and notifies. Callers must not hold mu.
func (c *Controller) addLog(level LogLevel, text string) {
	c.mu.Lock()
	c.appendLogLocked(level, text)
	c.mu.Unlock()
	c.emit()
}

func (c *Controller) appendLogLocked(level LogLevel, text string) {
	c.logGen++
	c.log = append(c.log, LogLine{Level: level, Text: text})
	if len(c.log) > maxLogLines {
		c.log = c.log[len(c.log)-maxLogLines:]
	}
}

// Configure applies fn to the settings. It's refused while a series loads or
// a download runs, so the goroutines reading the settings never see a write.
func (c *Controller) Configure(fn func(*grabber.Settings)) error {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return errBusy
	}
	fn(c.settings)
	c.mu.Unlock()
	c.emit()
	return nil
}

// Settings returns a copy of the current settings
func (c *Controller) Settings() grabber.Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return *c.settings
}

// Load identifies the site for url and fetches its title and chapter list.
// It replaces any loaded series and its selection.
func (c *Controller) Load(url string) error {
	if err := c.BeginLoad(url); err != nil {
		return err
	}
	return c.FinishLoad(url)
}

// BeginLoad marks a load as started, synchronously, so the view can disable
// its controls before the slow part (FinishLoad) runs on another goroutine.
func (c *Controller) BeginLoad(url string) error {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return errBusy
	}
	c.beginLoadLocked(url)
	c.mu.Unlock()
	c.emit()
	return nil
}

// ConfigureLoad is Configure then BeginLoad in one step, so the window never shows a
// picked filter as not applied.
func (c *Controller) ConfigureLoad(fn func(*grabber.Settings), url string) error {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return errBusy
	}
	fn(c.settings)
	c.beginLoadLocked(url)
	c.mu.Unlock()
	c.emit()
	return nil
}

// beginLoadLocked starts a load of url with the settings as they are now. The
// caller holds mu and has checked that nothing is loading or running.
func (c *Controller) beginLoadLocked(url string) {
	c.loading = true
	c.loadErr = nil
	// a fresh settings pointer: the previous series keeps its own, and no
	// Configure can run while this load reads it
	s := new(grabber.Settings)
	*s = *c.settings
	c.settings = s
	// a load of the URL of the last one with other filters is a filter change:
	// it's served from the listings it can be (see listingFor). Any other load
	// is an explicit reload, which fetches.
	c.filterChange = url == c.url && (s.Language != c.loadedLanguage || s.Scanlator != c.loadedScanlator)
	if urlKey(url) != urlKey(c.url) {
		// another series: the loaded one goes now, so the window shows the
		// new one loading rather than the old one with the new name; its
		// filters go with it. The list saved from the last time the new
		// one loaded, when there's one, stands in until the site answers.
		if c.series != nil {
			c.listGen++
		}
		c.dropLoadedCoverLocked()
		c.series = nil
		c.selected = nil
		c.selGen++
		c.languages, c.scanlators = nil, nil
		c.showSavedLocked(url)
	} else if c.series == nil {
		// the same series with nothing up (its last load failed): its saved
		// list stands in all the same
		c.showSavedLocked(url)
	}
	c.url = url
}

// showSavedLocked puts the saved list of url up as the loaded series (title
// and chapters only: no site, nothing to download from), locked behind the
// load that fetches the real one. The caller holds mu.
func (c *Controller) showSavedLocked(url string) {
	if c.lists == nil {
		return
	}
	saved, ok := c.lists.Get(url)
	if !ok {
		return
	}
	chapters := saved.filterables()
	c.series = &core.Series{Title: saved.Title, Chapters: chapters}
	c.listGen++
	c.selected = make([]bool, len(chapters))
	c.hasDuplicates = core.HasDuplicateChapterNumbers(chapters)
	c.hasGroupDuplicates = core.HasGroupDuplicates(chapters)
	c.siteVolumes = core.SiteVolumes(chapters)
	c.volumesText = c.siteVolumes
}

// SetLists gives the controller the store of saved lists (see lists.go)
func (c *Controller) SetLists(s *listStore) {
	c.mu.Lock()
	c.lists = s
	c.mu.Unlock()
}

// ListGen counts the changes of the loaded list
func (c *Controller) ListGen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listGen
}

// saveList keeps the list of a series the site answered for, so the next
// load of it shows the list at once; a failure to write is logged
func (c *Controller) saveList(url, title string, chapters grabber.Filterables) {
	c.mu.Lock()
	lists := c.lists
	c.mu.Unlock()
	if lists == nil || len(chapters) == 0 {
		return
	}
	if err := lists.Put(url, title, chapters); err != nil {
		c.addLog(LogWarn, "could not save the chapter list: "+err.Error())
	}
}

// abortLoad ends a load that panicked, so the window isn't left locked
// waiting for it. It's the same outcome as FinishLoad failing.
func (c *Controller) abortLoad(err error) {
	c.mu.Lock()
	c.loading = false
	c.loadErr = err
	c.series = nil
	c.selected = nil
	c.selGen++
	c.listGen++
	c.mu.Unlock()
	c.emit()
}

// FinishLoad does the work BeginLoad started. Call it exactly once after a
// successful BeginLoad.
func (c *Controller) FinishLoad(url string) error {
	c.mu.Lock()
	s := c.settings
	c.mu.Unlock()

	// a browser still closing after Clear would fail the load
	c.closing.Wait()
	// the browser's visibility is read when it starts, and loading a site
	// may be what starts it
	browser.SetVisible(s.BrowserVisible)
	release := browser.Hold()
	defer release()
	err := c.load(url, s)

	c.mu.Lock()
	c.loading = false
	c.loadErr = err
	var title string
	var numbers []float64
	var chapters grabber.Filterables
	if err != nil {
		c.series = nil
		c.selected = nil
		c.selGen++
	} else {
		title = c.series.Title
		chapters = c.series.Chapters
		numbers = chapterNumbers(chapters)
	}
	c.listGen++
	c.mu.Unlock()
	if err == nil {
		// the library remembers every series loaded: its count feeds the
		// search results, and a followed entry learns what's new
		if lerr := c.library.MarkSeen(url, title, numbers); lerr != nil {
			c.addLog(LogWarn, "could not save the library: "+lerr.Error())
		}
		c.saveList(url, title, chapters)
		c.ensureCover(url, false)
	}
	c.emit()
	return err
}

// chapterNumbers lists the numbers of a chapter list, in its order
func chapterNumbers(chapters grabber.Filterables) []float64 {
	out := make([]float64, 0, len(chapters))
	for _, ch := range chapters {
		out = append(out, ch.GetNumber())
	}
	return out
}

// busyLocked reports whether a load or a library check is on: what refuses
// the actions that would race it. A download running is not busy: it works
// on its own copies (see queue.go), so the series, the selection and the
// options can change under it. The caller holds mu.
func (c *Controller) busyLocked() bool {
	return c.loading || c.checking
}

func (c *Controller) load(url string, s *grabber.Settings) error {
	// a load fetches: a page remembered from the last one (see
	// mdhttp.PageMemory) must not stand in for what the site says now
	mdhttp.ResetPageMemory()
	// the filter change BeginLoad recorded is taken here, so it never bleeds
	// into a later load. A Refilterable site of the loaded series is reused on a
	// filter change that isn't cached, as it filters what it already fetched.
	c.mu.Lock()
	filterChange := c.filterChange
	c.filterChange = false
	var reuse grabber.Site
	if filterChange && c.series != nil {
		if _, ok := c.series.Site.(grabber.Refilterable); ok {
			reuse = c.series.Site
		}
	}
	c.mu.Unlock()

	listing, site, err := c.listingFor(url, s, reuse, filterChange)
	if site == nil {
		c.mu.Lock()
		c.languages, c.scanlators = nil, nil
		c.mu.Unlock()
		return err
	}

	var series *core.Series
	if err == nil {
		series, err = listing.Series(s)
	}
	if err != nil {
		// the reload gets the listing of its own filters (dropUnmatchedFilters'
		// settings): cached when the listing of the URL is, else fetched again
		// (the failed site may have read its filters), unless it's Refilterable
		if reload, dropLanguage := c.dropUnmatchedFilters(site, s, err); reload {
			// the site keeps the pointer it's identified with, and Configure
			// changes c.settings: a copy would leave the download ignoring
			// later option changes. So the language is cleared in place just
			// for the retry (nothing else touches s while a load runs).
			typed := s.Language
			if dropLanguage {
				c.mu.Lock()
				s.Language = ""
				c.mu.Unlock()
			}
			var retryReuse grabber.Site
			if _, ok := site.(grabber.Refilterable); ok {
				retryReuse = site
			}
			retry, retrySite, retryErr := c.listingFor(url, s, retryReuse, true)
			if retrySite != nil {
				site, err, series = retrySite, retryErr, nil
				if err == nil {
					series, err = retry.Series(s)
				}
			}
			if dropLanguage {
				c.mu.Lock()
				s.Language = typed
				c.mu.Unlock()
			}
		}
	}

	// the options are kept whether or not the load worked; a series already
	// carries them, and asking the site again costs requests
	var languages, scanlators *grabber.FilterOptions
	if series != nil {
		languages, scanlators = series.Languages, series.Scanlators
	} else {
		languages, scanlators = core.FilterOptionsOf(site, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.languages, c.scanlators = languages, scanlators
	// a value typed in another case takes the site's spelling, so the options
	// saved, the loaded filters and the selector all agree on it
	s.Language = listedSpelling(s.Language, languages)
	s.Scanlator = listedSpelling(s.Scanlator, scanlators)
	// a language dropped for this load on a site that lists none is recorded as
	// typed: no selector shows it, and the field would read as stale otherwise
	c.loadedLanguage = s.Language
	c.loadedScanlator = s.Scanlator
	if err != nil {
		return err
	}
	c.series = series
	c.selGen++
	c.selected = make([]bool, len(series.Chapters))
	c.hasDuplicates = core.HasDuplicateChapterNumbers(series.Chapters)
	c.hasGroupDuplicates = core.HasGroupDuplicates(series.Chapters)
	c.siteVolumes = core.SiteVolumes(series.Chapters)
	c.volumesText = c.siteVolumes
	c.appendLogLocked(LogInfo, fmt.Sprintf("Loaded %s: %d chapters", series.Title, len(series.Chapters)))
	return nil
}

// listingFor returns the unfiltered listing of url and its site, nil when no site
// matches. With useCache, a listing cached for the URL, language (and group for a
// ScanlatorLister) is reused. The site comes back even on a fetch error.
func (c *Controller) listingFor(url string, s *grabber.Settings, reuse grabber.Site, useCache bool) (*core.Listing, grabber.Site, error) {
	c.mu.Lock()
	// the site of the running job reads that job's settings until it ends:
	// a load of the same series meanwhile gets a site of its own
	if c.siteInUseLocked(reuse) {
		reuse = nil
	}
	if useCache {
		if listing := c.listings.get(url, s); listing != nil && !c.siteInUseLocked(listing.Site) {
			c.mu.Unlock()
			setSiteSettings(listing.Site, s)
			return listing, listing.Site, nil
		}
	} else {
		// an explicit reload replaces what the URL had cached, even when it fails
		c.listings.drop(url, s)
	}
	c.mu.Unlock()

	site := reuse
	if site != nil {
		setSiteSettings(site, s)
	} else {
		var siteErrs []error
		site, siteErrs = c.identify(url, s)
		for _, err := range siteErrs {
			c.addLog(LogWarn, "site check: "+err.Error())
		}
		if site == nil {
			return nil, nil, errors.New("site not recognised")
		}
	}

	listing, err := core.Fetch(site)
	if err != nil {
		return nil, site, err
	}
	c.mu.Lock()
	c.listings.put(url, s, listing)
	c.mu.Unlock()

	return listing, site, nil
}

// setSiteSettings points a site at the settings it reads from, as the
// settings pointer changes on each load
func setSiteSettings(site grabber.Site, s *grabber.Settings) {
	if su, ok := site.(interface{ SetSettings(*grabber.Settings) }); ok {
		su.SetSettings(s)
	}
}

// dropUnmatchedFilters handles a load that found nothing for a filter the
// series doesn't have, which the window would otherwise just fail on: each
// such filter is reset to its fallback (see unmatchedFallback) and logged, and
// whether to reload is returned, and whether to do so without the language. It
// only looks at errors that mean "no chapters for this filter": a network
// failure keeps the value.
//
// A language on a site that lists none is dropped for this load only: the
// saved language stays, for the other sites that take it. The caller clears it
// on the shared settings for the reload only and puts it back. Such a site can't say which languages it has, so only
// a NoChaptersError with no scanlator to blame is taken as the language's
// doing: a scanlator the site lists, or none, can't have emptied the list. A
// typed scanlator it doesn't list is reset instead, so the language is kept.
func (c *Controller) dropUnmatchedFilters(site grabber.Site, s *grabber.Settings, err error) (reload, dropLanguage bool) {
	var noChapters *core.NoChaptersError
	var chaptersErr *core.ChaptersError
	var unavailable *grabber.UnavailableFilterError
	// a site that picks the filter while fetching the title fails there
	// instead, with an error of its own
	rejected := errors.As(err, &unavailable)
	if !errors.As(err, &noChapters) && !errors.As(err, &chaptersErr) && !rejected {
		return false, false
	}

	languages, scanlators := core.FilterOptionsOf(site, err)
	// s is the settings the controller holds, so the typed values are read
	// before they're overwritten
	typedLanguage, typedScanlator := s.Language, s.Scanlator
	language, languageReset := unmatchedFallback(typedLanguage, languages)
	scanlator, scanlatorReset := unmatchedFallback(typedScanlator, scanlators)
	scanlatorCleared := typedScanlator == "" || typedScanlator == grabber.FilterAll || listsValue(scanlators, typedScanlator)
	languageDropped := !languageReset && !listsValues(languages) && (noChapters != nil || (rejected && unavailable.Filter == "language")) &&
		typedLanguage != "" && typedLanguage != grabber.FilterAll && scanlatorCleared
	if !languageReset && !scanlatorReset && !languageDropped {
		return false, false
	}

	c.mu.Lock()
	if languageReset {
		c.settings.Language = language
	}
	if scanlatorReset {
		c.settings.Scanlator = scanlator
	}
	c.mu.Unlock()
	if languageReset {
		c.addLog(LogInfo, fallbackMessage("language", typedLanguage, language))
	}
	if scanlatorReset {
		c.addLog(LogInfo, fallbackMessage("scanlation group", typedScanlator, scanlator))
	}

	if languageDropped {
		c.addLog(LogInfo, fallbackMessage("language", typedLanguage, ""))
	}

	return true, languageDropped
}

// listsValue reports whether the site offers value for a filter, ignoring case
func listsValue(options *grabber.FilterOptions, value string) bool {
	if !listsValues(options) {
		return false
	}
	for _, v := range options.Values {
		if strings.EqualFold(v, value) {
			return true
		}
	}

	return false
}

// unmatchedFallback is what a typed filter value falls back to when the series
// doesn't offer it: "all" where the site takes it, the site's Current value
// otherwise, and no filter ("") when it has neither. reset is false when
// there's nothing to fall back from: no value, "all" itself, a site that lists
// nothing, or a value the series does offer.
func unmatchedFallback(value string, options *grabber.FilterOptions) (fallback string, reset bool) {
	if value == "" || value == grabber.FilterAll || !listsValues(options) {
		return "", false
	}
	if listsValue(options, value) {
		return "", false
	}
	if options.All {
		return grabber.FilterAll, true
	}

	return options.Current, true
}

// listsValues reports whether the site offers any value for a filter: the
// selector isn't shown otherwise (see newFilter), so nothing can be matched
func listsValues(options *grabber.FilterOptions) bool {
	return options != nil && len(options.Values) > 0
}

// fallbackMessage is the log line for a filter that was reset, naming what
// the load went on with instead
func fallbackMessage(kind, value, fallback string) string {
	shown := fmt.Sprintf("%q", fallback)
	switch fallback {
	case grabber.FilterAll:
		shown = "all of them"
	case "":
		shown = "no filter"
	}

	return fmt.Sprintf("%s %q isn't in this series, loading %s instead", kind, value, shown)
}

// Filter is what a filter's selector offers for the loaded series
type Filter struct {
	// Values are the values the site lists for the filter
	Values []string
	// All is set when the site also takes "all" (every value, mixed)
	All bool
	// Selected is the value the loaded list shows: the one it was loaded with,
	// or else what the site shows by default (see selectedFilter)
	Selected string
}

// Filters returns the language and scanlator selectors: nil for a filter the
// site doesn't list, or when no site was identified
func (c *Controller) Filters() (language, scanlator *Filter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var chapters grabber.Filterables
	if c.series != nil {
		chapters = c.series.Chapters
	}

	language = newFilter(c.languages, selectedFilter(c.loadedLanguage, c.languages, commonLanguage(chapters)))
	scanlator = newFilter(c.scanlators, selectedFilter(c.loadedScanlator, c.scanlators, ""))

	return language, scanlator
}

// newFilter builds the selector of one filter, nil when the site doesn't list it
func newFilter(options *grabber.FilterOptions, selected string) *Filter {
	if options == nil || len(options.Values) == 0 {
		return nil
	}

	return &Filter{
		Values:   append([]string(nil), options.Values...),
		All:      options.All,
		Selected: selected,
	}
}

// listedSpelling returns the spelling the site lists value under when it
// matches one ignoring case ("delta" is "Delta"), value itself otherwise
func listedSpelling(value string, options *grabber.FilterOptions) string {
	if options == nil {
		return value
	}
	for _, v := range options.Values {
		if strings.EqualFold(v, value) {
			return v
		}
	}

	return value
}

// selectedFilter is the value the loaded list shows for one filter. A value
// the list was loaded with is what it shows, in the site's spelling. With none,
// the site's default shows: Current when the site has one, else the language
// every chapter shares (sites whose default is one language, like "en"), else
// every value when the site takes "all". uniform is commonLanguage, or "" for a
// filter whose chapters don't say.
func selectedFilter(loaded string, options *grabber.FilterOptions, uniform string) string {
	switch {
	case loaded != "" || options == nil:
		return listedSpelling(loaded, options)
	case options.Current != "":
		return options.Current
	case uniform != "":
		return uniform
	case options.All:
		return grabber.FilterAll
	}

	return ""
}

// commonLanguage returns the language every chapter reports, "" when they
// differ or don't report one
func commonLanguage(chapters grabber.Filterables) string {
	lang := ""
	for i, chap := range chapters {
		l, ok := chap.(interface{ GetLanguage() string })
		if !ok {
			return ""
		}
		if i == 0 {
			lang = l.GetLanguage()
		} else if l.GetLanguage() != lang {
			return ""
		}
	}

	return lang
}

// Clear forgets the loaded series: its URL, title, chapters, selection, the
// outcome of its load and the last run's results (the Activity tab), so the
// window starts from nothing. The options, the log and a download going or
// waiting stay. It's refused while a series loads or the library is checked.
func (c *Controller) Clear() error {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return errBusy
	}
	c.url = ""
	c.series = nil
	c.selected = nil
	c.selGen++
	c.listGen++
	c.hasDuplicates = false
	c.hasGroupDuplicates = false
	c.volumesText, c.siteVolumes = "", ""
	c.languages, c.scanlators = nil, nil
	c.loadedLanguage, c.loadedScanlator = "", ""
	// the sites of the cached listings may hold a browser: they go with the series
	c.listings = nil
	c.loadErr = nil
	c.attention = nil
	if c.active == nil {
		c.last = nil
	}
	// done with the series: its browser (and maybe window) can go, unless a
	// download is using it. It's closed in the background, as that can take
	// seconds; added under mu so a load that starts next is sure to wait for it.
	closeBrowser := c.active == nil && len(c.queue) == 0
	if closeBrowser {
		c.closing.Add(1)
	}
	c.mu.Unlock()
	c.emit()
	if closeBrowser {
		go func() {
			defer c.closing.Done()
			c.closeBrowser()
		}()
	}
	return nil
}

// Stale reports whether the language or scanlator filter changed since the
// loaded list was fetched, which means the list must be reloaded to match. A
// load already running is the reload that applies the change, so it isn't stale.
func (c *Controller) Stale() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.series != nil && !c.loading && c.filtersChanged()
}

// filtersChanged reports whether the filters differ from the loaded list's. For
// a scanlator "" and "all" are the same filter; for a language they aren't.
// The caller holds c.mu.
func (c *Controller) filtersChanged() bool {
	scanlator := func(v string) string {
		if strings.EqualFold(v, grabber.FilterAll) {
			return ""
		}

		return v
	}

	return c.settings.Language != c.loadedLanguage || scanlator(c.settings.Scanlator) != scanlator(c.loadedScanlator)
}

// URL is the last URL passed to Load
func (c *Controller) URL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.url
}

// Loaded reports whether a series is loaded
func (c *Controller) Loaded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.series != nil
}

// LoadError is the error of the last load, nil when it succeeded
func (c *Controller) LoadError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadErr
}

// Cancelling reports whether the running download has been asked to stop
func (c *Controller) Cancelling() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active != nil && c.active.cancelling
}

// SelectionGen changes whenever the selection does (see selGen)
func (c *Controller) SelectionGen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.selGen
}

// LogGen changes whenever a line is added to the log (see logGen)
func (c *Controller) LogGen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logGen
}

// LoadedFilters returns the language and scanlator the loaded list was
// fetched with
func (c *Controller) LoadedFilters() (language, scanlator string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadedLanguage, c.loadedScanlator
}

// Loading reports whether a series is being loaded
func (c *Controller) Loading() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loading
}

// Running reports whether a download is running
func (c *Controller) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active != nil
}

// Title is the loaded series' title
func (c *Controller) Title() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil {
		return ""
	}
	return c.series.Title
}

// Chapters returns the loaded chapter list, sorted by number. The list is
// never modified after Load, so the slice is shared with the caller.
func (c *Controller) Chapters() grabber.Filterables {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil {
		return nil
	}
	return c.series.Chapters
}

// HasDuplicates reports whether the list repeats chapter numbers across
// languages, so the view should show each chapter's language
func (c *Controller) HasDuplicates() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasDuplicates
}

// HasGroupDuplicates reports whether the list repeats a chapter number in one
// language with the groups naming the copies, so the view should show each
// chapter's group
func (c *Controller) HasGroupDuplicates() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasGroupDuplicates
}

// SelectedIndices returns the indices of the selected chapters, in list order,
// read under one lock so a concurrent load can't interleave with it
func (c *Controller) SelectedIndices() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []int{}
	for i, on := range c.selected {
		if on {
			out = append(out, i)
		}
	}
	return out
}

// IsSelected reports whether chapter i of Chapters is selected
func (c *Controller) IsSelected(i int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return i >= 0 && i < len(c.selected) && c.selected[i]
}

// SelectedCount returns how many chapters are selected
func (c *Controller) SelectedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, on := range c.selected {
		if on {
			n++
		}
	}
	return n
}

// selectable reports whether chapter i may be selected: the ones the
// listing marks as locked can't be, as a run could only fail them. Needs c.mu.
func (c *Controller) selectable(i int) bool {
	return !grabber.LockedOf(c.series.Chapters[i])
}

// SetSelected selects or deselects chapter i; a locked chapter is never
// selected. It's refused while a series loads or a download runs, as
// SetSelectedMany is.
func (c *Controller) SetSelected(i int, on bool) error {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return errBusy
	}
	if i < 0 || i >= len(c.selected) {
		c.mu.Unlock()
		return nil
	}
	if on && !c.selectable(i) {
		c.mu.Unlock()
		return nil
	}
	c.selected[i] = on
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SetSelectedMany selects or deselects the chapters at indices in one step,
// leaving locked ones unselected; one index out of range rejects the whole
// call. It's refused while a series loads or a download runs, as Configure is.
func (c *Controller) SetSelectedMany(indices []int, on bool) error {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return errBusy
	}
	for _, i := range indices {
		if i < 0 || i >= len(c.selected) {
			c.mu.Unlock()
			return fmt.Errorf("chapter %d is not in the list", i)
		}
	}
	for _, i := range indices {
		c.selected[i] = on && c.selectable(i)
	}
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SetAll selects or deselects every chapter but the locked ones. It's refused
// while a series loads or a download runs.
func (c *Controller) SetAll(on bool) error {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return errBusy
	}
	for i := range c.selected {
		c.selected[i] = on && c.selectable(i)
	}
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SelectRange adds the unlocked chapters whose number falls in the range text
// (e.g. "1-10,12") to the selection and returns how many it selected. It's
// refused while a series loads or a download runs.
func (c *Controller) SelectRange(text string) (int, error) {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return 0, errBusy
	}
	// parsed under the lock: it's cheap, and the busy check must come first
	rngs, err := ranges.Parse(text)
	if err != nil {
		c.mu.Unlock()
		return 0, err
	}
	n := 0
	if c.series != nil {
		for i, chap := range c.series.Chapters {
			if inRanges(chap.GetNumber(), rngs) && c.selectable(i) {
				c.selected[i] = true
				n++
			}
		}
		c.selGen++
	}
	c.mu.Unlock()
	c.emit()
	return n, nil
}

func inRanges(n float64, rngs []ranges.Range) bool {
	for _, r := range rngs {
		if n >= r.Begin && n <= r.End {
			return true
		}
	}
	return false
}

// Volumes returns the Volumes box and the site's volumes, in --volumes'
// syntax; site is "" when the site doesn't say
func (c *Controller) Volumes() (text, site string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.volumesText, c.siteVolumes
}

// SetVolumes sets the Volumes box: chapter ranges, one group per volume
// ("1-7/8-16", "16:112-114"), "auto" or "" for the site's own. A value that doesn't parse
// is refused, as is any change while a series loads or a download runs.
func (c *Controller) SetVolumes(text string) error {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return errBusy
	}
	if _, err := core.ParseVolumes(text); err != nil {
		c.mu.Unlock()
		return err
	}
	c.volumesText = strings.TrimSpace(text)
	c.mu.Unlock()
	c.emit()
	return nil
}

// runVolumes is the --volumes value a run in volume mode packs: the box,
// or the site's volumes when it's empty. Callers hold mu.
func (c *Controller) runVolumes() string {
	if c.volumesText == "" {
		return core.VolumesAuto
	}
	return c.volumesText
}

// Run describes the current or last run; a zero RunInfo before the first
func (c *Controller) Run() RunInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.shownRunLocked()
	if r == nil {
		return RunInfo{}
	}
	info := r.info
	// a fresh job has no info until its list is fetched: name it meanwhile
	info.ID, info.URL, info.Title, info.Fresh = r.id, r.url, r.title, r.fresh
	return info
}

// Fetching reports whether the running job is fetching its chapter list (a
// fresh job, before any chapter is known)
func (c *Controller) Fetching() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active != nil && c.active.fetching
}

// Jobs returns a copy of the progress rows of the current or last run, in
// chapter order (completion order is what the events arrive in)
func (c *Controller) Jobs() []Job {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.shownRunLocked()
	if r == nil {
		return nil
	}
	out := make([]Job, len(r.jobs))
	for i, j := range r.jobs {
		out[i] = *j
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// Summary returns the outcome of the last run, or nil while one runs or
// before the first run
func (c *Controller) Summary() *Summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil || c.last == nil || c.last.summary == nil {
		return nil
	}
	s := *c.last.summary
	s.Failures = append([]string(nil), c.last.summary.Failures...)
	return &s
}

// Log returns a copy of the log
func (c *Controller) Log() []LogLine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]LogLine(nil), c.log...)
}

// Start downloads the selected chapters: now, when nothing runs, or after
// the jobs going and waiting (queued is then true; see queue.go). The job
// keeps its own copy of the series, the chapters and the settings, and the
// selection is cleared, as it became the job. Validation errors are returned
// synchronously; a job for the same chapters already going or waiting is
// refused. It's refused while the series loads or the library is checked.
func (c *Controller) Start() (queued bool, err error) {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return false, errBusy
	}
	if c.series == nil {
		c.mu.Unlock()
		return false, errors.New("load a series first")
	}
	if c.filtersChanged() {
		c.mu.Unlock()
		return false, errors.New("the language or scanlator changed: reload the series first")
	}
	// the job's own settings: an option changed later doesn't reach it. Volume
	// mode packs the Volumes box, which isn't saved with the options.
	settings := new(grabber.Settings)
	*settings = *c.settings
	volumes := settings.Volumes != ""
	if volumes {
		settings.Volumes = c.runVolumes()
	}
	skip, err := core.Validate(settings)
	if err != nil {
		c.mu.Unlock()
		return false, err
	}

	var chapters grabber.Filterables
	var numbers []float64
	var selected []bool
	for i, chap := range c.series.Chapters {
		numbers = append(numbers, chap.GetNumber())
		selected = append(selected, c.selected[i])
		if c.selected[i] {
			chapters = append(chapters, chap)
		}
	}
	if len(chapters) == 0 {
		c.mu.Unlock()
		return false, errors.New("select at least one chapter")
	}
	// a chapter whose file is already in this folder, in this format, isn't
	// saved again (a bundle or volumes pack every chosen chapter)
	already := 0
	if !settings.Bundle && !volumes {
		chapters, already = c.dropSavedLocked(chapters)
		if len(chapters) == 0 {
			c.mu.Unlock()
			return false, fmt.Errorf("the %s chosen already in this folder, as %s", plural(already, "chapter is", "chapters are"), formatName(settings.Format))
		}
	}
	r := &run{
		url:                c.url,
		title:              c.series.Title,
		series:             c.series,
		settings:           settings,
		chapters:           chapters,
		hasDuplicates:      c.hasDuplicates,
		hasGroupDuplicates: c.hasGroupDuplicates,
		skip:               skip,
		rng:                bundleRange(numbers, selected),
	}
	if err := prepareRunLocked(r); err != nil {
		c.mu.Unlock()
		return false, err
	}
	if c.sameJobLocked(r) {
		c.mu.Unlock()
		return false, errors.New("those chapters are being downloaded, or waiting their turn, already")
	}
	// the selection became the job
	for i := range c.selected {
		c.selected[i] = false
	}
	c.selGen++
	queued = c.enqueueLocked(r)
	c.appendLogLocked(LogInfo, queueLine(r, queued))
	if already > 0 {
		c.appendLogLocked(LogInfo, fmt.Sprintf("Skipping %s already in %s", plural(already, "chapter", "chapters"), settings.OutputDir))
	}
	c.mu.Unlock()
	c.emit()
	return queued, nil
}

// Cancel stops the running download from starting new chapters. Chapters
// already in flight finish. The next job waiting starts when it ends; remove
// the waiting ones first to stop everything. It's a no-op when nothing runs
// or a cancel already happened.
func (c *Controller) Cancel() {
	c.mu.Lock()
	var cancel context.CancelFunc
	first := false
	if r := c.active; r != nil && r.cancel != nil && !r.cancelling {
		r.cancelling = true
		cancel = r.cancel
		first = true
		c.appendLogLocked(LogInfo, "Cancelling: chapters already downloading will finish, no new ones start")
	}
	c.mu.Unlock()
	if first {
		cancel()
		c.emit()
	}
}

// shutdownWait bounds how long quitting waits for a run to wind down and for
// the browser to close, so a stuck render can't hang the window's exit
const shutdownWait = 3 * time.Second

// Shutdown drops the waiting jobs, cancels any run and closes the browser, so
// Chrome doesn't outlive the app. It waits at most shutdownWait for each
// step. Safe to call more than once.
func (c *Controller) Shutdown() {
	c.mu.Lock()
	c.queue = nil
	c.mu.Unlock()
	c.Cancel()
	deadline := time.Now().Add(shutdownWait)
	for c.Running() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() {
		browser.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownWait):
	}
}

// events maps core.Events onto a run's progress. Each method takes the
// controller's lock itself; core calls them from several goroutines at once.
type events struct {
	c *Controller
	r *run
}

var _ core.Events = events{}

// jobFor returns the row of a chapter, nil when it has none (bundle mode has
// a single row, and a chapter whose fetch failed never got one).
// Callers must hold mu.
func (r *run) jobFor(ch *grabber.Chapter) *Job {
	if r.bundle != nil {
		return r.bundle
	}
	if j := r.chapVol[ch]; j != nil {
		return j
	}
	return r.byChap[ch]
}

// releaseLabel names a release the way the chapter list does: the language and
// group only when the run's list has copies that need them. Callers hold mu.
func (r *run) releaseLabel(f grabber.Filterable) string {
	return core.ReleaseLabel(f, r.hasDuplicates, r.hasGroupDuplicates)
}

// releaseTags is the bracketed part of releaseLabel, which a job row shows
// apart from its title. Callers hold mu.
func (r *run) releaseTags(f grabber.Filterable) string {
	return core.ReleaseTags(f, r.hasDuplicates, r.hasGroupDuplicates)
}

// live reports whether a chapter still accepts page progress: once it failed,
// pages still in flight finish but must not move its bar. Callers hold mu.
func (r *run) live(ch *grabber.Chapter) bool {
	return !r.failedC[ch]
}

// advance moves a row into the archiving phase at its first archived step,
// the point the CLI's bar switches its label: the downloads fill the bar up
// to the download share and no further. Callers hold mu.
func advance(j *Job) {
	if j.State == JobDownloading && j.Done > j.Total*core.DownloadWeight/(core.DownloadWeight+core.ArchiveWeight) {
		j.State = JobArchiving
	}
}

func (e events) BundleStart(pages int64) {
	c, r := e.c, e.r
	c.mu.Lock()
	if r.bundle != nil {
		r.bundle.Total = pages * (core.DownloadWeight + core.ArchiveWeight)
		// a corrected total comes again mid-download, so keep the status a chapter set
		if r.bundle.Status == "Gathering info..." {
			r.bundle.Status = "Downloading..."
		}
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Fetching(listed grabber.Filterable) {
	c, r := e.c, e.r
	c.mu.Lock()
	// a bundle or a volume has its own row
	if r.bundle == nil && r.listedVol[listed] == nil {
		j := &Job{Title: grabber.ChapterLabel(listed), Tags: r.releaseTags(listed), Number: listed.GetNumber(), State: JobFetching, Status: "Fetching pages..."}
		r.jobs = append(r.jobs, j)
		r.fetchRows[listed] = j
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Fetched(listed grabber.Filterable, ch *grabber.Chapter) {
	c, r := e.c, e.r
	c.mu.Lock()
	if j := r.fetchRows[listed]; j != nil {
		// the fetched chapter's title can differ from the listing's
		j.Title, j.Tags = grabber.ChapterLabel(ch), r.releaseTags(ch)
		r.byChap[ch] = j
		delete(r.fetchRows, listed)
	}
	c.mu.Unlock()
	c.emit()
}

// endRow marks the row of a chapter that ended before it started, or adds one
// when it never had any. Callers hold mu.
func (r *run) endRow(ch *grabber.Chapter, state JobState, reason string) {
	if j := r.byChap[ch]; j != nil {
		j.State, j.Reason, j.Status = state, reason, ""
		return
	}
	r.jobs = append(r.jobs, &Job{Title: grabber.ChapterLabel(ch), Tags: r.releaseTags(ch), Number: ch.GetNumber(), State: state, Reason: reason})
}

func (e events) FetchFailed(listed grabber.Filterable, err error) {
	c, r := e.c, e.r
	c.mu.Lock()
	msg := fmt.Sprintf("%s: %s", r.releaseLabel(listed), err)
	r.stats.failures = append(r.stats.failures, msg)
	if j := r.fetchRows[listed]; j != nil {
		j.State, j.Reason, j.Status = JobFailed, err.Error(), ""
		delete(r.fetchRows, listed)
	} else if r.bundle == nil && r.listedVol[listed] == nil {
		r.jobs = append(r.jobs, &Job{Title: grabber.ChapterLabel(listed), Tags: r.releaseTags(listed), Number: listed.GetNumber(), State: JobFailed, Reason: err.Error()})
	}
	c.appendLogLocked(LogError, "error fetching "+msg)
	c.mu.Unlock()
	c.emit()
}

func (e events) Skipped(ch *grabber.Chapter) {
	c, r := e.c, e.r
	c.mu.Lock()
	r.stats.skipped++
	if r.bundle == nil && r.chapVol[ch] == nil {
		r.endRow(ch, JobSkipped, "every page excluded by --skip-pages")
	}
	c.appendLogLocked(LogWarn, fmt.Sprintf("skipping %s: every page is excluded by --skip-pages", r.releaseLabel(ch)))
	c.mu.Unlock()
	c.emit()
}

func (e events) FilenameFailed(ch *grabber.Chapter, err error) {
	c, r := e.c, e.r
	c.mu.Lock()
	msg := fmt.Sprintf("%s: filename: %s", r.releaseLabel(ch), err)
	r.stats.failures = append(r.stats.failures, msg)
	if r.bundle == nil {
		r.endRow(ch, JobFailed, "filename: "+err.Error())
	}
	c.appendLogLocked(LogError, "error creating filename for "+msg)
	c.mu.Unlock()
	c.emit()
}

func (e events) Started(ch *grabber.Chapter) {
	c, r := e.c, e.r
	c.mu.Lock()
	if r.bundle != nil {
		r.bundle.Status = "Downloading " + r.releaseLabel(ch)
	} else if j := r.chapVol[ch]; j != nil {
		// its row is too narrow for the chapter's title: the phase and
		// percentage say enough. The bar moves with VolumeProgress.
		j.Status = ""
	} else if j := r.byChap[ch]; j != nil {
		// the row Fetching made becomes the downloading one
		j.Total = ch.PagesCount * (core.DownloadWeight + core.ArchiveWeight)
		j.State, j.Status = JobDownloading, ""
	} else {
		// the CLI's bar: each page's download and archive steps
		j := &Job{Title: grabber.ChapterLabel(ch), Tags: r.releaseTags(ch), Number: ch.GetNumber(), Total: ch.PagesCount * (core.DownloadWeight + core.ArchiveWeight), State: JobDownloading}
		r.jobs = append(r.jobs, j)
		r.byChap[ch] = j
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Page(ch *grabber.Chapter, idx int, err error) {
	c, r := e.c, e.r
	c.mu.Lock()
	if err != nil {
		line := fmt.Sprintf("error downloading page %d of %s: %s", idx+1, r.releaseLabel(ch), err)
		switch {
		case !is404(err) || r.held404 == nil:
			c.appendLogLocked(LogError, line)
		case r.failedC[ch]:
			// its failure was already explained; a late 404 says nothing new
		default:
			r.held404[ch] = append(r.held404[ch], line)
		}
	} else if j := r.jobFor(ch); j != nil && !j.Volume && r.live(ch) {
		// a volume's row moves with VolumeProgress instead
		j.Done += core.DownloadWeight
		advance(j)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) DownloadFailed(ch *grabber.Chapter, err error) {
	c, r := e.c, e.r
	c.mu.Lock()
	r.failedC[ch] = true
	label := r.releaseLabel(ch)
	held := r.held404[ch]
	delete(r.held404, ch)
	if is404(err) && r.series != nil {
		// ask the site for each page, outside the lock, and say what it lost
		referer := r.series.Site.BaseUrl()
		c.mu.Unlock()
		missing, others := missingPages(ch, referer)
		c.mu.Lock()
		if line, reason, ok := lostMessage(label, len(ch.Pages), missing, others); ok {
			r.stats.failures = append(r.stats.failures, line)
			if j := r.byChap[ch]; j != nil && r.bundle == nil {
				j.State = JobFailed
				j.Reason = reason
			}
			c.appendLogLocked(LogError, line)
			c.mu.Unlock()
			c.emit()
			return
		}
	}
	for _, line := range held {
		c.appendLogLocked(LogError, line)
	}
	msg := fmt.Sprintf("%s: %s", label, err)
	r.stats.failures = append(r.stats.failures, msg)
	// a volume goes on with its other chapters
	if j := r.byChap[ch]; j != nil && r.bundle == nil {
		j.State = JobFailed
		j.Reason = "download: " + err.Error()
	}
	c.appendLogLocked(LogError, "error downloading "+msg)
	c.mu.Unlock()
	c.emit()
}

func (e events) Archived(ch *grabber.Chapter) {
	c, r := e.c, e.r
	c.mu.Lock()
	if j := r.byChap[ch]; j != nil && r.bundle == nil && j.State != JobFailed {
		j.Done += core.ArchiveWeight
		advance(j)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Packed(ch *grabber.Chapter, name string, err error) {
	c, r := e.c, e.r
	c.mu.Lock()
	j := r.byChap[ch]
	if err != nil {
		msg := fmt.Sprintf("%s: packing: %s", r.releaseLabel(ch), err)
		r.stats.failures = append(r.stats.failures, msg)
		if j != nil {
			j.State = JobFailed
			j.Reason = "packing: " + err.Error()
		}
		c.appendLogLocked(LogError, msg)
	} else {
		r.stats.saved++
		r.stats.savedNumbers = append(r.stats.savedNumbers, ch.GetNumber())
		r.stats.savedFiles = append(r.stats.savedFiles, SavedFile{Number: ch.GetNumber(), Name: name})
		if j != nil {
			j.State = JobDone
			// the skipped pages were never counted, so the bar is exact only now
			j.Done = j.Total
		}
		c.appendLogLocked(LogInfo, "saved "+name)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Downloaded(ch *grabber.Chapter) {
	c, r := e.c, e.r
	c.mu.Lock()
	// the pages were counted as they came in; this only tallies the chapter
	r.stats.saved++
	r.stats.downloaded = append(r.stats.downloaded, ch.GetNumber())
	c.mu.Unlock()
	c.emit()
}

func (e events) BundlePacking() {
	c, r := e.c, e.r
	c.mu.Lock()
	if r.bundle != nil {
		r.bundle.Status = "Creating bundle for chapters " + r.rng
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) BundleArchived(n int) {
	c, r := e.c, e.r
	c.mu.Lock()
	if r.bundle != nil {
		r.bundle.Done += int64(n) * core.ArchiveWeight
	}
	c.mu.Unlock()
	c.emit()
}

// volumeTitle is the row title of a volume: "Vol. 01 · chapters 1–7"
func volumeTitle(v *core.Volume) string {
	first, last := formatNumber(v.FirstNumber()), formatNumber(v.LastNumber())
	if first == last {
		return fmt.Sprintf("%s · chapter %s", core.VolumeLabel(v.Number), first)
	}
	return fmt.Sprintf("%s · chapters %s–%s", core.VolumeLabel(v.Number), first, last)
}

func (e events) VolumeStart(v *core.Volume) {
	c, r := e.c, e.r
	c.mu.Lock()
	j := &Job{Title: volumeTitle(v), Number: v.FirstNumber(), Total: volumeScale, State: JobDownloading, Status: "Waiting...", Volume: true}
	r.jobs = append(r.jobs, j)
	r.byVolume[v] = j
	for _, chap := range v.Chapters {
		r.listedVol[chap] = j
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) VolumeChapter(v *core.Volume, ch *grabber.Chapter) {
	c, r := e.c, e.r
	c.mu.Lock()
	if j := r.byVolume[v]; j != nil {
		r.chapVol[ch] = j
	}
	c.mu.Unlock()
}

func (e events) VolumePacking(v *core.Volume) {
	c, r := e.c, e.r
	c.mu.Lock()
	if j := r.byVolume[v]; j != nil {
		j.State = JobArchiving
		j.Status = "Packing..."
	}
	c.mu.Unlock()
	c.emit()
}

// volumeScale is the Total of a volume's row, whose Done is VolumeProgress's
// fraction of it
const volumeScale = 10000

func (e events) VolumeProgress(v *core.Volume, fraction float64) {
	c, r := e.c, e.r
	c.mu.Lock()
	// core never reports a volume going back; max keeps the row honest even
	// so, and leaves a row that already ended alone
	if j := r.byVolume[v]; j != nil && j.State != JobDone && j.State != JobFailed && j.State != JobSkipped {
		j.Done = max(j.Done, int64(fraction*volumeScale))
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) VolumePacked(v *core.Volume, name string, err error) {
	c, r := e.c, e.r
	c.mu.Lock()
	j := r.byVolume[v]
	label := core.VolumeLabel(v.Number)
	switch {
	case errors.Is(err, context.Canceled):
		// a cancelled run says so in its summary; the row only stops
		if j != nil {
			j.State = JobSkipped
			j.Reason = "cancelled before all of its chapters started"
		}
	case errors.Is(err, core.ErrSkippedVolume):
		// its chapters were each counted as skipped already
		if j != nil {
			j.State = JobSkipped
			j.Reason = err.Error()
		}
	case err != nil:
		msg := fmt.Sprintf("%s: %s", label, err)
		r.stats.failures = append(r.stats.failures, msg)
		if j != nil {
			j.State = JobFailed
			j.Reason = err.Error()
		}
		c.appendLogLocked(LogError, "error packing "+msg)
	default:
		r.stats.volumes++
		for _, ch := range v.Chapters {
			r.stats.savedNumbers = append(r.stats.savedNumbers, ch.GetNumber())
			r.stats.savedFiles = append(r.stats.savedFiles, SavedFile{Number: ch.GetNumber(), Name: name})
		}
		if j != nil {
			j.State = JobDone
			j.Done = j.Total
		}
		c.appendLogLocked(LogInfo, "saved "+name)
	}
	if j != nil {
		j.Status = ""
	}
	c.mu.Unlock()
	c.emit()
}
