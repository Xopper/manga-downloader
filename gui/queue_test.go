// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

// startGated loads the test series, selects the chapters at indices and
// starts them, then waits for the fake site to block on its gate
func startGated(t *testing.T, c *Controller, site *fakeSite, indices ...int) {
	t.Helper()
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSelectedMany(indices, true); err != nil {
		t.Fatal(err)
	}
	if queued, err := c.Start(); err != nil || queued {
		t.Fatalf("Start = %v, %v; want started", queued, err)
	}
	<-site.entered
}

// waitIdle waits until nothing runs and nothing waits
func waitIdle(t *testing.T, c *Controller) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !c.Running() && len(c.Queue()) == 0 && c.Summary() != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the queue never drained")
}

func TestStartWhileRunningQueuesAndTheNextStartsWhenItEnds(t *testing.T) {
	gate := make(chan struct{})
	c, site, dir := newTestController(t, false, gate, chapters(3, 1)...)
	startGated(t, c, site, 0)
	if c.SelectedCount() != 0 {
		t.Errorf("Start kept the selection: %d selected", c.SelectedCount())
	}
	first := c.Run()
	if first.ID != 1 || first.Title != "Test Series" || first.URL != testURL || first.Chapters != 1 {
		t.Errorf("run = %+v", first)
	}

	// the selection works during the run, and Download queues it
	if err := c.SetSelectedMany([]int{1, 2}, true); err != nil {
		t.Fatalf("selecting during a run = %v", err)
	}
	queued, err := c.Start()
	if err != nil || !queued {
		t.Fatalf("Start during a run = %v, %v; want queued", queued, err)
	}
	q := c.Queue()
	if len(q) != 1 || q[0].ID != 2 || q[0].Chapters != 2 || q[0].Title != "Test Series" || q[0].Fresh {
		t.Fatalf("queue = %+v", q)
	}
	if !c.Running() || c.Run().ID != 1 {
		t.Error("the queued job replaced the running one")
	}

	close(gate)
	waitIdle(t, c)
	if r := c.Run(); r.ID != 2 || r.Chapters != 2 {
		t.Errorf("the run shown after the queue drained = %+v, want job 2", r)
	}
	if s := c.Summary(); s.Saved != 2 {
		t.Errorf("job 2 saved %d, want 2", s.Saved)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.cbz"))
	if len(files) != 3 {
		t.Errorf("wrote %d cbz files, want 3", len(files))
	}
	log := fmt.Sprint(c.Log())
	if !strings.Contains(log, "Queued 2 chapter(s) of Test Series") {
		t.Errorf("log = %s", log)
	}
}

func TestSameChaptersQueuedTwiceAreRefused(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(3, 1)...)
	startGated(t, c, site, 0)
	c.SetSelectedMany([]int{1}, true)
	if _, err := c.Start(); err != nil {
		t.Fatal(err)
	}
	c.SetSelectedMany([]int{1}, true)
	if _, err := c.Start(); err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("the same chapters twice = %v, want a refusal", err)
	}
	// the one going is refused too (the refused selection is still picked:
	// it's cleared only when a job takes it)
	c.SetAll(false)
	c.SetSelectedMany([]int{0}, true)
	if _, err := c.Start(); err == nil {
		t.Error("the chapters of the running job were queued again")
	}
	if len(c.Queue()) != 1 {
		t.Errorf("queue = %+v", c.Queue())
	}
	close(gate)
	waitIdle(t, c)
}

func TestRemoveQueuedDropsAWaitingJob(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(3, 1)...)
	startGated(t, c, site, 0)
	c.SetSelectedMany([]int{1}, true)
	c.Start()
	c.SetSelectedMany([]int{2}, true)
	c.Start()
	q := c.Queue()
	if len(q) != 2 {
		t.Fatalf("queue = %+v", q)
	}
	if err := c.RemoveQueued(q[0].ID); err != nil {
		t.Fatal(err)
	}
	if q = c.Queue(); len(q) != 1 || q[0].ID != 3 {
		t.Errorf("queue after the remove = %+v", q)
	}
	if err := c.RemoveQueued(99); err == nil {
		t.Error("removing a job that isn't waiting succeeded")
	}
	if err := c.RemoveQueued(1); err == nil {
		t.Error("the running job was removed as if waiting")
	}
	close(gate)
	waitIdle(t, c)
	if r := c.Run(); r.ID != 3 {
		t.Errorf("the run shown = %+v, want job 3", r)
	}
}

func TestCancelEndsTheRunningJobAndStartsTheNext(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(3, 1)...)
	startGated(t, c, site, 0)
	c.SetSelectedMany([]int{1}, true)
	c.Start()
	c.Cancel()
	if !c.Cancelling() {
		t.Error("Cancel didn't mark the run")
	}
	// the gate holds the first job's chapter; once released it winds down and
	// the second starts (and blocks on the gate too, which is open by then)
	close(gate)
	waitIdle(t, c)
	if r := c.Run(); r.ID != 2 {
		t.Errorf("the run shown = %+v, want job 2", r)
	}
	if s := c.Summary(); s.Cancelled || s.Saved != 1 {
		t.Errorf("job 2's summary = %+v", s)
	}
}

func TestLoadingAnotherSeriesDuringARunLeavesItAlone(t *testing.T) {
	gate := make(chan struct{})
	c, site, dir := newTestController(t, false, gate, chapters(2, 1)...)
	startGated(t, c, site, 0, 1)

	// another site answers for the other series
	other := &fakeSite{Grabber: &grabber.Grabber{URL: otherURL}, server: site.server, chapters: chapters(5, 1), entered: make(chan struct{}, 16)}
	c.identify = func(url string, s *grabber.Settings) (grabber.Site, []error) {
		if url == otherURL {
			other.Settings = s
			return other, nil
		}
		site.Settings = s
		return site, nil
	}
	if err := c.Load(otherURL); err != nil {
		t.Fatalf("loading during a run = %v", err)
	}
	if c.URL() != otherURL || len(c.Chapters()) != 5 {
		t.Errorf("loaded %q with %d chapters", c.URL(), len(c.Chapters()))
	}
	// options change too; the run keeps its own
	if err := c.Configure(func(s *grabber.Settings) { s.Format = "epub" }); err != nil {
		t.Fatalf("Configure during a run = %v", err)
	}
	if r := c.Run(); r.URL != testURL || r.Title != "Test Series" || r.Chapters != 2 {
		t.Errorf("the run changed with the load: %+v", r)
	}

	close(gate)
	s := waitFinished(t, c)
	if s.Saved != 2 {
		t.Errorf("summary = %+v", s)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "*.cbz")); len(files) != 2 {
		t.Errorf("the run saved %d cbz files, want 2 (the format changed under it?)", len(files))
	}
	// and the window still shows the other series
	if c.URL() != otherURL || c.Settings().Format != "epub" {
		t.Errorf("the run's end changed the window: %q, %q", c.URL(), c.Settings().Format)
	}
}

func TestReloadingTheRunningSeriesGetsItsOwnSite(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(2, 1)...)
	startGated(t, c, site, 0)
	identified := 0
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		identified++
		fresh := &fakeSite{Grabber: &grabber.Grabber{URL: testURL, Settings: s}, server: site.server, chapters: site.chapters, entered: make(chan struct{}, 16)}
		return fresh, nil
	}
	// a filter change would reuse the listing's site: not the running job's
	if err := c.ConfigureLoad(func(s *grabber.Settings) { s.Language = "en" }, testURL); err != nil {
		t.Fatal(err)
	}
	if err := c.FinishLoad(testURL); err != nil {
		t.Fatal(err)
	}
	if identified != 1 {
		t.Errorf("identified %d times, want 1: the running job's site was reused", identified)
	}
	if site.Settings.Language != "" {
		t.Errorf("the running job's site reads language %q now", site.Settings.Language)
	}
	close(gate)
	waitFinished(t, c)
}

func TestCheckRefusedWhileARunGoesButLoadsAreNot(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(2, 1)...)
	_ = c.Library().Follow(Followed{URL: testURL, Downloadable: true})
	startGated(t, c, site, 0)
	if err := c.CheckLibrary(t.Context()); !errors.Is(err, errBusy) {
		t.Errorf("CheckLibrary during a run = %v, want errBusy", err)
	}
	if err := c.BeginLoad(testURL); err != nil {
		t.Errorf("BeginLoad during a run = %v, want allowed", err)
	}
	c.abortLoad(fmt.Errorf("cancelled"))
	if err := c.SetAll(true); err != nil {
		t.Errorf("SetAll during a run = %v, want allowed", err)
	}
	close(gate)
	waitFinished(t, c)
}

func TestShutdownDropsTheWaitingJobs(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(3, 1)...)
	startGated(t, c, site, 0)
	c.SetSelectedMany([]int{1}, true)
	c.Start()
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(gate)
	}()
	c.Shutdown()
	deadline := time.Now().Add(5 * time.Second)
	for c.Running() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(c.Queue()) != 0 || c.Running() {
		t.Errorf("after Shutdown: %d waiting, running %v", len(c.Queue()), c.Running())
	}
	if r := c.Run(); r.ID != 1 {
		t.Errorf("job 2 started while quitting: %+v", r)
	}
}

func TestFreshJobWaitsBehindASelectionJob(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(4, 1)...)
	_ = c.Library().Follow(Followed{URL: testURL, Title: "Test", Known: []float64{1, 2}, Baselined: true, New: 2, Downloadable: true})
	startGated(t, c, site, 0)
	queued, err := c.EnqueueNew(testURL)
	if err != nil || !queued {
		t.Fatalf("EnqueueNew during a run = %v, %v; want queued", queued, err)
	}
	q := c.Queue()
	// the load recorded the site's title and recounted what's new (3 and 4)
	if len(q) != 1 || !q[0].Fresh || q[0].Chapters != 2 || q[0].Title != "Test Series" {
		t.Fatalf("queue = %+v", q)
	}
	if _, err := c.EnqueueNew(testURL); err == nil {
		t.Error("the same fresh job was queued twice")
	}
	if n, err := c.EnqueueAllNew(); err == nil || n != 0 {
		t.Errorf("EnqueueAllNew with the only job queued already = %d, %v", n, err)
	}
	close(gate)
	waitIdle(t, c)
	// chapter 1 was saved by the first job, 2 is known: 3 and 4 are new
	if r, s := c.Run(), c.Summary(); !r.Fresh || r.Chapters != 2 || s.Saved != 2 {
		t.Errorf("run %+v, summary %+v", r, s)
	}
}

const otherURL = "https://example.com/manga/other"
