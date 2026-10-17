// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComiclandResolveSeriesURL(t *testing.T) {
	cases := []struct{ input, want string }{
		{"https://comicland.org/comic/i-want-to-prey-on-you/chapter/50", "https://comicland.org/comic/i-want-to-prey-on-you"},
		{"https://comicland.org/comic/i-want-to-prey-on-you/chapter/50/?x=1", "https://comicland.org/comic/i-want-to-prey-on-you"},
		{"https://comicland.org/comic/i-want-to-prey-on-you", "https://comicland.org/comic/i-want-to-prey-on-you"},
		{"https://comicland.org/genres", "https://comicland.org/genres"},
	}
	for _, c := range cases {
		g := &Comicland{Grabber: &Grabber{URL: c.input}}
		g.resolveSeriesURL()
		if g.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, g.URL, c.want)
		}
	}
}

func TestComiclandFetch(t *testing.T) {
	var site string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		// the API refuses requests that don't come from the site
		if r.Header.Get("Origin") != site || r.Header.Get("Referer") == "" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":403,"message":"Forbidden: invalid referer or origin"}`))
			return
		}
		switch r.URL.Path {
		case "/api/comic/detail":
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"id":16039,"slug":"i-want-to-prey-on-you","title":"I Want to Prey on You","chapters":[
				{"id":112600,"chapter_index":1,"title":"Chapter 1","page_count":24},
				{"id":112601,"chapter_index":2,"title":"The Tiger","page_count":27}]}}`))
		case "/api/chapter/pages_by_index":
			if r.URL.Query().Get("index") == "1" {
				_, _ = w.Write([]byte(`{"code":0,"data":{"comic_id":16039,"chapter_id":1,"pages":["https://cdn.example/001.jpg","https://cdn.example/002.jpg"]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"comic_id":16039,"chapter_id":0,"pages":[]}}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	site = srv.URL

	g := NewComicland(&Grabber{URL: srv.URL + "/comic/i-want-to-prey-on-you/chapter/2", Settings: &Settings{}})
	g.api = srv.URL + "/api"
	if title, err := g.FetchTitle(); err != nil || title != "I Want to Prey on You" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := g.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v", len(chapters), errs)
	}
	// a bare "Chapter N" is the label's job, a real title is kept
	if chapters[0].GetTitle() != "" || chapters[1].GetTitle() != "The Tiger" {
		t.Errorf("titles = %q, %q", chapters[0].GetTitle(), chapters[1].GetTitle())
	}

	chapter, err := g.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://cdn.example/002.jpg" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
	if _, err := g.FetchChapter(chapters[1]); err == nil {
		t.Error("expected an error for a chapter the API has no pages for")
	}
}
