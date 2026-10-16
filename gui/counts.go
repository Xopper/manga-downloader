// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"strings"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/grabber"
)

// The search shows a series' chapter count when the app knows it: one it
// loaded, follows, or whose catalog row carries the latest number. For the
// rest the window asks CountChapters for its top results, which fetches the
// list the way a library check does and remembers it, so the next search
// and the series' next open have it at once.

// countSlots bounds the counts in flight, whatever the window asks for
var countSlots = make(chan struct{}, 3)

// errNeedsWindow refuses a count from a site that shows the browser window:
// a search mustn't pop it up
var errNeedsWindow = errors.New("the site needs the browser window")

// CountChapters returns how many chapters url lists, fetching the list
// when the library hasn't seen it. The count is remembered (the library's
// seen list, and a followed entry's counts) and the list saved for the
// series' next open.
func (c *Controller) CountChapters(url string) (int, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return 0, errors.New("no link to count")
	}
	if seen, ok := c.library.SeenOf(url); ok && seen.Chapters > 0 {
		return seen.Chapters, nil
	}
	if browser.NeedsVisible(url) {
		return 0, errNeedsWindow
	}
	countSlots <- struct{}{}
	defer func() { <-countSlots }()
	c.mu.Lock()
	s := new(grabber.Settings)
	*s = *c.settings
	c.mu.Unlock()
	title, numbers, err := c.fetchNumbers(url, s)
	if err != nil {
		return 0, err
	}
	if lerr := c.library.MarkSeen(url, title, numbers); lerr != nil {
		c.addLog(LogWarn, "could not save the library: "+lerr.Error())
	}
	return len(numbers), nil
}

// CountChapters counts a search result's chapters (see Controller.CountChapters)
func (a *App) CountChapters(url string) (int, error) {
	return a.ctrl.CountChapters(url)
}
