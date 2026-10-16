// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// SavedFile is the file a chapter was saved in: Name in Dir, written as
// Format ("cbz", "raw" or "epub"). A bundle or a volume holds several
// chapters, each with its own entry naming the same file.
type SavedFile struct {
	Number float64 `json:"n"`
	Dir    string  `json:"dir"`
	Format string  `json:"format"`
	Name   string  `json:"name"`
}

// addFiles adds files to have, replacing an entry for the same chapter in
// the same folder and format (a chapter saved again there)
func addFiles(have, files []SavedFile) []SavedFile {
	for _, nf := range files {
		have = slices.DeleteFunc(have, func(f SavedFile) bool {
			return f.Number == nf.Number && f.Dir == nf.Dir && f.Format == nf.Format
		})
		have = append(have, nf)
	}
	return have
}

// SavedIn returns the numbers of the chapters whose file is in dir, written
// as format, and still there (onDisk), sorted
func (f *Followed) SavedIn(dir, format string, onDisk func(path string) bool) []float64 {
	var numbers []float64
	for _, sf := range f.Files {
		if sf.Dir != dir || sf.Format != format || containsNumber(numbers, sf.Number) {
			continue
		}
		if onDisk(filepath.Join(sf.Dir, sf.Name)) {
			numbers = append(numbers, sf.Number)
			slices.Sort(numbers)
		}
	}
	return numbers
}

// presenceTTL is how long a file's presence is trusted: every state push
// asks for every saved file, and a file deleted in Finder shows within it
const presenceTTL = 3 * time.Second

// presence answers whether a path exists, remembering each answer for
// presenceTTL
type presence struct {
	mu sync.Mutex
	at map[string]presenceEntry
}

type presenceEntry struct {
	exists bool
	at     time.Time
}

func (p *presence) exists(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.at[path]; ok && time.Since(e.at) < presenceTTL {
		return e.exists
	}
	_, err := os.Stat(path)
	if p.at == nil {
		p.at = map[string]presenceEntry{}
	}
	p.at[path] = presenceEntry{exists: err == nil, at: time.Now()}
	return err == nil
}

// forget drops what p remembers, after a run wrote files
func (p *presence) forget() {
	p.mu.Lock()
	p.at = nil
	p.mu.Unlock()
}

// AdoptFiles gives the chapters saved before Files existed the files
// they're in, when the default file names find them in dir as format.
// Each series is looked at once (FilesChecked); a chapter not found there
// isn't shown saved, as if it was saved elsewhere.
func (l *Library) AdoptFiles(dir, format string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var entries []os.DirEntry
	read := false
	changed := false
	for i := range l.followed {
		f := &l.followed[i]
		if f.FilesChecked {
			continue
		}
		f.FilesChecked = true
		changed = true
		if len(f.Saved) == 0 || dir == "" {
			continue
		}
		if !read {
			entries, _ = os.ReadDir(dir)
			read = true
		}
		for _, n := range f.Saved {
			if name := findSaved(entries, f.Title, n, format); name != "" {
				f.Files = append(f.Files, SavedFile{Number: n, Dir: dir, Format: format, Name: name})
			}
		}
	}
	if !changed {
		return nil
	}
	return l.saveLocked()
}

// findSaved returns the entry the default file name template would have
// given chapter n of title in format, "" when there's none
func findSaved(entries []os.DirEntry, title string, n float64, format string) string {
	// as packer.NewChapterFileTemplateParts writes the number
	number := strings.Replace(fmt.Sprintf("%.1f", n), ".0", "", 1)
	prefix := packer.SanitizeFilename(title) + " " + number + " - "
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		switch format {
		case packer.FormatRaw:
			if e.IsDir() {
				return name
			}
		case packer.FormatEPUB:
			if !e.IsDir() && strings.HasSuffix(name, ".epub") {
				return name
			}
		default:
			if !e.IsDir() && strings.HasSuffix(name, ".cbz") {
				return name
			}
		}
	}
	return ""
}

// dropSavedLocked returns chapters without the ones saved in the folder and
// format chosen now, their files still there, and how many it dropped. The
// caller holds c.mu.
func (c *Controller) dropSavedLocked(chapters grabber.Filterables) (grabber.Filterables, int) {
	if c.library == nil {
		return chapters, 0
	}
	f, ok := c.library.Get(c.url)
	if !ok {
		return chapters, 0
	}
	here := f.SavedIn(c.settings.OutputDir, c.settings.Format, c.onDisk.exists)
	if len(here) == 0 {
		return chapters, 0
	}
	var kept grabber.Filterables
	for _, chap := range chapters {
		if !containsNumber(here, chap.GetNumber()) {
			kept = append(kept, chap)
		}
	}
	return kept, len(chapters) - len(kept)
}

// plural is n with the singular or plural word: "1 chapter", "3 chapters"
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// formatName is how the window names an output format
func formatName(format string) string {
	switch format {
	case packer.FormatRaw:
		return "image folders"
	case packer.FormatEPUB:
		return "EPUB"
	default:
		return "CBZ"
	}
}

// LatestSavedHere returns the path of url's highest chapter saved in the
// folder and format chosen now, its file still there; "" when none is
func (c *Controller) LatestSavedHere(url string) string {
	f, ok := c.Library().Get(url)
	if !ok {
		return ""
	}
	s := c.Settings()
	here := f.SavedIn(s.OutputDir, s.Format, c.onDisk.exists)
	if len(here) == 0 {
		return ""
	}
	top := here[len(here)-1]
	for _, sf := range f.Files {
		if sf.Number == top && sf.Dir == s.OutputDir && sf.Format == s.Format {
			return filepath.Join(sf.Dir, sf.Name)
		}
	}
	return ""
}
