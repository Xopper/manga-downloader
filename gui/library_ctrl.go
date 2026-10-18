// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/elboletaire/manga-downloader/catalog"
	"slices"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	mdhttp "github.com/elboletaire/manga-downloader/http"
)

// The controller's side of the library: recording loads and runs happens in
// controller.go; here are the actions the window asks for. Checking the
// followed series fetches each one's chapter list without touching the
// loaded series, and counts as busy, so no load or download races it.

// SetLibrary gives the controller the library the window shows and keeps
// (loaded by the caller). Until then it works on a memory-only one.
func (c *Controller) SetLibrary(l *Library) {
	c.mu.Lock()
	c.library = l
	dir, format := c.settings.OutputDir, c.settings.Format
	c.mu.Unlock()
	if err := l.AdoptFiles(dir, format); err != nil {
		c.addLog(LogWarn, "could not save the library: "+err.Error())
	}
	// a site that was search-only when the series was followed may be read now
	if err := l.MarkDownloadable(func(site string) bool { _, ok := catalog.SiteByHost(site); return ok }); err != nil {
		c.addLog(LogWarn, "could not save the library: "+err.Error())
	}
	c.emit()
}

// Library returns the library
func (c *Controller) Library() *Library {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.library
}

// Checking reports whether the followed series are being checked, and how
// many of them are done
func (c *Controller) Checking() (on bool, done, total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checking, c.checkDone, c.checkTotal
}

// CheckLibrary fetches the chapter list of every followed series the
// downloader can read, one at a time, and records each one's count and
// what's new (or the failure) in the library. The loaded series, its
// selection and filters are left alone. It's refused while a series loads
// or a download runs (the two share the browser and the sites; refusing is
// simpler than sharing them), and loads and downloads are refused while it
// runs. It stops early when
// ctx ends or CancelCheck is called; the series already checked keep their
// new counts.
func (c *Controller) CheckLibrary(ctx context.Context) error {
	c.mu.Lock()
	if c.busyLocked() || c.active != nil {
		c.mu.Unlock()
		return errBusy
	}
	entries := c.library.List()
	var todo []Followed
	for _, f := range entries {
		if f.Downloadable {
			todo = append(todo, f)
		}
	}
	if len(todo) == 0 {
		c.mu.Unlock()
		return errors.New("nothing to check: no followed series the downloader can read")
	}
	ctx, cancel := context.WithCancel(ctx)
	c.checking = true
	c.checkDone, c.checkTotal = 0, len(todo)
	c.checkCancel = cancel
	// the settings as of now, so a Configure during the check (refused, but
	// the copy costs nothing) can't reach the sites mid-fetch
	s := new(grabber.Settings)
	*s = *c.settings
	c.appendLogLocked(LogInfo, fmt.Sprintf("Checking %d followed series for new chapters", len(todo)))
	c.mu.Unlock()
	c.emit()

	defer func() {
		c.mu.Lock()
		c.checking = false
		c.checkCancel = nil
		c.mu.Unlock()
		cancel()
		c.emit()
	}()

	for _, f := range todo {
		if ctx.Err() != nil {
			c.addLog(LogInfo, "Check stopped")
			return nil
		}
		title, numbers, err := c.fetchNumbers(f.URL, s)
		if err != nil {
			c.addLog(LogWarn, fmt.Sprintf("%s: %s", f.Title, err))
			if lerr := c.library.MarkFailed(f.URL, err); lerr != nil {
				c.addLog(LogWarn, "could not save the library: "+lerr.Error())
			}
		} else {
			if lerr := c.library.MarkSeen(f.URL, title, numbers); lerr != nil {
				c.addLog(LogWarn, "could not save the library: "+lerr.Error())
			}
			if entry, ok := c.library.Get(f.URL); ok && entry.New > 0 {
				c.addLog(LogInfo, fmt.Sprintf("%s: %d new", entry.Title, entry.New))
			}
			c.ensureCover(f.URL, false)
		}
		c.mu.Lock()
		c.checkDone++
		c.mu.Unlock()
		c.emit()
	}
	c.addLog(LogInfo, "Check done")
	return nil
}

// CancelCheck stops a running check after the series being fetched
func (c *Controller) CancelCheck() {
	c.mu.Lock()
	cancel := c.checkCancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// fetchNumbers fetches the title and chapter numbers of url with the
// settings s, as a load does, without keeping anything: the loaded series,
// the listings cache and the filters are untouched
func (c *Controller) fetchNumbers(url string, s *grabber.Settings) (title string, numbers []float64, err error) {
	// a browser still closing after Clear would fail the fetch
	c.closing.Wait()
	browser.SetVisible(s.BrowserVisible)
	release := browser.Hold()
	defer release()

	// a check fetches, as a load does
	mdhttp.ResetPageMemory()
	site, siteErrs := c.identify(url, s)
	for _, serr := range siteErrs {
		c.addLog(LogWarn, "site check: "+serr.Error())
	}
	if site == nil {
		return "", nil, errors.New("site not recognised")
	}
	series, err := fetchSeries(site, s)
	if err != nil && (s.Language != "" || s.Scanlator != "") && filterEmptiedList(err) {
		// a filter the series lacks: a load would fall back (see
		// dropUnmatchedFilters); the check only counts, so it counts them all
		plain := *s
		plain.Language, plain.Scanlator = "", ""
		setSiteSettings(site, &plain)
		series, err = fetchSeries(site, &plain)
	}
	if err != nil {
		return "", nil, err
	}
	// the list a check fetched is as good as a load's: the next open of
	// the series shows it at once
	c.saveList(url, series.Title, series.Chapters)
	return series.Title, chapterNumbers(series.Chapters), nil
}

// fetchSeries fetches and filters the chapter list of site with s
func fetchSeries(site grabber.Site, s *grabber.Settings) (*core.Series, error) {
	listing, err := core.Fetch(site)
	if err != nil {
		return nil, err
	}
	return listing.Series(s)
}

// filterEmptiedList reports whether err means "no chapters for this filter"
// rather than a fetch failure
func filterEmptiedList(err error) bool {
	var noChapters *core.NoChaptersError
	var chaptersErr *core.ChaptersError
	var unavailable *grabber.UnavailableFilterError
	return errors.As(err, &noChapters) || errors.As(err, &chaptersErr) || errors.As(err, &unavailable)
}

// SelectNumbers selects the chapters numbered as one of numbers, leaving the
// rest as they are, and returns how many it selected
func (c *Controller) SelectNumbers(numbers []float64) (int, error) {
	c.mu.Lock()
	if c.busyLocked() {
		c.mu.Unlock()
		return 0, errBusy
	}
	n := 0
	if c.series != nil {
		for i, chap := range c.series.Chapters {
			if slices.Contains(numbers, chap.GetNumber()) {
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
