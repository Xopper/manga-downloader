// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"strings"
	"testing"
)

// errorLines is the log's error lines
func errorLines(c *Controller) []string {
	var out []string
	for _, l := range c.Log() {
		if l.Level == LogError {
			out = append(out, l.Text)
		}
	}
	return out
}

func TestAChapterWhosePagesAreAllGoneSaysSoOnce(t *testing.T) {
	gone := map[int]bool{0: true, 1: true, 2: true}
	c, _, _ := newTestController(t, false, nil, fakeChapter{number: 1, pages: 3, gone: gone})
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if _, err := c.Start(); err != nil {
		t.Fatal(err)
	}
	s := waitFinished(t, c)
	lines := errorLines(c)
	if len(lines) != 1 || !strings.Contains(lines[0], "the site has lost this chapter's pages (all 3 are missing, 404)") {
		t.Fatalf("error lines = %q", lines)
	}
	if len(s.Failures) != 1 || s.Failures[0] != lines[0] {
		t.Errorf("failures = %q", s.Failures)
	}
	if j := c.Jobs(); len(j) != 1 || j[0].State != JobFailed || j[0].Reason != "the site has lost its pages" {
		t.Errorf("jobs = %+v", j)
	}
}

func TestAChapterMissingSomePagesNamesThem(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, fakeChapter{number: 1, pages: 4, gone: map[int]bool{1: true, 3: true}})
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if _, err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)
	lines := errorLines(c)
	if len(lines) != 1 || !strings.Contains(lines[0], "the site is missing pages 2 and 4 of 4 (404)") {
		t.Fatalf("error lines = %q", lines)
	}
}

func TestAPageFailingOtherwiseKeepsTheRawLines(t *testing.T) {
	// a 500 isn't a lost page: the lines stay as they were
	c, _, _ := newTestController(t, false, nil, fakeChapter{number: 1, pages: 2, fail: map[int]bool{0: true}})
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if _, err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)
	for _, l := range errorLines(c) {
		if strings.Contains(l, "the site") {
			t.Errorf("a 500 was explained as lost pages: %q", l)
		}
	}
}

func TestPageList(t *testing.T) {
	cases := map[string][]int64{
		"page 3":           {3},
		"pages 3 and 5":    {3, 5},
		"pages 1, 3 and 5": {1, 3, 5},
		"pages 1, 2, 3, 4, 5, 6, 7, 8 and 2 more": {1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}
	for want, in := range cases {
		if got := pageList(in); got != want {
			t.Errorf("pageList(%v) = %q, want %q", in, got, want)
		}
	}
}
