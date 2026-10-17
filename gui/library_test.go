// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestURLKeyNormalises(t *testing.T) {
	cases := map[string]string{
		"https://WWW.Example.com/manga/x/":   "https://example.com/manga/x",
		"  https://example.com/manga/x  ":    "https://example.com/manga/x",
		"HTTPS://example.com/manga/x#top":    "https://example.com/manga/x",
		"https://example.com/manga/x?page=2": "https://example.com/manga/x?page=2",
	}
	for raw, want := range cases {
		if got := urlKey(raw); got != want {
			t.Errorf("urlKey(%q) = %q, want %q", raw, got, want)
		}
	}
	if urlKey("https://example.com/manga/X") == urlKey("https://example.com/manga/x") {
		t.Error("the path's case must be kept: sites tell slugs apart by it")
	}
	if got := hostOf("https://www.Example.com/a"); got != "example.com" {
		t.Errorf("hostOf = %q", got)
	}
}

func TestFollowUnfollowPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	l := NewLibrary(path)
	if err := l.Follow(Followed{URL: "https://www.example.com/manga/x/", Title: "X", Downloadable: true}); err != nil {
		t.Fatal(err)
	}
	again := NewLibrary(path)
	if err := again.Load(); err != nil {
		t.Fatal(err)
	}
	list := again.List()
	if len(list) != 1 || list[0].Title != "X" || list[0].Site != "example.com" || list[0].FollowedAt.IsZero() {
		t.Fatalf("list after reload = %+v", list)
	}
	if _, ok := again.Get("https://example.com/manga/x"); !ok {
		t.Error("Get must match the URL in another spelling")
	}
	if err := again.Unfollow("https://example.com/manga/x"); err != nil {
		t.Fatal(err)
	}
	third := NewLibrary(path)
	if err := third.Load(); err != nil {
		t.Fatal(err)
	}
	if len(third.List()) != 0 {
		t.Errorf("still followed after unfollow: %+v", third.List())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestMarkSeenUpdatesCountsAndNew(t *testing.T) {
	l := NewLibrary("")
	url := "https://example.com/manga/x"
	if err := l.Follow(Followed{URL: url, SavedUpTo: 3, Saved: []float64{1, 2, 3}, Baselined: true}); err != nil {
		t.Fatal(err)
	}
	if err := l.MarkSeen(url, "X", []float64{1, 2, 3, 4, 5}); err != nil {
		t.Fatal(err)
	}
	f, _ := l.Get(url)
	if f.Title != "X" || f.Chapters != 5 || f.Latest != 5 || f.New != 2 || f.CheckedAt.IsZero() {
		t.Errorf("entry after MarkSeen = %+v", f)
	}
	seen, ok := l.SeenOf("https://www.example.com/manga/x/")
	if !ok || seen.Chapters != 5 || seen.Title != "X" {
		t.Errorf("seen = %+v, %v", seen, ok)
	}
	// a series not followed is still remembered
	if err := l.MarkSeen("https://other.test/s", "S", []float64{1}); err != nil {
		t.Fatal(err)
	}
	if s, ok := l.SeenOf("https://other.test/s"); !ok || s.Chapters != 1 {
		t.Errorf("unfollowed series not seen: %+v", s)
	}
	if _, ok := l.Get("https://other.test/s"); ok {
		t.Error("MarkSeen must not follow")
	}
}

func TestFollowTakesTheCountsAlreadySeen(t *testing.T) {
	l := NewLibrary("")
	url := "https://example.com/manga/x"
	if err := l.MarkSeen(url, "X", []float64{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if err := l.Follow(Followed{URL: url}); err != nil {
		t.Fatal(err)
	}
	f, _ := l.Get(url)
	if f.Title != "X" || f.Chapters != 3 || f.Latest != 3 {
		t.Errorf("entry = %+v", f)
	}
}

func TestMarkSavedNeverGoesBack(t *testing.T) {
	l := NewLibrary("")
	url := "https://example.com/manga/x"
	numbers := []float64{1, 2, 3, 4, 5, 6}
	if err := l.Follow(Followed{URL: url, Baselined: true}); err != nil {
		t.Fatal(err)
	}
	if err := l.MarkSeen(url, "X", numbers); err != nil {
		t.Fatal(err)
	}
	if f, _ := l.Get(url); f.New != 6 {
		t.Fatalf("nothing saved yet: New = %d, want 6", f.New)
	}
	if err := l.MarkSaved(url, []float64{5}, numbers); err != nil {
		t.Fatal(err)
	}
	if err := l.MarkSaved(url, []float64{4}, numbers); err != nil {
		t.Fatal(err)
	}
	f, _ := l.Get(url)
	if f.SavedUpTo != 5 || f.New != 4 || f.LastSaved.IsZero() {
		t.Errorf("entry = %+v", f)
	}
	if err := l.MarkSaved("https://not.followed/x", []float64{9}, numbers); err != nil {
		t.Errorf("MarkSaved of an unfollowed URL must be a no-op, got %v", err)
	}
}

func TestMarkFailedKeepsTheCounts(t *testing.T) {
	l := NewLibrary("")
	url := "https://example.com/manga/x"
	_ = l.Follow(Followed{URL: url})
	_ = l.MarkSeen(url, "X", []float64{1, 2})
	if err := l.MarkFailed(url, fmt.Errorf("site down")); err != nil {
		t.Fatal(err)
	}
	f, _ := l.Get(url)
	if f.Error != "site down" || f.Chapters != 2 {
		t.Errorf("entry = %+v", f)
	}
	_ = l.MarkSeen(url, "X", []float64{1, 2, 3})
	if f, _ := l.Get(url); f.Error != "" {
		t.Errorf("a later success must clear the error: %+v", f)
	}
}

func TestSeenIsCapped(t *testing.T) {
	l := NewLibrary("")
	for i := 0; i < seenCap+20; i++ {
		_ = l.MarkSeen(fmt.Sprintf("https://example.com/manga/%d", i), "S", []float64{1})
		// the newest entries must survive: give them a later time
		l.mu.Lock()
		s := l.seen[urlKey(fmt.Sprintf("https://example.com/manga/%d", i))]
		s.At = time.Unix(int64(i), 0)
		l.seen[urlKey(fmt.Sprintf("https://example.com/manga/%d", i))] = s
		l.mu.Unlock()
	}
	_ = l.MarkSeen("https://example.com/manga/last", "L", []float64{1})
	l.mu.Lock()
	n := len(l.seen)
	l.mu.Unlock()
	if n != seenCap {
		t.Errorf("seen holds %d, want %d", n, seenCap)
	}
	if _, ok := l.SeenOf("https://example.com/manga/0"); ok {
		t.Error("the oldest entry must go first")
	}
	if _, ok := l.SeenOf("https://example.com/manga/last"); !ok {
		t.Error("the newest entry must stay")
	}
}

func TestBadFileIsBackedUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	l := NewLibrary(path)
	if err := l.Load(); err == nil {
		t.Fatal("a bad file must be reported")
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("the bad file was not moved aside: %v", err)
	}
	if len(l.List()) != 0 {
		t.Error("a bad file must leave the library empty")
	}
	if err := l.Follow(Followed{URL: "https://example.com/manga/x"}); err != nil {
		t.Fatal(err)
	}
	again := NewLibrary(path)
	if err := again.Load(); err != nil || len(again.List()) != 1 {
		t.Errorf("a fresh file wasn't written: %v, %d entries", err, len(again.List()))
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	l := NewLibrary(filepath.Join(t.TempDir(), "none.json"))
	if err := l.Load(); err != nil || len(l.List()) != 0 {
		t.Errorf("Load of a missing file: %v, %d entries", err, len(l.List()))
	}
}

func TestNewIsWhatCameAfterTheFollowUntilSaved(t *testing.T) {
	l := NewLibrary("")
	url := "https://example.com/manga/x"
	_ = l.Follow(Followed{URL: url})
	// the first list seen is known, whatever was saved
	_ = l.MarkSeen(url, "X", []float64{0, 1, 2, 3})
	if f, _ := l.Get(url); f.New != 0 || !f.Baselined || !slices.Equal(f.Known, []float64{0, 1, 2, 3}) {
		t.Fatalf("after the first list: %+v", f)
	}
	// saving one chapter makes nothing else new
	_ = l.MarkSaved(url, []float64{2}, []float64{0, 1, 2, 3})
	if f, _ := l.Get(url); f.New != 0 {
		t.Fatalf("saving 2 made %d new", f.New)
	}
	// chapters listed later are new until saved
	numbers := []float64{0, 1, 2, 3, 4, 5}
	_ = l.MarkSeen(url, "X", numbers)
	f, _ := l.Get(url)
	if f.New != 2 || !slices.Equal(f.NewNumbers(numbers), []float64{4, 5}) {
		t.Fatalf("after 4 and 5 came out: New %d, %v", f.New, f.NewNumbers(numbers))
	}
	_ = l.MarkSaved(url, []float64{4}, numbers)
	if f, _ := l.Get(url); f.New != 1 || !slices.Equal(f.NewNumbers(numbers), []float64{5}) {
		t.Errorf("after saving 4: %+v", f)
	}
	// an entry without its known list has nothing new yet
	if got := (&Followed{}).NewNumbers(numbers); got != nil {
		t.Errorf("unbaselined: %v", got)
	}
}

func TestRefollowKeepsTheKnownList(t *testing.T) {
	l := NewLibrary("")
	url := "https://example.com/manga/x"
	_ = l.Follow(Followed{URL: url})
	_ = l.MarkSeen(url, "X", []float64{1, 2})
	_ = l.MarkSeen(url, "X", []float64{1, 2, 3})
	_ = l.Follow(Followed{URL: url, Title: "X again"})
	if f, _ := l.Get(url); !f.Baselined || f.New != 1 || !slices.Equal(f.Known, []float64{1, 2}) {
		t.Errorf("after a re-follow: %+v", f)
	}
}

func TestMarkSavedKeepsTheChaptersSaved(t *testing.T) {
	// saving chapter 355 alone says nothing of the chapters below it
	l := NewLibrary("")
	url := "https://example.com/manga/x"
	numbers := []float64{353, 354, 355, 356, 357}
	_ = l.Follow(Followed{URL: url, Known: numbers, Baselined: true})
	_ = l.MarkSeen(url, "X", numbers)
	if err := l.MarkSaved(url, []float64{355}, numbers); err != nil {
		t.Fatal(err)
	}
	if err := l.MarkSaved(url, []float64{357, 353, 355}, numbers); err != nil {
		t.Fatal(err)
	}
	f, _ := l.Get(url)
	if !slices.Equal(f.Saved, []float64{353, 355, 357}) || f.SavedUpTo != 357 || f.New != 0 {
		t.Errorf("entry = %+v", f)
	}
	// with no file recorded, no folder holds them
	if dto := followedDTO(f, "/out", "cbz", func(string) bool { return true }); len(dto.Saved) != 0 {
		t.Errorf("dto saved = %v", dto.Saved)
	}
	if dto := followedDTO(Followed{URL: url}, "/out", "cbz", nil); dto.Saved == nil {
		t.Error("the window gets [] for nothing saved, not null")
	}
}

func TestLoadTakesAnOldMarkAsOneChapterSaved(t *testing.T) {
	// a library from before Saved knows only the highest chapter saved
	path := filepath.Join(t.TempDir(), "library.json")
	old := `{"followed":[{"url":"https://example.com/manga/x","savedUpTo":355},{"url":"https://example.com/manga/y"}],"seen":{}}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	l := NewLibrary(path)
	if err := l.Load(); err != nil {
		t.Fatal(err)
	}
	if f, _ := l.Get("https://example.com/manga/x"); !slices.Equal(f.Saved, []float64{355}) {
		t.Errorf("x saved = %v", f.Saved)
	}
	if f, _ := l.Get("https://example.com/manga/y"); f.Saved != nil {
		t.Errorf("y saved = %v", f.Saved)
	}
}

// a series followed while its site was search-only becomes downloadable once
// the site is read, and none is ever turned back
func TestMarkDownloadable(t *testing.T) {
	l := NewLibrary("")
	_ = l.Follow(Followed{URL: "https://akuma.moe/g/x", Site: "akuma.moe"})
	_ = l.Follow(Followed{URL: "https://kemono.cr/patreon/user/1", Site: "kemono.cr"})
	_ = l.Follow(Followed{URL: "https://gone.example/manga/y", Site: "gone.example", Downloadable: true})
	if err := l.MarkDownloadable(func(site string) bool { return site == "akuma.moe" }); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"akuma.moe": true, "kemono.cr": false, "gone.example": true}
	for _, f := range l.List() {
		if f.Downloadable != want[f.Site] {
			t.Errorf("%s downloadable = %v, want %v", f.Site, f.Downloadable, want[f.Site])
		}
	}
}
