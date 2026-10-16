// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

func TestListStoreSavesAndReadsAList(t *testing.T) {
	s := newListStore(filepath.Join(t.TempDir(), "lists"))
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 1, Title: "One", Language: "en"},
		&grabber.Chapter{Number: 2, Title: "Two", Language: "en"},
	}
	if _, ok := s.Get("https://example.com/manga/test"); ok {
		t.Fatal("a list before any save")
	}
	if err := s.Put("https://example.com/manga/test", "Test", chapters); err != nil {
		t.Fatal(err)
	}
	// another spelling of the URL finds it
	l, ok := s.Get("https://www.example.com/manga/test/")
	if !ok {
		t.Fatal("the saved list wasn't found")
	}
	if l.Title != "Test" || len(l.Chapters) != 2 || l.Chapters[1].Title != "Two" || l.Chapters[1].Language != "en" {
		t.Errorf("read back %+v", l)
	}
	f := l.filterables()
	if f[0].GetNumber() != 1 || f[1].GetTitle() != "Two" {
		t.Errorf("filterables: %v", f)
	}
	if lang, ok := f[0].(interface{ GetLanguage() string }); !ok || lang.GetLanguage() != "en" {
		t.Error("a saved chapter doesn't report its language")
	}
	if err := s.Put("https://example.com/manga/test", "Test", nil); err == nil {
		t.Error("an empty list was saved")
	}
}

func TestListStoreIgnoresABrokenFile(t *testing.T) {
	s := newListStore(t.TempDir())
	if err := os.WriteFile(s.file("https://example.com/a"), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("https://example.com/a"); ok {
		t.Error("a broken file was read as a list")
	}
}

func TestListStoreDropsTheOldestPastTheCap(t *testing.T) {
	s := newListStore(t.TempDir())
	chapters := grabber.Filterables{&grabber.Chapter{Number: 1, Title: "One"}}
	for i := 0; i <= listStoreCap; i++ {
		url := "https://example.com/manga/" + string(rune('a'+i%26)) + "/" + time.Duration(i).String()
		if err := s.Put(url, "T", chapters); err != nil {
			t.Fatal(err)
		}
		// the first file is the oldest by a clear margin
		if i == 0 {
			old := time.Now().Add(-time.Hour)
			_ = os.Chtimes(s.file(url), old, old)
		}
	}
	entries, _ := os.ReadDir(s.dir)
	if len(entries) != listStoreCap {
		t.Errorf("%d files kept, want %d", len(entries), listStoreCap)
	}
	if _, ok := s.Get("https://example.com/manga/a/0s"); ok {
		t.Error("the oldest list is still there")
	}
}
