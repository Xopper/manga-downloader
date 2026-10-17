// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// seenCap bounds how many series the library remembers the counts of
const seenCap = 500

// Followed is a series the user follows: where it is, what the app last saw
// of it, and how far it has saved it
type Followed struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	// Site is the host of URL without "www."
	Site string `json:"site"`
	// Downloadable is whether the downloader has a grabber for the site; a
	// search-only series is followed as a link only
	Downloadable bool `json:"downloadable"`
	// Chapters and Latest are the listed chapter count and the highest
	// chapter number, as of the last load or check
	Chapters int     `json:"chapters"`
	Latest   float64 `json:"latest"`
	// SavedUpTo is the highest chapter number the app saved for the series,
	// 0 when it never did
	SavedUpTo float64 `json:"savedUpTo"`
	// New is how many listed chapters are new: not Known and not Saved
	New int `json:"new"`
	// Known is the numbers of the chapters already listed when the series
	// was followed, or at its first load or check for a library from before
	// it (Baselined false until then): they're never new. A chapter listed
	// later is new until it's saved.
	Known     []float64 `json:"known,omitempty"`
	Baselined bool      `json:"baselined,omitempty"`
	// Saved is the numbers of the chapters the app saved, sorted, so a list
	// shows "saved" on those only: saving chapter 355 alone says nothing of
	// 1 to 354. A library from before it knows only SavedUpTo, which load
	// takes as the one chapter known saved.
	Saved []float64 `json:"saved,omitempty"`
	// Files is where each saved chapter went (savedfiles.go): the window
	// shows a chapter saved only while its file is in the folder and format
	// chosen now. FilesChecked is set once the chapters saved before Files
	// existed were looked for on disk (AdoptFiles).
	Files        []SavedFile `json:"files,omitempty"`
	FilesChecked bool        `json:"filesChecked,omitempty"`
	FollowedAt   time.Time   `json:"followedAt"`
	CheckedAt    time.Time   `json:"checkedAt,omitempty"`
	LastSaved    time.Time   `json:"lastSaved,omitempty"`
	// Error is the last check's failure, "" once one succeeds
	Error string `json:"error,omitempty"`
	// Cover is the file name of the series' cover in the covers dir (see
	// covers.go), "" until one is fetched
	Cover string `json:"cover,omitempty"`
}

// Seen is what the app last saw of a series it loaded, followed or not
type Seen struct {
	Title    string    `json:"title"`
	Chapters int       `json:"chapters"`
	Latest   float64   `json:"latest"`
	At       time.Time `json:"at"`
}

// libraryFile is the shape of library.json
type libraryFile struct {
	Followed []Followed      `json:"followed"`
	Seen     map[string]Seen `json:"seen"`
}

// Library is the followed series and the counts of every series seen, kept
// on disk at path (a memory-only library has none). Every change is saved.
type Library struct {
	mu       sync.Mutex
	path     string
	followed []Followed
	seen     map[string]Seen
}

// NewLibrary returns an empty library saved at path; "" keeps it in memory
func NewLibrary(path string) *Library {
	return &Library{path: path, seen: map[string]Seen{}}
}

// Load reads the file. A missing file is an empty library. A file that
// doesn't parse is moved aside to path+".bak" (so a later save can't bury
// it), the error is returned, and the library starts empty.
func (l *Library) Load() error {
	if l.path == "" {
		return nil
	}
	data, err := os.ReadFile(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var file libraryFile
	if err := json.Unmarshal(data, &file); err != nil {
		if renameErr := os.Rename(l.path, l.path+".bak"); renameErr != nil {
			return fmt.Errorf("%s: %w (and could not move it aside: %v)", l.path, err, renameErr)
		}
		return fmt.Errorf("%s: %w (moved to %s.bak)", l.path, err, l.path)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.followed = file.Followed
	l.seen = file.Seen
	if l.seen == nil {
		l.seen = map[string]Seen{}
	}
	for i := range l.followed {
		f := &l.followed[i]
		if f.Site == "" {
			f.Site = hostOf(f.URL)
		}
		if len(f.Saved) == 0 && f.SavedUpTo > 0 {
			f.Saved = []float64{f.SavedUpTo}
		}
		// what was new by the old rule (above the highest saved) isn't by
		// this one; the next load or check takes the list as known
		if !f.Baselined {
			f.New = 0
		}
	}
	return nil
}

// saveLocked writes the file. The caller holds mu.
func (l *Library) saveLocked() error {
	if l.path == "" {
		return nil
	}
	file := libraryFile{Followed: l.followed, Seen: l.seen}
	if file.Followed == nil {
		file.Followed = []Followed{}
	}
	return writeJSONAtomic(l.path, file)
}

// List returns the followed series, in follow order
func (l *Library) List() []Followed {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Followed(nil), l.followed...)
}

// Get returns the followed entry of url
func (l *Library) Get(url string) (Followed, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i := l.indexLocked(url); i >= 0 {
		return l.followed[i], true
	}
	return Followed{}, false
}

// indexLocked finds the entry of url, -1 when none. The caller holds mu.
func (l *Library) indexLocked(url string) int {
	key := urlKey(url)
	for i, f := range l.followed {
		if urlKey(f.URL) == key {
			return i
		}
	}
	return -1
}

// Follow adds f, or replaces the entry of the same series. Site is filled
// from the URL and FollowedAt set to now when they're empty; what the
// library already saw of the series fills in counts f doesn't carry.
func (l *Library) Follow(f Followed) error {
	f.URL = strings.TrimSpace(f.URL)
	if f.URL == "" {
		return errors.New("a followed series needs a URL")
	}
	if f.Site == "" {
		f.Site = hostOf(f.URL)
	}
	if f.FollowedAt.IsZero() {
		f.FollowedAt = time.Now()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if seen, ok := l.seen[urlKey(f.URL)]; ok && f.Chapters == 0 {
		f.Chapters, f.Latest = seen.Chapters, seen.Latest
		if f.Title == "" {
			f.Title = seen.Title
		}
		if f.CheckedAt.IsZero() {
			f.CheckedAt = seen.At
		}
	}
	if i := l.indexLocked(f.URL); i >= 0 {
		// a re-follow keeps what the app saved for it
		if f.SavedUpTo == 0 {
			f.SavedUpTo = l.followed[i].SavedUpTo
			f.Saved = l.followed[i].Saved
			f.LastSaved = l.followed[i].LastSaved
		}
		if len(f.Files) == 0 {
			f.Files, f.FilesChecked = l.followed[i].Files, l.followed[i].FilesChecked
		}
		if !f.Baselined {
			f.Known, f.Baselined, f.New = l.followed[i].Known, l.followed[i].Baselined, l.followed[i].New
		}
		if f.Cover == "" {
			f.Cover = l.followed[i].Cover
		}
		l.followed[i] = f
	} else {
		l.followed = append(l.followed, f)
	}
	return l.saveLocked()
}

// Unfollow removes the entry of url; a URL not followed is a no-op
func (l *Library) Unfollow(url string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	i := l.indexLocked(url)
	if i < 0 {
		return nil
	}
	l.followed = append(l.followed[:i], l.followed[i+1:]...)
	return l.saveLocked()
}

// Update applies fn to the entry of url and saves; a URL not followed is a no-op
func (l *Library) Update(url string, fn func(*Followed)) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	i := l.indexLocked(url)
	if i < 0 {
		return nil
	}
	fn(&l.followed[i])
	return l.saveLocked()
}

// MarkSeen records the chapter list the app just fetched for url: its count
// and highest number in seen (for every series), and on a followed entry its
// count, latest, how many are new, when it was checked, and no error.
func (l *Library) MarkSeen(url, title string, numbers []float64) error {
	now := time.Now()
	latest := maxNumber(numbers)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen[urlKey(url)] = Seen{Title: title, Chapters: len(numbers), Latest: latest, At: now}
	l.trimSeenLocked()
	if i := l.indexLocked(url); i >= 0 {
		f := &l.followed[i]
		if title != "" {
			f.Title = title
		}
		f.Chapters = len(numbers)
		f.Latest = latest
		if !f.Baselined {
			f.Known, f.Baselined = mergeNumbers(nil, numbers), true
		}
		f.New = len(f.NewNumbers(numbers))
		f.CheckedAt = now
		f.Error = ""
	}
	return l.saveLocked()
}

// MarkFailed records a check of url that failed; a URL not followed is a no-op
func (l *Library) MarkFailed(url string, err error) error {
	return l.Update(url, func(f *Followed) {
		f.CheckedAt = time.Now()
		f.Error = err.Error()
	})
}

// MarkSaved adds saved, the numbers of the chapters a run wrote, to what
// url has saved, raises the saved mark to the highest of them (never lowers
// it) and recounts what's new from numbers, the listed chapters. A URL not
// followed is a no-op.
func (l *Library) MarkSaved(url string, saved []float64, numbers []float64) error {
	return l.MarkSavedFiles(url, saved, nil, numbers)
}

// MarkSavedFiles is MarkSaved that also records the files the chapters went
// to; an entry for the same chapter, folder and format is replaced
func (l *Library) MarkSavedFiles(url string, saved []float64, files []SavedFile, numbers []float64) error {
	return l.Update(url, func(f *Followed) {
		f.Files = addFiles(f.Files, files)
		f.Saved = mergeNumbers(f.Saved, saved)
		if upTo := maxNumber(saved); upTo > f.SavedUpTo {
			f.SavedUpTo = upTo
		}
		f.LastSaved = time.Now()
		if len(numbers) > 0 {
			f.New = len(f.NewNumbers(numbers))
		}
	})
}

// MarkDownloadable turns on Downloadable for the followed series of the sites
// the downloader reads now (is), followed when their site was search-only;
// the library is saved when one changed. It never turns one off: a site
// that drops out of the table keeps the series its user could download.
func (l *Library) MarkDownloadable(is func(site string) bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := false
	for i := range l.followed {
		f := &l.followed[i]
		if !f.Downloadable && is(f.Site) {
			f.Downloadable = true
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return l.saveLocked()
}

// SetCover records the cover file name of url; a URL not followed is a no-op
func (l *Library) SetCover(url, name string) error {
	return l.Update(url, func(f *Followed) { f.Cover = name })
}

// SeenOf returns what the app last saw of url, followed or not
func (l *Library) SeenOf(url string) (Seen, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.seen[urlKey(url)]
	return s, ok
}

// trimSeenLocked drops the oldest entries past seenCap. The caller holds mu.
func (l *Library) trimSeenLocked() {
	if len(l.seen) <= seenCap {
		return
	}
	keys := make([]string, 0, len(l.seen))
	for k := range l.seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return l.seen[keys[i]].At.Before(l.seen[keys[j]].At) })
	for _, k := range keys[:len(keys)-seenCap] {
		delete(l.seen, k)
	}
}

// mergeNumbers is a and b together, sorted, each number once
func mergeNumbers(a, b []float64) []float64 {
	out := append(append([]float64(nil), a...), b...)
	sort.Float64s(out)
	return slices.Compact(out)
}

// NewNumbers is the numbers among numbers, the listed chapters, that are
// new: listed after the series was followed and not saved since. Each
// number once, sorted; none before the entry has its known list.
func (f *Followed) NewNumbers(numbers []float64) []float64 {
	if !f.Baselined {
		return nil
	}
	var out []float64
	for _, n := range mergeNumbers(nil, numbers) {
		if !containsNumber(f.Known, n) && !containsNumber(f.Saved, n) {
			out = append(out, n)
		}
	}
	return out
}

// containsNumber reports whether sorted holds n
func containsNumber(sorted []float64, n float64) bool {
	_, ok := slices.BinarySearch(sorted, n)
	return ok
}

func maxNumber(numbers []float64) float64 {
	m := 0.0
	for _, n := range numbers {
		if n > m {
			m = n
		}
	}
	return m
}

// urlKey is the form two spellings of one series URL share: trimmed, the
// scheme and host in lower case, "www." dropped, no trailing slash
func urlKey(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	u.Path = strings.TrimRight(u.Path, "/")
	u.Fragment = ""
	return u.String()
}

// hostOf is the host of raw without "www.", "" when it doesn't parse
func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}
