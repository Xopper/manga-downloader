// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
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
	// Known is the numbers listed when the series was followed, never new;
	// Baselined is false until it was taken (no chapter is new before)
	Known     []float64 `json:"known"`
	Baselined bool      `json:"baselined"`
	// Saved is the numbers of the chapters saved in the folder and format
	// chosen now, their files still there; never null
	Saved []float64 `json:"saved"`
	// EverSaved is every chapter the app saved for it, wherever: those are
	// never new (see Followed.NewNumbers)
	EverSaved []float64 `json:"everSaved"`
	New       int       `json:"new"`
	// FollowedAt, CheckedAt and LastSaved are RFC 3339, "" when unset
	FollowedAt string `json:"followedAt"`
	CheckedAt  string `json:"checkedAt"`
	LastSaved  string `json:"lastSaved"`
	Error      string `json:"error"`
	// Cover is the series' cover as the window loads it, "/covers/<file>",
	// "" when the app has none
	Cover string `json:"cover"`
}

// LibraryDTO is State.library: the followed series, and the check in
// progress, if any. The downloads of new chapters are jobs of the queue
// (State.queue, State.run).
type LibraryDTO struct {
	Followed   []FollowedDTO `json:"followed"`
	Checking   bool          `json:"checking"`
	CheckDone  int           `json:"checkDone"`
	CheckTotal int           `json:"checkTotal"`
}

func timeDTO(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// followedDTO is f as the window shows it: saved is what's in the folder
// and format chosen now (dir, format), and savedUpTo the highest of it
func followedDTO(f Followed, dir, format string, onDisk func(string) bool) FollowedDTO {
	here := f.SavedIn(dir, format, onDisk)
	return FollowedDTO{
		URL:          f.URL,
		Title:        f.Title,
		Site:         f.Site,
		Downloadable: f.Downloadable,
		Chapters:     f.Chapters,
		Latest:       f.Latest,
		SavedUpTo:    maxNumber(here),
		Saved:        append([]float64{}, here...),
		EverSaved:    append([]float64{}, f.Saved...),
		Known:        append([]float64{}, f.Known...),
		Baselined:    f.Baselined,
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
	dto := LibraryDTO{
		Followed:   []FollowedDTO{},
		Checking:   on,
		CheckDone:  done,
		CheckTotal: total,
	}
	s := c.Settings()
	for _, f := range c.Library().List() {
		dto.Followed = append(dto.Followed, followedDTO(f, s.OutputDir, s.Format, c.onDisk.exists))
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
		// what's listed now is known: only what comes later is new
		entry.Known, entry.Baselined, entry.New = mergeNumbers(nil, numbers), true, 0
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
		// the series page still shows the cover of the series loaded: it goes
		// when another one loads
		if s := c.Covers(); s != nil && entry.Cover != "" && !c.isLoaded(url) {
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
// loads, a download runs (the check and a download share the browser and the
// sites, and refusing is simpler than sharing; the window says so) or a
// check is on.
func (a *App) CheckLibrary() error {
	c := a.ctrl
	if on, _, _ := c.Checking(); on {
		return errors.New("a check is already running")
	}
	if c.Running() {
		return errRunning
	}
	if c.Loading() {
		return errBusy
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

// DownloadNew queues the new chapters of a followed series (see
// Followed.NewNumbers) as a job of its own: its list is fetched when it
// starts, so the loaded series is left alone. It reports whether the job
// waits for another. Refused for a series that isn't followed, can't be
// read, has nothing new or is queued already, and while the library is
// being checked.
func (a *App) DownloadNew(url string) (bool, error) {
	return a.ctrl.EnqueueNew(strings.TrimSpace(url))
}

// DownloadAllNew queues a job for every followed series with new chapters,
// one after the other, and returns how many it queued. Refused when nothing
// is new, and while the library is being checked.
func (a *App) DownloadAllNew() (int, error) {
	return a.ctrl.EnqueueAllNew()
}

// RemoveQueued drops a waiting download; the running one is stopped with Cancel
func (a *App) RemoveQueued(id int) error {
	return a.ctrl.RemoveQueued(id)
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
