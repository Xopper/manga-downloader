// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	mdhttp "github.com/elboletaire/manga-downloader/http"
)

// A chapter whose page answers 404 fails at that page: the downloader asks
// for no more. The site may have lost one page or all of them, and the raw
// lines ("error downloading page 5 of …: received 404 response code") don't
// say which. So the controller holds a 404 chapter's lines back, asks the
// site once for each of its pages, and logs one line that says it.

// is404 reports whether err is, or wraps, a 404 answer
func is404(err error) bool {
	var status *mdhttp.StatusError
	return errors.As(err, &status) && status.Code == 404
}

// pageStatus asks the site for one page and returns the answer's status:
// 200 when it came, the code of a StatusError, 0 for any other failure. A
// var so tests can stand in for the network.
var pageStatus = func(page grabber.Page, referer string) int {
	if page.NoReferer {
		referer = ""
	}
	body, err := mdhttp.Get(mdhttp.RequestParams{
		URL:          page.URL,
		Referer:      referer,
		Headers:      page.Headers,
		Accept:       mdhttp.ImageAccept,
		DisableRetry: true,
		Timeout:      15 * time.Second,
	})
	if err == nil {
		// the answer is all that's wanted, not the picture
		body.Close()
		return 200
	}
	var status *mdhttp.StatusError
	if errors.As(err, &status) {
		return status.Code
	}
	return 0
}

// missingPages asks the site for every page of ch, four at a time, and
// returns the numbers of those it answers 404 for, and whether every other
// page came (false when one failed some other way)
func missingPages(ch *grabber.Chapter, referer string) (missing []int64, others bool) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	others = true
	sem := make(chan struct{}, 4)
	for _, page := range ch.Pages {
		wg.Add(1)
		sem <- struct{}{}
		go func(page grabber.Page) {
			defer wg.Done()
			defer func() { <-sem }()
			code := pageStatus(page, referer)
			mu.Lock()
			defer mu.Unlock()
			switch code {
			case 404:
				missing = append(missing, page.Number)
			case 200:
			default:
				others = false
			}
		}(page)
	}
	wg.Wait()
	slices.Sort(missing)
	return missing, others
}

// lostMessage is the one line for a 404 chapter: what the site lost, and
// the job's shorter reason. ok is false when the check doesn't support a
// plain statement (a page failed some other way, or none answered 404),
// and the raw lines are logged instead.
func lostMessage(label string, total int, missing []int64, others bool) (line, reason string, ok bool) {
	if len(missing) == 0 || !others {
		return "", "", false
	}
	if len(missing) == total {
		return fmt.Sprintf("%s: the site has lost this chapter's pages (all %d are missing, 404). Try another site for it.", label, total),
			"the site has lost its pages", true
	}
	pages := pageList(missing)
	return fmt.Sprintf("%s: the site is missing %s of %d (404), so the chapter can't be saved whole.", label, pages, total),
		"the site is missing " + pages, true
}

// pageList is "page 3", "pages 3 and 5", "pages 1, 3 and 5", or the first
// eight and how many more
func pageList(numbers []int64) string {
	if len(numbers) == 1 {
		return "page " + strconv.FormatInt(numbers[0], 10)
	}
	shown := numbers
	more := 0
	if len(shown) > 8 {
		shown, more = shown[:8], len(shown)-8
	}
	parts := make([]string, len(shown))
	for i, n := range shown {
		parts[i] = strconv.FormatInt(n, 10)
	}
	if more > 0 {
		return fmt.Sprintf("pages %s and %d more", strings.Join(parts, ", "), more)
	}
	return "pages " + strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
