// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

const libURL = "https://example.com/manga/test"

func TestLoadMarksTheSeriesSeen(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 2)...)
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	seen, ok := c.Library().SeenOf(testURL)
	if !ok || seen.Chapters != 5 || seen.Latest != 5 || seen.Title != "Test Series" {
		t.Errorf("seen = %+v, %v", seen, ok)
	}
}

func TestLoadUpdatesAFollowedSeries(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 2)...)
	if err := c.Library().Follow(Followed{URL: testURL, SavedUpTo: 3, Downloadable: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	f, _ := c.Library().Get(testURL)
	if f.Title != "Test Series" || f.Chapters != 5 || f.New != 2 || f.CheckedAt.IsZero() {
		t.Errorf("entry = %+v", f)
	}
}

func TestRunRaisesTheSavedMark(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 2)...)
	if err := c.Library().Follow(Followed{URL: testURL, Downloadable: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SelectRange("1-3"); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	s := waitFinished(t, c)
	if s.Saved != 3 {
		t.Fatalf("saved %d", s.Saved)
	}
	f, _ := c.Library().Get(testURL)
	if f.SavedUpTo != 3 || f.New != 2 || f.LastSaved.IsZero() {
		t.Errorf("entry = %+v", f)
	}
}

func TestCancelledRunBeforeAnythingSavedKeepsTheMark(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(3, 2)...)
	if err := c.Library().Follow(Followed{URL: testURL, Downloadable: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-site.entered
	c.Cancel()
	close(gate)
	waitFinished(t, c)
	f, _ := c.Library().Get(testURL)
	if f.SavedUpTo != 0 && f.SavedUpTo > 3 {
		t.Errorf("entry = %+v", f)
	}
	if f.New != 3-int(f.SavedUpTo) {
		t.Errorf("New %d doesn't match the mark %v", f.New, f.SavedUpTo)
	}
}

func TestCheckLibraryUpdatesEveryFollowedSeries(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 2)...)
	other := "https://example.com/manga/other"
	lib := c.Library()
	_ = lib.Follow(Followed{URL: testURL, SavedUpTo: 4, Downloadable: true})
	_ = lib.Follow(Followed{URL: other, Downloadable: true})
	_ = lib.Follow(Followed{URL: "https://search.only/x", Downloadable: false})
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SelectRange("2-3"); err != nil {
		t.Fatal(err)
	}
	if err := c.CheckLibrary(context.Background()); err != nil {
		t.Fatal(err)
	}
	if on, done, total := c.Checking(); on || done != 2 || total != 2 {
		t.Errorf("Checking = %v, %d of %d", on, done, total)
	}
	f, _ := lib.Get(testURL)
	if f.Chapters != 5 || f.New != 1 {
		t.Errorf("first entry = %+v", f)
	}
	g, _ := lib.Get(other)
	if g.Chapters != 5 || g.New != 5 || g.Title != "Test Series" {
		t.Errorf("second entry = %+v", g)
	}
	if c.Title() != "Test Series" || len(c.Chapters()) != 5 || c.SelectedCount() != 2 || c.URL() != testURL {
		t.Errorf("the loaded series changed: %q, %d chapters, %d selected", c.Title(), len(c.Chapters()), c.SelectedCount())
	}
}

func TestCheckLibraryGoesOnPastAFailure(t *testing.T) {
	c, site, _ := newTestController(t, false, nil, chapters(2, 1)...)
	bad := "https://example.com/manga/bad"
	lib := c.Library()
	_ = lib.Follow(Followed{URL: bad, Title: "Bad", Downloadable: true})
	_ = lib.Follow(Followed{URL: testURL, Downloadable: true})
	c.identify = func(url string, s *grabber.Settings) (grabber.Site, []error) {
		if url == bad {
			return nil, nil
		}
		site.Settings = s
		return site, nil
	}
	if err := c.CheckLibrary(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := lib.Get(bad)
	if b.Error != "site not recognised" || b.CheckedAt.IsZero() {
		t.Errorf("bad entry = %+v", b)
	}
	g, _ := lib.Get(testURL)
	if g.Chapters != 2 || g.Error != "" {
		t.Errorf("good entry = %+v", g)
	}
}

func TestCheckIsRefusedWhileRunningAndTheOtherWayRound(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(2, 1)...)
	_ = c.Library().Follow(Followed{URL: testURL, Downloadable: true})
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-site.entered
	if err := c.CheckLibrary(context.Background()); !errors.Is(err, errBusy) {
		t.Errorf("CheckLibrary during a run = %v, want errBusy", err)
	}
	close(gate)
	waitFinished(t, c)

	// a check that hangs on its site: loads are refused meanwhile
	hold := make(chan struct{})
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		<-hold
		site.Settings = s
		return site, nil
	}
	done := make(chan error, 1)
	go func() { done <- c.CheckLibrary(context.Background()) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if on, _, _ := c.Checking(); on {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the check never started")
		}
		time.Sleep(time.Millisecond)
	}
	if err := c.BeginLoad(testURL); !errors.Is(err, errBusy) {
		t.Errorf("BeginLoad during a check = %v, want errBusy", err)
	}
	if err := c.Start(); !errors.Is(err, errBusy) {
		t.Errorf("Start during a check = %v, want errBusy", err)
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCheckLibraryWithNothingToCheck(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(1, 1)...)
	_ = c.Library().Follow(Followed{URL: "https://search.only/x", Downloadable: false})
	if err := c.CheckLibrary(context.Background()); err == nil {
		t.Error("a library with no readable series must be refused")
	}
}

func TestSelectAbove(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 1)...)
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	n, err := c.SelectAbove(3)
	if err != nil || n != 2 {
		t.Fatalf("SelectAbove = %d, %v", n, err)
	}
	if c.IsSelected(2) || !c.IsSelected(3) || !c.IsSelected(4) {
		t.Error("the wrong chapters are selected")
	}
}
