// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

// A chapter shows saved only in the folder and format it went to, and only
// while its file is there
func TestSavedInFollowsTheFolderAndTheFile(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("X 1 - One.cbz")
	write("X 2 - Two.cbz")
	f := Followed{Files: []SavedFile{
		{Number: 1, Dir: dir, Format: "cbz", Name: "X 1 - One.cbz"},
		{Number: 2, Dir: dir, Format: "cbz", Name: "X 2 - Two.cbz"},
		{Number: 3, Dir: dir, Format: "cbz", Name: "X 3 - Three.cbz"}, // deleted since
		{Number: 4, Dir: other, Format: "cbz", Name: "X 4 - Four.cbz"},
	}}
	var p presence
	if got := f.SavedIn(dir, "cbz", p.exists); !slices.Equal(got, []float64{1, 2}) {
		t.Errorf("in dir as cbz = %v, want [1 2]", got)
	}
	if got := f.SavedIn(dir, "epub", p.exists); len(got) != 0 {
		t.Errorf("in dir as epub = %v, want none", got)
	}
	if got := f.SavedIn(t.TempDir(), "cbz", p.exists); len(got) != 0 {
		t.Errorf("in a new folder = %v, want none", got)
	}
}

// Saving a chapter again in the same folder and format replaces its entry
func TestAddFilesReplacesTheSameChapterThere(t *testing.T) {
	have := []SavedFile{{Number: 1, Dir: "/a", Format: "cbz", Name: "old.cbz"}, {Number: 1, Dir: "/b", Format: "cbz", Name: "b.cbz"}}
	got := addFiles(have, []SavedFile{{Number: 1, Dir: "/a", Format: "cbz", Name: "new.cbz"}})
	if len(got) != 2 || got[1].Name != "new.cbz" || got[0].Dir != "/b" {
		t.Errorf("files = %+v", got)
	}
}

// The chapters saved before files were recorded are found by their default
// names, once
func TestAdoptFilesFindsOldSaves(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Hunter X Hunter 5 - Chapter 5.cbz", "Hunter X Hunter 6.5 - Extra.cbz", "Hunter X Hunter 7 - Seven.epub", "Other 5 - Five.cbz"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	l := NewLibrary("")
	url := "https://example.com/manga/hxh"
	_ = l.Follow(Followed{URL: url, Title: "Hunter X Hunter"})
	_ = l.MarkSaved(url, []float64{5, 6.5, 7, 8}, nil)
	if err := l.AdoptFiles(dir, "cbz"); err != nil {
		t.Fatal(err)
	}
	f, _ := l.Get(url)
	var p presence
	if got := f.SavedIn(dir, "cbz", p.exists); !slices.Equal(got, []float64{5, 6.5}) {
		t.Errorf("adopted = %v, want [5 6.5] (7 is an epub, 8 isn't there)", got)
	}
	if !f.FilesChecked {
		t.Error("the series isn't marked as checked")
	}
	// a second look changes nothing, even with the files gone
	_ = l.AdoptFiles(t.TempDir(), "cbz")
	if f2, _ := l.Get(url); len(f2.Files) != len(f.Files) {
		t.Errorf("files changed on a second look: %+v", f2.Files)
	}
}

// A download skips the chosen chapters already in the folder
func TestDropSavedSkipsTheChaptersAlreadyThere(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "X 2 - Two.cbz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	url := "https://example.com/manga/x"
	l := NewLibrary("")
	_ = l.Follow(Followed{URL: url, Title: "X"})
	_ = l.MarkSavedFiles(url, []float64{2}, []SavedFile{{Number: 2, Dir: dir, Format: "cbz", Name: "X 2 - Two.cbz"}}, nil)
	c := &Controller{library: l, url: url, settings: &grabber.Settings{OutputDir: dir, Format: "cbz"}}
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 1, Title: "One"},
		&grabber.Chapter{Number: 2, Title: "Two"},
		&grabber.Chapter{Number: 3, Title: "Three"},
	}
	kept, dropped := c.dropSavedLocked(chapters)
	if dropped != 1 || len(kept) != 2 || kept[0].GetNumber() != 1 || kept[1].GetNumber() != 3 {
		t.Errorf("kept %d, dropped %d", len(kept), dropped)
	}
	// another folder has none of them
	c.settings.OutputDir = t.TempDir()
	if _, dropped := c.dropSavedLocked(chapters); dropped != 0 {
		t.Errorf("dropped %d in another folder", dropped)
	}
}
