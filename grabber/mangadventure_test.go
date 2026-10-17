// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestMangAdventure serves a MangAdventure instance: the reader page with
// its generator meta and trimmed copies of assortedscans.com's API answers
func newTestMangAdventure(t *testing.T) *httptest.Server {
	t.Helper()
	files := map[string]string{
		"/reader/boku-girl/":       `<html><head><meta name="generator" content="MangAdventure v0.9.6"></head></html>`,
		"/reader/other/":           `<html><head><meta name="generator" content="WordPress 6.8"></head></html>`,
		"/api/v2/series/boku-girl": `{"slug":"boku-girl","title":"Boku Girl","url":"/reader/boku-girl/"}`,
		"/api/v2/series/boku-girl/chapters": `{"results":[` +
			`{"id":384,"title":"My Date","number":90.0,"volume":10,"groups":["Helvetica Scans"],"url":"/reader/boku-girl/10/90/"},` +
			`{"id":504,"title":"One-shot","number":0.0,"volume":null,"groups":["Zaibatsu","Arc-Relight"],"url":"/reader/boku-girl/0/0/"}]}`,
		"/api/v2/chapters/384/pages": `{"results":[` +
			`{"id":1,"image":"https://cdn.example.com/media/series/boku-girl/10/90/a.png","number":1},` +
			`{"id":2,"image":"https://cdn.example.com/media/series/boku-girl/10/90/b.png","number":2}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestMangAdventureTest(t *testing.T) {
	srv := newTestMangAdventure(t)
	cases := []struct {
		path string
		want bool
	}{
		{"/reader/boku-girl/", true},
		{"/reader/other/", false},
		{"/reader/missing/", false},
		{"/series/boku-girl/", false},
	}
	for _, c := range cases {
		m := NewMangAdventure(&Grabber{URL: srv.URL + c.path, Settings: &Settings{}})
		got, err := m.Test()
		if err != nil || got != c.want {
			t.Errorf("Test(%q) = %v, %v, want %v", c.path, got, err, c.want)
		}
	}
}

func TestMangAdventureSlug(t *testing.T) {
	cases := map[string]string{
		"https://assortedscans.com/reader/boku-girl/":         "boku-girl",
		"https://assortedscans.com/reader/boku-girl/10/90/":   "boku-girl",
		"https://arc-relight.com/reader/twitter-4-koma?x=1#y": "twitter-4-koma",
		"https://assortedscans.com/directory/":                "",
	}
	for u, want := range cases {
		if got := NewMangAdventure(&Grabber{URL: u}).slug(); got != want {
			t.Errorf("slug(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestMangAdventureFetch(t *testing.T) {
	srv := newTestMangAdventure(t)
	// a chapter URL lists the whole series
	m := NewMangAdventure(&Grabber{URL: srv.URL + "/reader/boku-girl/10/90/", Settings: &Settings{}})

	title, err := m.FetchTitle()
	if err != nil || title != "Boku Girl" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v", len(chapters), errs)
	}
	first := chapters[0].(*MangAdventureChapter)
	if first.Number != 90 || first.Volume != 10 || first.Title != "My Date" || first.ID != "384" || first.Group != "Helvetica Scans" {
		t.Errorf("first chapter = %+v", first.Chapter)
	}
	oneshot := chapters[1].(*MangAdventureChapter)
	if oneshot.Number != 0 || oneshot.Volume != 0 || len(oneshot.GetGroups()) != 2 {
		t.Errorf("one-shot = %+v", oneshot.Chapter)
	}

	chapter, err := m.FetchChapter(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://cdn.example.com/media/series/boku-girl/10/90/b.png" || chapter.Pages[1].Number != 2 {
		t.Errorf("pages = %+v", chapter.Pages)
	}
	if chapter.ID != "384" || chapter.Group != "Helvetica Scans" || chapter.Volume != 10 {
		t.Errorf("chapter = %+v", chapter)
	}
}
