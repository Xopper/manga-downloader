// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// fakeFetch serves fixture pages by URL and 404s anything else, recording
// every URL asked for
type fakeFetch struct {
	mu    sync.Mutex
	pages map[string]string
	calls []string
}

func (f *fakeFetch) fetch(_ context.Context, url string) (*goquery.Document, error) {
	f.mu.Lock()
	f.calls = append(f.calls, url)
	page, ok := f.pages[url]
	f.mu.Unlock()
	if !ok {
		return nil, &http.StatusError{Code: 404}
	}

	return goquery.NewDocumentFromReader(strings.NewReader(page))
}

// withFakeFetch replaces the page fetcher for the test and disables pacing
func withFakeFetch(t *testing.T, pages map[string]string) *fakeFetch {
	t.Helper()
	f := &fakeFetch{pages: pages}
	prevFetch, prevPause := fetch, pagePause
	fetch, pagePause = f.fetch, 0
	t.Cleanup(func() { fetch, pagePause = prevFetch, prevPause })

	return f
}

// read returns a testdata fixture
func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func TestAbsURL(t *testing.T) {
	cases := map[string]string{
		"/manga/x/":         "https://a.test/manga/x/",
		"//a.test/manga/y/": "https://a.test/manga/y/",
		"https://b.test/z":  "https://b.test/z",
		"manga/w":           "https://a.test/manga/list-mode/manga/w",
		"":                  "",
		"  /manga/v/  ":     "https://a.test/manga/v/",
	}
	for in, want := range cases {
		if got := absURL("https://a.test/manga/list-mode/", in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestDedupeByPath(t *testing.T) {
	in := []Entry{
		{Title: "A", URL: "https://a.test/manga/a/"},
		{Title: "A again", URL: "https://www.a.test/manga/a"},
		{Title: "B", URL: "https://a.test/manga/b/"},
		{Title: "", URL: "https://a.test/manga/c/"},
		{Title: "No URL", URL: ""},
	}
	got := dedupe(in)
	if len(got) != 2 || got[0].Title != "A" || got[1].Title != "B" {
		t.Fatalf("got %+v", got)
	}
}

func TestPageURL(t *testing.T) {
	if got := pageURL("https://a.test/manga/page/{page}/", 3); got != "https://a.test/manga/page/3/" {
		t.Error(got)
	}
	if got := pageURL("https://a.test/manga/page/{page}/", 12); got != "https://a.test/manga/page/12/" {
		t.Error(got)
	}
	if got := pageURL("https://a.test/manga/list-mode/", 3); got != "https://a.test/manga/list-mode/" {
		t.Error(got)
	}
}

func TestCrawlerForKnowsFamilies(t *testing.T) {
	if _, ok := CrawlerFor(SiteInfo{Family: "nope"}); ok {
		t.Error("unknown family has a crawler")
	}
}

func TestFakeFetch404(t *testing.T) {
	f := withFakeFetch(t, nil)
	_, err := fetch(context.Background(), "https://a.test/none")
	if se, ok := err.(*http.StatusError); !ok || se.Code != 404 {
		t.Fatalf("got %v", err)
	}
	if len(f.calls) != 1 {
		t.Error("call not recorded")
	}
}
