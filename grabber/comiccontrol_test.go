// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestComicControl serves trimmed copies of egscomics.com's archive and
// strip pages, and a non-ComicControl site under /blog
func newTestComicControl(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/egsnp/archive":
			fmt.Fprint(w, `<html><head><title>El Goonish Shive - EGS:NP - Archive</title>`+
				`<link rel="stylesheet" href="https://www.egscomics.com/comiccontrol/defaultstyles.css"></head><body>`+
				`<select name="comic"><option value="">Select a comic...</option>`+
				`<option value="egsnp/2004-02-24">February 24, 2004 - 2004-02-24</option>`+
				`<option value="comic/other">March 1, 2004 - Other comic</option>`+
				`<option value="egsnp/masks-029">September 8, 2026 - masks-029</option></select></body></html>`)
		case "/egsnp/masks-029":
			fmt.Fprint(w, `<div id="cc-comicbody"><a href="/egsnp/masks-030"><img title="" src="/comics/1788841496-masks-029.png" id="cc-comic" /></a></div>`)
		case "/blog/archive":
			fmt.Fprint(w, `<html><head><title>Blog</title></head><select name="comic"><option value="blog/a">a</option></select></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestComicControlTest(t *testing.T) {
	srv := newTestComicControl(t)
	cases := map[string]bool{
		"/egsnp/archive":    true,
		"/egsnp/masks-029":  true,
		"/blog/":            false,
		"/missing/whatever": false,
		"/":                 false,
	}
	for path, want := range cases {
		got, err := NewComicControl(&Grabber{URL: srv.URL + path, Settings: &Settings{}}).Test()
		if err != nil || got != want {
			t.Errorf("Test(%q) = %v, %v, want %v", path, got, err, want)
		}
	}
}

func TestComicControlFetch(t *testing.T) {
	srv := newTestComicControl(t)
	c := NewComicControl(&Grabber{URL: srv.URL + "/egsnp/masks-029", Settings: &Settings{}})

	title, err := c.FetchTitle()
	if err != nil || title != "El Goonish Shive - EGS:NP" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := c.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	last := chapters[1].(*ComicControlChapter)
	if last.Number != 2 || last.Title != "masks-029" || last.ID != "egsnp/masks-029" {
		t.Errorf("last = %+v", last)
	}
	chapter, err := c.FetchChapter(last)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 1 || chapter.Pages[0].URL != srv.URL+"/comics/1788841496-masks-029.png" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
