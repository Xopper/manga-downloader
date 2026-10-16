// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

// The saved lists: the chapter list the app last saw of every series it
// loaded, one small JSON file per series in lists/ next to library.json. A
// load of a series with a saved list shows that list at once, locked and
// marked Reloading…, while the site is asked for the current one, so a
// series opened from the library is on screen in a moment instead of after
// the site answers. The saved list is display only: a download needs the
// site's own chapters, which the load brings.

// listStoreCap bounds the files kept; the oldest go past it
const listStoreCap = 200

// savedChapter is one row of a saved list. It's a Filterable (and reports
// its language, group and volume as the site's chapters do), so the window
// shows it as it shows a fetched chapter.
type savedChapter struct {
	Number   float64 `json:"number"`
	Title    string  `json:"title"`
	Language string  `json:"language,omitempty"`
	Group    string  `json:"group,omitempty"`
	Volume   int     `json:"volume,omitempty"`
}

func (c savedChapter) GetNumber() float64  { return c.Number }
func (c savedChapter) GetTitle() string    { return c.Title }
func (c savedChapter) GetLanguage() string { return c.Language }
func (c savedChapter) GetGroup() string    { return c.Group }
func (c savedChapter) GetVolume() int      { return c.Volume }

// savedList is the content of a list file
type savedList struct {
	URL      string         `json:"url"`
	Title    string         `json:"title"`
	SavedAt  time.Time      `json:"savedAt"`
	Chapters []savedChapter `json:"chapters"`
}

// filterables returns the list as the controller holds a chapter list
func (l *savedList) filterables() grabber.Filterables {
	out := make(grabber.Filterables, 0, len(l.Chapters))
	for _, c := range l.Chapters {
		out = append(out, c)
	}
	return out
}

// savedChaptersOf copies what the window shows of a chapter list
func savedChaptersOf(chapters grabber.Filterables) []savedChapter {
	out := make([]savedChapter, 0, len(chapters))
	for _, ch := range chapters {
		row := savedChapter{Number: ch.GetNumber(), Title: ch.GetTitle(), Volume: grabber.VolumeOf(ch)}
		if l, ok := ch.(interface{ GetLanguage() string }); ok {
			row.Language = l.GetLanguage()
		}
		if g, ok := ch.(interface{ GetGroup() string }); ok {
			row.Group = g.GetGroup()
		}
		out = append(out, row)
	}
	return out
}

// listStore is the saved lists on disk
type listStore struct {
	dir string
}

// newListStore keeps the lists in dir, made on the first save
func newListStore(dir string) *listStore {
	return &listStore{dir: dir}
}

// file is the path of the list of url: two spellings of one series share it
func (s *listStore) file(url string) string {
	sum := sha256.Sum256([]byte(urlKey(url)))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}

// Get returns the saved list of url, false when there's none (or it doesn't
// read: a broken file is as good as none)
func (s *listStore) Get(url string) (*savedList, bool) {
	b, err := os.ReadFile(s.file(url))
	if err != nil {
		return nil, false
	}
	var l savedList
	if err := json.Unmarshal(b, &l); err != nil || len(l.Chapters) == 0 {
		return nil, false
	}
	return &l, true
}

// Put saves the list of url, and drops the oldest files past listStoreCap
func (s *listStore) Put(url, title string, chapters grabber.Filterables) error {
	if len(chapters) == 0 {
		return errors.New("nothing to save")
	}
	l := savedList{URL: url, Title: title, SavedAt: time.Now(), Chapters: savedChaptersOf(chapters)}
	if err := writeJSONAtomic(s.file(url), l); err != nil {
		return err
	}
	s.prune()
	return nil
}

// prune removes the oldest files past listStoreCap
func (s *listStore) prune() {
	entries, err := os.ReadDir(s.dir)
	if err != nil || len(entries) <= listStoreCap {
		return
	}
	type aged struct {
		name string
		at   time.Time
	}
	var files []aged
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, aged{name: e.Name(), at: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.Before(files[j].at) })
	for len(files) > listStoreCap {
		_ = os.Remove(filepath.Join(s.dir, files[0].name))
		files = files[1:]
	}
}
