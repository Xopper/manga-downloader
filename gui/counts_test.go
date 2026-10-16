// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import "testing"

func TestCountChaptersFetchesOnceAndRemembers(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(4, 1)...)
	n, err := c.CountChapters(testURL)
	if err != nil || n != 4 {
		t.Fatalf("CountChapters = %d, %v", n, err)
	}
	seen, ok := c.Library().SeenOf(testURL)
	if !ok || seen.Chapters != 4 || seen.Title != "Test Series" {
		t.Errorf("seen = %+v, %v", seen, ok)
	}
	// nothing loaded: the count leaves the window's series alone
	if c.Loaded() {
		t.Error("a count loaded the series")
	}
	if n, err := c.CountChapters(testURL); err != nil || n != 4 {
		t.Errorf("second count = %d, %v", n, err)
	}
	if _, err := c.CountChapters("  "); err == nil {
		t.Error("an empty link must be refused")
	}
}
