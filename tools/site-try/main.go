// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Site-try checks that the downloader reads a site end to end, without
// saving anything: for each series URL it finds the grabber, the title and
// the chapter list, then the pages of one chapter, and fetches that
// chapter's first page. It prints one JSON line per URL. Not part of the app.
//
//	go run ./tools/site-try URL...        or a URL per line on stdin
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"os"
	"strings"
	"time"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
)

// Result is what one URL got to; Step is the last step reached ("page" when
// everything worked) and Error why it stopped there
type Result struct {
	URL      string `json:"url"`
	Grabber  string `json:"grabber,omitempty"`
	Title    string `json:"title,omitempty"`
	Chapters int    `json:"chapters"`
	Pages    int    `json:"pages"`
	Bytes    int    `json:"bytes"`
	Type     string `json:"type,omitempty"`
	Step     string `json:"step"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
}

const perURL = 90 * time.Second

func main() {
	urls := os.Args[1:]
	if len(urls) == 0 {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if u := strings.TrimSpace(sc.Text()); u != "" {
				urls = append(urls, u)
			}
		}
	}
	out := json.NewEncoder(os.Stdout)
	for _, u := range urls {
		done := make(chan Result, 1)
		go func() { done <- try(u) }()
		select {
		case r := <-done:
			_ = out.Encode(r)
		case <-time.After(perURL):
			_ = out.Encode(Result{URL: u, Step: "timeout", Error: fmt.Sprintf("no answer in %s", perURL)})
		}
	}
	browser.Close()
}

func try(u string) (r Result) {
	r.URL = u
	defer func() {
		if p := recover(); p != nil {
			r.Error = fmt.Sprint("panic: ", p)
		}
	}()
	r.Step = "identify"
	site, errs := core.Identify(u, &grabber.Settings{})
	if len(errs) > 0 || site == nil {
		r.Error = joinErrs(errs)
		return
	}
	r.Grabber = strings.TrimPrefix(fmt.Sprintf("%T", site), "*grabber.")
	r.Step = "title"
	title, err := site.FetchTitle()
	if err != nil {
		r.Error = err.Error()
		return
	}
	r.Title = title
	r.Step = "chapters"
	chapters, errs := site.FetchChapters()
	r.Chapters = len(chapters)
	if len(chapters) == 0 {
		r.Error = joinErrs(errs)
		if r.Error == "" {
			r.Error = "no chapters"
		}
		return
	}
	// locked (paid) chapters are left out, as the CLI and the app do
	if open, _ := core.SplitLocked(chapters); len(open) > 0 {
		chapters = open
	}
	r.Step = "chapter"
	ch, err := site.FetchChapter(chapters[0])
	if err != nil {
		r.Error = err.Error()
		return
	}
	r.Pages = len(ch.Pages)
	if r.Pages == 0 {
		r.Error = "no pages"
		return
	}
	r.Step = "page"
	p := ch.Pages[0]
	referer := site.BaseUrl()
	if p.NoReferer {
		referer = ""
	}
	f, err := downloader.FetchFile(context.Background(), http.RequestParams{URL: p.URL, Referer: referer, Headers: p.Headers, Accept: http.ImageAccept}, 1, 1, p.Transform)
	if err != nil {
		r.Error = err.Error()
		return
	}
	r.Bytes = len(f.Data)
	r.Type = stdhttp.DetectContentType(f.Data)
	r.OK = strings.HasPrefix(r.Type, "image/") && r.Bytes > 1000
	if !r.OK {
		r.Error = "the first page isn't a picture"
	}
	return
}

func joinErrs(errs []error) string {
	var s []string
	for _, e := range errs {
		if e != nil {
			s = append(s, e.Error())
		}
	}
	return strings.Join(s, "; ")
}
