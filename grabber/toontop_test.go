// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToontopResolveSeriesURL(t *testing.T) {
	cases := []struct{ input, want string }{
		{"https://toontop.io/solo-leveling/chapter-200-5", "https://toontop.io/solo-leveling"},
		{"https://toontop.io/solo-leveling/chapter-1/?p=2", "https://toontop.io/solo-leveling"},
		{"https://toontop.io/solo-leveling", "https://toontop.io/solo-leveling"},
		{"https://toontop.io/", "https://toontop.io/"},
	}
	for _, c := range cases {
		g := &Toontop{Grabber: &Grabber{URL: c.input}}
		g.resolveSeriesURL()
		if g.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, g.URL, c.want)
		}
	}
}

func TestToontopChapters(t *testing.T) {
	rows := []toontopChapterRow{
		{ID: "a", URL: "/solo-leveling/side-story-21", Name: "Side Story 21 - The End", Number: 227},
		{ID: "b", URL: "/solo-leveling/chapter-200-5", Name: "Chapter 200.5", Number: 205},
		{ID: "c", URL: "/solo-leveling/chapter-0-prologue", Name: "Chapter 0 - Prologue", Number: 1},
	}
	chapters := toontopChapters(rows, "https://toontop.io")
	want := []struct {
		number float64
		title  string
		url    string
	}{
		{227, "Side Story 21 - The End", "https://toontop.io/solo-leveling/side-story-21"},
		{200.5, "", "https://toontop.io/solo-leveling/chapter-200-5"},
		{0, "Prologue", "https://toontop.io/solo-leveling/chapter-0-prologue"},
	}
	for i, w := range want {
		c := chapters[i].(*ToontopChapter)
		if c.Number != w.number || c.Title != w.title || c.URL != w.url {
			t.Errorf("chapter[%d] = %v %q %q, want %v %q %q", i, c.Number, c.Title, c.URL, w.number, w.title, w.url)
		}
	}
}

func TestToontopFetch(t *testing.T) {
	var srv *httptest.Server
	nextData := func(props string) string {
		return `<html><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":` + props + `}}</script></body></html>`
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ikumonogakari", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(nextData(`{"siteConfig":{"apiUrl":"` + srv.URL + `/api"},"initialManga":{"id":"02pzzgYR","name":"Ikumonogakari","chapters":[]}}`)))
	})
	mux.HandleFunc("/api/titles/02pzzgYR/chapters", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"chapters":[{"id":"LDgEGV48","url":"/ikumonogakari/chapter-3","name":"Chapter 3","slug":"chapter-3","number":3}]}}`))
	})
	mux.HandleFunc("/ikumonogakari/chapter-3", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(nextData(`{"initialChapter":{"id":"LDgEGV48","pages":[{"url":"https://rx.example/1.webp"},{"url":"https://rx.example/2.webp"}]}}`)))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	g := NewToontop(&Grabber{URL: srv.URL + "/ikumonogakari/chapter-3", Settings: &Settings{}})
	if title, err := g.FetchTitle(); err != nil || title != "Ikumonogakari" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := g.FetchChapters()
	if len(errs) != 0 || len(chapters) != 1 {
		t.Fatalf("FetchChapters() = %d chapters, %v", len(chapters), errs)
	}
	chapter, err := g.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || !strings.HasSuffix(chapter.Pages[1].URL, "/2.webp") || chapter.Number != 3 {
		t.Errorf("chapter = %+v", chapter)
	}
}
