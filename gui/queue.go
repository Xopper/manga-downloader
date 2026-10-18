// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	mdhttp "github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/ranges"
)

// The download queue, first in first out. It's the desktop app's alone: the
// CLI downloads what it's given and exits, so there's nothing to queue there
// (see "Design every feature for both the CLI and the desktop app" in
// AGENTS.md; this is one of the deliberate one-sided features).
//
// A run owns copies of everything it needs, so the window can load, search
// and follow other series while it downloads: loading another series or
// changing an option never reaches a run that's going or waiting. Two kinds
// of job wait in the queue:
//
//   - a selection job (Download on the series page) keeps the chapters picked
//     from the list loaded at the time, site and all, and downloads exactly
//     those when its turn comes: the user chose rows, and fetching the list
//     again could reorder or drop them;
//   - a fresh job (Download N new, Download all new) names a followed series
//     and fetches its chapter list when it starts (fetchFresh), then takes
//     the chapters the library counts as new; until then its count is the
//     library's.
//
// The next job starts when the running one ends, however it ends (finished,
// failed or stopped); Shutdown empties the queue first, so nothing starts
// while the app quits.

// errNothingNew ends a fresh job whose series has nothing new once fetched
var errNothingNew = errors.New("nothing new to download")

// errRunning is returned to an action that waits for the download to end:
// the library check, which shares the browser and the sites with a run
var errRunning = errors.New("a download is running: wait for it to finish, or stop it, first")

// run is one download, waiting or started. Until it starts only the request
// is filled (what to download, with which settings); the rest is its progress.
type run struct {
	id    int
	url   string
	title string
	// fresh is a job of a followed series' new chapters, whose list is
	// fetched when it starts: series and chapters are nil until then, and
	// count is what the library counted as new
	fresh bool
	count int

	series   *core.Series
	settings *grabber.Settings
	chapters grabber.Filterables
	// hasDuplicates and hasGroupDuplicates are the list's, for the labels
	// (see Controller.releaseLabel)
	hasDuplicates      bool
	hasGroupDuplicates bool
	skip               []ranges.Range
	rng                string

	info       RunInfo
	cancel     context.CancelFunc
	cancelling bool
	// fetching is set while a fresh job fetches its chapter list
	fetching bool

	jobs    []*Job
	byChap  map[*grabber.Chapter]*Job
	bundle  *Job
	failedC map[*grabber.Chapter]bool
	// held404 is the raw "error downloading page" lines of a chapter's 404
	// pages, held back until its failure says whether the site lost them
	// (see lostpages.go)
	held404 map[*grabber.Chapter][]string
	// the rows of the volumes, by volume, by listed chapter (for a failed
	// fetch) and by fetched chapter
	byVolume  map[*core.Volume]*Job
	listedVol map[grabber.Filterable]*Job
	chapVol   map[*grabber.Chapter]*Job
	// fetchRows holds the rows of the chapters whose page list is being
	// fetched, by listed chapter, until Fetched moves them to byChap
	fetchRows map[grabber.Filterable]*Job
	stats     runStats
	summary   *Summary
}

// Queued is a waiting job as the window lists it
type Queued struct {
	ID    int
	URL   string
	Title string
	// Chapters is how many the job downloads; a fresh job's is the
	// library's count of new chapters until its list is fetched
	Chapters int
	Fresh    bool
}

// Queue returns the waiting jobs, first to start first
func (c *Controller) Queue() []Queued {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Queued, 0, len(c.queue))
	for _, r := range c.queue {
		out = append(out, Queued{ID: r.id, URL: r.url, Title: r.title, Chapters: r.count, Fresh: r.fresh})
	}
	return out
}

// shownRunLocked is the run the window shows: the one going, else the last
// one that ended. nil before the first. The caller holds mu.
func (c *Controller) shownRunLocked() *run {
	if c.active != nil {
		return c.active
	}
	return c.last
}

// enqueueLocked numbers r and starts it when nothing runs, else appends it
// to the queue; it reports whether r waits. The caller holds mu.
func (c *Controller) enqueueLocked(r *run) (queued bool) {
	c.nextID++
	r.id = c.nextID
	if c.active == nil {
		c.startLocked(r)
		return false
	}
	c.queue = append(c.queue, r)
	return true
}

// startLocked makes r the active run and starts its goroutine. The caller
// holds mu and has checked nothing runs.
func (c *Controller) startLocked(r *run) {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.cancelling = false
	c.active = r
	go c.runJob(ctx, r)
}

// startNextLocked starts the first waiting job, if any. The caller holds mu
// and has cleared the active run.
func (c *Controller) startNextLocked() {
	if c.active != nil || len(c.queue) == 0 {
		return
	}
	next := c.queue[0]
	c.queue = c.queue[1:]
	c.startLocked(next)
}

// sameJobLocked reports whether a job for the same series and chapters is
// going or waiting already. The caller holds mu.
func (c *Controller) sameJobLocked(r *run) bool {
	same := func(o *run) bool {
		return urlKey(o.url) == urlKey(r.url) && o.fresh == r.fresh && (o.fresh || o.rng == r.rng)
	}
	if c.active != nil && same(c.active) {
		return true
	}
	for _, o := range c.queue {
		if same(o) {
			return true
		}
	}
	return false
}

// siteInUseLocked reports whether site is the one the running job downloads
// from: a load must not point it at other settings meanwhile (see
// listingFor). The caller holds mu.
func (c *Controller) siteInUseLocked(site grabber.Site) bool {
	return site != nil && c.active != nil && c.active.series != nil && c.active.series.Site == site
}

// prepareRunLocked fills in what a run shows from what it downloads: its
// rows, its bundle row, the maps the events write. A volume run whose
// chapters group into no volume is refused here, before anything starts.
// The caller holds mu; r.settings, r.chapters and r.rng are set.
func prepareRunLocked(r *run) error {
	s := r.settings
	volumes := s.Volumes != ""
	rows := len(r.chapters)
	if s.Bundle {
		rows = 1
	}
	if volumes {
		spec, err := core.ParseVolumes(s.Volumes)
		if err != nil {
			return err
		}
		groups, loose, err := core.GroupVolumes(r.chapters, spec)
		if err != nil {
			return err
		}
		rows = len(groups) + len(loose)
	}
	r.count = len(r.chapters)
	r.info = RunInfo{ID: r.id, URL: r.url, Title: r.title, Fresh: r.fresh, Chapters: len(r.chapters), Bundle: s.Bundle, Volumes: volumes, Rows: rows}
	r.jobs = nil
	r.byChap = map[*grabber.Chapter]*Job{}
	r.failedC = map[*grabber.Chapter]bool{}
	r.held404 = map[*grabber.Chapter][]string{}
	r.byVolume = map[*core.Volume]*Job{}
	r.listedVol = map[grabber.Filterable]*Job{}
	r.chapVol = map[*grabber.Chapter]*Job{}
	r.fetchRows = map[grabber.Filterable]*Job{}
	r.bundle = nil
	r.stats = runStats{}
	r.summary = nil
	if s.Bundle {
		r.bundle = &Job{Title: "Bundle: chapters " + r.rng, Status: "Gathering info..."}
		r.jobs = append(r.jobs, r.bundle)
	}
	return nil
}

// runJob is a run's goroutine: a fresh job fetches its list first, then the
// chapters download, and the next waiting job starts when it's over
func (c *Controller) runJob(ctx context.Context, r *run) {
	// the browser's visibility is read when it starts, so it must follow the
	// option as of this run, not the one in force when the series was loaded
	browser.SetVisible(r.settings.BrowserVisible)
	// Hold keeps the browser open; releasing closes it when the download is done
	release := browser.Hold()
	defer release()

	if r.fresh {
		if err := c.fetchFresh(ctx, r); err != nil {
			c.endRun(r, "", err)
			return
		}
	}
	// the site is shared with the loaded series (and with other jobs of the
	// same series): it reads this run's settings from here to the end
	setSiteSettings(r.series.Site, r.settings)
	series := r.series.WithSettings(r.settings)
	file, err := series.Download(ctx, r.chapters, r.skip, r.rng, events{c, r})
	c.endRun(r, file, err)
}

// fetchFresh fetches the chapter list of a fresh job's series and picks the
// chapters the library counts as new, as a load followed by Download N new
// would, without touching the loaded series. It ends with errNothingNew when
// nothing is left to download.
func (c *Controller) fetchFresh(ctx context.Context, r *run) error {
	c.mu.Lock()
	r.fetching = true
	c.mu.Unlock()
	c.emit()
	defer func() {
		c.mu.Lock()
		r.fetching = false
		c.mu.Unlock()
	}()

	// a browser still closing after Clear would fail the fetch
	c.closing.Wait()
	// a fetch is a load: a page remembered from the last one must not stand in
	mdhttp.ResetPageMemory()
	site, siteErrs := c.identify(r.url, r.settings)
	for _, err := range siteErrs {
		c.addLog(LogWarn, "site check: "+err.Error())
	}
	if site == nil {
		return errors.New("site not recognised")
	}
	series, err := fetchSeries(site, r.settings)
	if err != nil && (r.settings.Language != "" || r.settings.Scanlator != "") && filterEmptiedList(err) {
		// a filter the series lacks: a load would fall back (see
		// dropUnmatchedFilters); the job takes every chapter instead
		plain := *r.settings
		plain.Language, plain.Scanlator = "", ""
		setSiteSettings(site, &plain)
		series, err = fetchSeries(site, &plain)
	}
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	numbers := chapterNumbers(series.Chapters)
	// the list is as good as a load's: the library and the saved lists learn it
	if lerr := c.library.MarkSeen(r.url, series.Title, numbers); lerr != nil {
		c.addLog(LogWarn, "could not save the library: "+lerr.Error())
	}
	c.saveList(r.url, series.Title, series.Chapters)
	c.ensureCover(r.url, false)

	entry, _ := c.library.Get(r.url)
	fresh := entry.NewNumbers(numbers)
	here := entry.SavedIn(r.settings.OutputDir, r.settings.Format, c.onDisk.exists)
	var chapters grabber.Filterables
	var selected []bool
	for _, ch := range series.Chapters {
		on := containsNumber(fresh, ch.GetNumber()) && !grabber.LockedOf(ch) && !containsNumber(here, ch.GetNumber())
		selected = append(selected, on)
		if on {
			chapters = append(chapters, ch)
		}
	}
	if len(chapters) == 0 {
		return errNothingNew
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	r.series = series
	r.title = series.Title
	r.chapters = chapters
	r.hasDuplicates = core.HasDuplicateChapterNumbers(series.Chapters)
	r.hasGroupDuplicates = core.HasGroupDuplicates(series.Chapters)
	r.rng = bundleRange(numbers, selected)
	if err := prepareRunLocked(r); err != nil {
		return err
	}
	c.appendLogLocked(LogInfo, fmt.Sprintf("Downloading %d chapter(s) of %s", len(chapters), series.Title))
	return nil
}

// endRun records how r ended, makes it the run shown, and starts the next
// waiting job. file is the bundle written, err Download's error (or a fresh
// job's, before any chapter).
func (c *Controller) endRun(r *run, file string, err error) {
	c.mu.Lock()
	defer func() {
		saved := r.stats.savedNumbers
		files := r.stats.savedFiles
		for i := range files {
			files[i].Dir, files[i].Format = r.settings.OutputDir, r.settings.Format
		}
		var numbers []float64
		if r.series != nil {
			numbers = chapterNumbers(r.series.Chapters)
		}
		c.mu.Unlock()
		// a run that saved something adds its chapters to the library's entry
		if len(saved) > 0 {
			c.onDisk.forget()
			if lerr := c.library.MarkSavedFiles(r.url, saved, files, numbers); lerr != nil {
				c.addLog(LogWarn, "could not save the library: "+lerr.Error())
			}
		}
		c.emit()
	}()

	s := &Summary{
		Dir:      r.settings.OutputDir,
		Bundled:  r.bundle != nil,
		Saved:    r.stats.saved,
		Volumes:  r.stats.volumes,
		Skipped:  r.stats.skipped,
		Failures: r.stats.failures,
	}
	switch {
	case r.fresh && r.series == nil && errors.Is(err, errNothingNew):
		// fetched, and the library's count was stale: nothing to do
		c.appendLogLocked(LogInfo, fmt.Sprintf("%s: nothing new to download", nonEmpty(r.title, r.url)))
	case r.fresh && r.series == nil && errors.Is(err, context.Canceled):
		s.Cancelled = true
	case r.fresh && r.series == nil && err != nil:
		// the list couldn't be fetched: the job ends before any chapter
		s.Err = err
		s.Failures = append(s.Failures, "loading the chapter list: "+err.Error())
		c.appendLogLocked(LogError, fmt.Sprintf("%s: %s", nonEmpty(r.title, r.url), err))
	case err != nil && r.info.Volumes && !errors.Is(err, context.Canceled):
		// only the volumes check prepareRunLocked already ran can fail a volume run
		s.Err = err
		s.Failures = append(s.Failures, "volumes: "+err.Error())
		c.appendLogLocked(LogError, "volumes: "+err.Error())
	case errors.Is(err, context.Canceled):
		// a cancelled bundle is never packed: the chapters it never started
		// would be silently missing from it
		s.Cancelled = true
	case err != nil:
		s.Err = err
		s.Failures = append(s.Failures, "packing bundle: "+err.Error())
		c.appendLogLocked(LogError, "packing bundle: "+err.Error())
	case r.bundle != nil && r.stats.saved == 0:
		// core still packs an empty bundle when every chapter failed (as the
		// CLI does), so the file it leaves behind is not a result
		s.Err = errors.New("no chapter could be downloaded")
		s.Failures = append(s.Failures, "packing bundle: no chapter could be downloaded")
		c.appendLogLocked(LogError, "packing bundle: no chapter could be downloaded")
	default:
		s.Bundle = file
		// the bundle's chapters reached the disk with it
		if r.bundle != nil && r.stats.saved > 0 {
			r.stats.savedNumbers = append(r.stats.savedNumbers, r.stats.downloaded...)
			for _, n := range r.stats.downloaded {
				r.stats.savedFiles = append(r.stats.savedFiles, SavedFile{Number: n, Name: file})
			}
		}
	}
	if r.bundle != nil {
		if s.Bundle == "" {
			r.bundle.State = JobFailed
		} else {
			r.bundle.State = JobDone
			r.bundle.Done = r.bundle.Total
		}
		r.bundle.Status = ""
	}
	r.summary = s
	// released in the same step that clears the run: once active is nil a
	// new job may start
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if c.active == r {
		c.active = nil
	}
	c.last = r
	c.startNextLocked()
}

// EnqueueNew queues the new chapters of a followed series (see
// Followed.NewNumbers): a fresh job, whose list is fetched when it starts. It
// reports whether the job waits for another. It's refused for a series that
// isn't followed, that the downloader can't read, that has nothing new, that
// is queued already, or while the library is being checked.
func (c *Controller) EnqueueNew(url string) (queued bool, err error) {
	entry, ok := c.Library().Get(url)
	if !ok {
		return false, errors.New("that series isn't followed")
	}
	if !entry.Downloadable {
		return false, errors.New("the downloader can't read this site yet")
	}
	if entry.New == 0 {
		return false, errNothingNew
	}
	c.mu.Lock()
	if c.checking {
		c.mu.Unlock()
		return false, errBusy
	}
	r, err := c.freshRunLocked(entry)
	if err != nil {
		c.mu.Unlock()
		return false, err
	}
	queued = c.enqueueLocked(r)
	c.appendLogLocked(LogInfo, queueLine(r, queued))
	c.mu.Unlock()
	c.emit()
	return queued, nil
}

// freshRunLocked builds the fresh job of a followed series with the settings
// as they are now; a job for it going or waiting already is refused. The
// caller holds mu.
func (c *Controller) freshRunLocked(entry Followed) (*run, error) {
	s := new(grabber.Settings)
	*s = *c.settings
	if s.Volumes != "" {
		// the Volumes box belongs to the loaded series: a fresh job packs the
		// site's own volumes
		s.Volumes = core.VolumesAuto
	}
	if _, err := core.Validate(s); err != nil {
		return nil, err
	}
	r := &run{url: entry.URL, title: nonEmpty(entry.Title, entry.URL), fresh: true, count: entry.New, settings: s}
	if c.sameJobLocked(r) {
		return nil, fmt.Errorf("%s is in the queue already", r.title)
	}
	return r, nil
}

// EnqueueAllNew queues a fresh job for every followed series with new
// chapters (skipping the ones queued already) and returns how many it queued.
// It's refused while the library is being checked, and when nothing is new.
func (c *Controller) EnqueueAllNew() (int, error) {
	entries := c.Library().List()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.checking {
		return 0, errBusy
	}
	n := 0
	for _, f := range entries {
		if !f.Downloadable || f.New == 0 {
			continue
		}
		r, err := c.freshRunLocked(f)
		if err != nil {
			continue
		}
		queued := c.enqueueLocked(r)
		c.appendLogLocked(LogInfo, queueLine(r, queued))
		n++
	}
	if n == 0 {
		return 0, errors.New("nothing new in the library")
	}
	go c.emit()
	return n, nil
}

// queueLine is the log line of a job that was queued or started
func queueLine(r *run, queued bool) string {
	what := fmt.Sprintf("%d chapter(s) of %s", r.count, r.title)
	if r.fresh {
		what = fmt.Sprintf("the %d new chapter(s) of %s", r.count, r.title)
	}
	if queued {
		return "Queued " + what
	}
	return "Downloading " + what
}

// RemoveQueued drops a waiting job; the running one is stopped with Cancel
func (c *Controller) RemoveQueued(id int) error {
	c.mu.Lock()
	for i, r := range c.queue {
		if r.id != id {
			continue
		}
		c.queue = append(c.queue[:i], c.queue[i+1:]...)
		c.appendLogLocked(LogInfo, fmt.Sprintf("Removed %s from the queue", r.title))
		c.mu.Unlock()
		c.emit()
		return nil
	}
	c.mu.Unlock()
	return errors.New("that download isn't waiting any more")
}
