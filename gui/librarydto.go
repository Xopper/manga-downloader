// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// FollowedDTO is a followed series as the window shows it
type FollowedDTO struct {
	URL          string  `json:"url"`
	Title        string  `json:"title"`
	Site         string  `json:"site"`
	Downloadable bool    `json:"downloadable"`
	Chapters     int     `json:"chapters"`
	Latest       float64 `json:"latest"`
	SavedUpTo    float64 `json:"savedUpTo"`
	New          int     `json:"new"`
	// FollowedAt, CheckedAt and LastSaved are RFC 3339, "" when unset
	FollowedAt string `json:"followedAt"`
	CheckedAt  string `json:"checkedAt"`
	LastSaved  string `json:"lastSaved"`
	Error      string `json:"error"`
	// Cover is the series' cover as the window loads it, "/covers/<file>",
	// "" when the app has none
	Cover string `json:"cover"`
}

// LibraryDTO is State.library: the followed series, and the check and the
// batch download in progress, if any
type LibraryDTO struct {
	Followed   []FollowedDTO `json:"followed"`
	Checking   bool          `json:"checking"`
	CheckDone  int           `json:"checkDone"`
	CheckTotal int           `json:"checkTotal"`
	Batch      BatchDTO      `json:"batch"`
}

// BatchDTO is the download of every followed series' new chapters in turn
type BatchDTO struct {
	Active  bool   `json:"active"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Current string `json:"current"`
	// CurrentURL is the series being downloaded, as the library keys it
	CurrentURL string `json:"currentURL"`
}

func timeDTO(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func followedDTO(f Followed) FollowedDTO {
	return FollowedDTO{
		URL:          f.URL,
		Title:        f.Title,
		Site:         f.Site,
		Downloadable: f.Downloadable,
		Chapters:     f.Chapters,
		Latest:       f.Latest,
		SavedUpTo:    f.SavedUpTo,
		New:          f.New,
		FollowedAt:   timeDTO(f.FollowedAt),
		CheckedAt:    timeDTO(f.CheckedAt),
		LastSaved:    timeDTO(f.LastSaved),
		Error:        f.Error,
		Cover:        coverPath(f.Cover),
	}
}

// coverPath is the window's path of a cover file name, "" for none
func coverPath(name string) string {
	if name == "" {
		return ""
	}
	return "/covers/" + name
}

func libraryDTO(c *Controller) LibraryDTO {
	on, done, total := c.Checking()
	batch := c.Batch()
	dto := LibraryDTO{
		Followed:   []FollowedDTO{},
		Checking:   on,
		CheckDone:  done,
		CheckTotal: total,
		Batch:      BatchDTO{Active: batch.Active, Done: batch.Done, Total: batch.Total, Current: batch.Current, CurrentURL: batch.CurrentURL},
	}
	for _, f := range c.Library().List() {
		dto.Followed = append(dto.Followed, followedDTO(f))
	}
	return dto
}

// Follow adds a series to the library (or updates its entry). The loaded
// series fills in its own count; any other takes what the library saw of it.
func (a *App) Follow(f FollowedDTO) error {
	url := strings.TrimSpace(f.URL)
	if url == "" {
		return errors.New("a followed series needs a link")
	}
	entry := Followed{URL: url, Title: strings.TrimSpace(f.Title), Downloadable: f.Downloadable}
	c := a.ctrl
	if c.Loaded() && urlKey(c.URL()) == urlKey(url) {
		numbers := chapterNumbers(c.Chapters())
		entry.Title = c.Title()
		entry.Chapters = len(numbers)
		entry.Latest = maxNumber(numbers)
		entry.New = newAbove(numbers, 0)
		entry.CheckedAt = time.Now()
		entry.Downloadable = true
	}
	if err := c.Library().Follow(entry); err != nil {
		return err
	}
	c.addLog(LogInfo, fmt.Sprintf("Following %s", nonEmpty(entry.Title, url)))
	c.ensureCover(url, true)
	return nil
}

// Unfollow removes a series from the library; what's on disk stays
func (a *App) Unfollow(url string) error {
	c := a.ctrl
	entry, ok := c.Library().Get(url)
	if err := c.Library().Unfollow(url); err != nil {
		return err
	}
	if ok {
		c.addLog(LogInfo, fmt.Sprintf("Unfollowed %s", nonEmpty(entry.Title, url)))
		if s := c.Covers(); s != nil && entry.Cover != "" {
			if err := s.Remove(entry.Cover); err != nil {
				c.addLog(LogWarn, "could not remove the cover: "+err.Error())
			}
		}
	}
	c.emit()
	return nil
}

func nonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// CheckLibrary fetches every followed series' chapter list in the
// background; the counts land in State.library. It's refused while a series
// loads, a download runs or a check is on.
func (a *App) CheckLibrary() error {
	c := a.ctrl
	if on, _, _ := c.Checking(); on {
		return errors.New("a check is already running")
	}
	if c.Loading() || c.Running() {
		return errBusy
	}
	if c.BatchActive() {
		return errBatch
	}
	if len(c.Library().List()) == 0 {
		return errors.New("follow a series first")
	}
	go func() {
		if err := c.CheckLibrary(a.updateCtx); err != nil && !errors.Is(err, errBusy) {
			c.addLog(LogWarn, err.Error())
		}
	}()
	return nil
}

// CancelCheck stops the check after the series being fetched
func (a *App) CancelCheck() {
	a.ctrl.CancelCheck()
}

// DownloadNew loads a followed series, selects the chapters above its saved
// mark and starts the download, in the background. Refusals (busy, not
// followed, nothing new once loaded) come back as the load's would: the
// first two here, the last in the log.
func (a *App) DownloadNew(url string) error {
	url = strings.TrimSpace(url)
	entry, ok := a.ctrl.Library().Get(url)
	if !ok {
		return errors.New("that series isn't followed")
	}
	if !entry.Downloadable {
		return errors.New("the downloader can't read this site yet")
	}
	if a.ctrl.BatchActive() {
		return errBatch
	}
	if err := a.ctrl.BeginLoad(url); err != nil {
		return err
	}
	go func() {
		_ = a.downloadNewLoaded(context.Background(), url)
	}()
	return nil
}

// errBatch is returned to an action that would race the batch download
var errBatch = errors.New("every new chapter is being downloaded: stop that first")

// downloadNewLoaded does the load BeginLoad started, then selects and starts
// the new chapters of url, unless ctx ended meanwhile. It returns once the
// run has started (or couldn't).
func (a *App) downloadNewLoaded(ctx context.Context, url string) error {
	c := a.ctrl
	if err := a.finishLoadNow(url); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	entry, _ := c.Library().Get(url)
	n, err := c.SelectAbove(entry.SavedUpTo)
	if err != nil {
		return err
	}
	if n == 0 {
		c.addLog(LogInfo, fmt.Sprintf("%s: nothing new to download", nonEmpty(entry.Title, url)))
		return errors.New("nothing new")
	}
	if err := c.Start(); err != nil {
		c.addLog(LogWarn, "could not start: "+err.Error())
		return err
	}
	return nil
}

// finishLoadNow is finishLoad's body, run on the caller's goroutine
func (a *App) finishLoadNow(url string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the site's parser failed: %v", r)
			a.ctrl.abortLoad(err)
		}
		a.sink.Emit("chapters", a.Chapters())
	}()
	before := optionsOf(a.ctrl.Settings())
	err = a.ctrl.FinishLoad(url)
	if optionsOf(a.ctrl.Settings()) != before {
		a.save()
	}
	return err
}

// DownloadAllNew downloads the new chapters of every followed series that
// has some, one series after the other, in the background. Its progress is
// State.library.batch. It's refused while busy or while a batch runs.
func (a *App) DownloadAllNew() error {
	c := a.ctrl
	if c.Loading() || c.Running() {
		return errBusy
	}
	if on, _, _ := c.Checking(); on {
		return errBusy
	}
	if c.BatchActive() {
		return errBatch
	}
	var todo []Followed
	for _, f := range c.Library().List() {
		if f.Downloadable && f.New > 0 {
			todo = append(todo, f)
		}
	}
	if len(todo) == 0 {
		return errors.New("nothing new in the library")
	}
	ctx, err := c.beginBatch(len(todo))
	if err != nil {
		return err
	}
	go a.runBatch(ctx, todo)
	return nil
}

// runBatch is the batch's goroutine: each series is loaded, its new
// chapters selected and downloaded, and the next waits for the run to end
func (a *App) runBatch(ctx context.Context, todo []Followed) {
	c := a.ctrl
	defer c.endBatch()
	done, skipped := 0, 0
	for i, f := range todo {
		if ctx.Err() != nil {
			c.addLog(LogInfo, fmt.Sprintf("Stopped after %d of %d series", done, len(todo)))
			return
		}
		c.batchStep(nonEmpty(f.Title, f.URL), f.URL, i)
		if err := c.BeginLoad(f.URL); err != nil {
			c.addLog(LogWarn, fmt.Sprintf("%s: %s", f.Title, err))
			skipped++
			continue
		}
		if err := a.downloadNewLoaded(ctx, f.URL); err != nil {
			skipped++
			continue
		}
		// the run ends on its own goroutine; the next series waits for it
		for c.Running() {
			// CancelBatch cancels the run too, which then winds down
			time.Sleep(50 * time.Millisecond)
		}
		done++
	}
	c.batchStep("", "", len(todo))
	if skipped > 0 {
		c.addLog(LogWarn, fmt.Sprintf("Downloaded the new chapters of %d series; %d couldn't be loaded", done, skipped))
	} else {
		c.addLog(LogInfo, fmt.Sprintf("Downloaded the new chapters of %d series", done))
	}
}

// CancelBatch stops the batch after the series being downloaded, whose run
// is cancelled too
func (a *App) CancelBatch() {
	a.ctrl.CancelBatch()
}
