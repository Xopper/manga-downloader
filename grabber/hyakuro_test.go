// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHyakuroResolveSeriesURL(t *testing.T) {
	cases := []struct{ input, want string }{
		{"https://hyakuro.net/manga/balaclover/read/1/3", "https://hyakuro.net/manga/balaclover"},
		{"https://hyakuro.net/manga/balaclover/download/1", "https://hyakuro.net/manga/balaclover"},
		{"https://hyakuro.net/manga/balaclover", "https://hyakuro.net/manga/balaclover"},
		{"https://hyakuro.net/manga/balaclover/", "https://hyakuro.net/manga/balaclover/"},
		{"https://hyakuro.net/search/-", "https://hyakuro.net/search/-"},
	}
	for _, c := range cases {
		g := &Hyakuro{Grabber: &Grabber{URL: c.input}}
		g.resolveSeriesURL()
		if g.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, g.URL, c.want)
		}
	}
}

// hyakuroMangasJSON is a trimmed Strapi /mangas answer, its pages out of
// order
const hyakuroMangasJSON = `{"data":[{"id":1,"attributes":{"Title":"BALACLOVER","slug":"balaclover","Chapters":[
	{"id":698,"Title":null,"Chapter":1,"Volume":2,"Pages":{"data":[
		{"id":2,"attributes":{"name":"Balaclover Page 010.png","url":"/uploads/Balaclover_Page_010_ff.png"}},
		{"id":1,"attributes":{"name":"Balaclover Page 002.png","url":"/uploads/Balaclover_Page_002_aa.png"}}]}}]}}],"meta":{}}`

func TestHyakuroFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend/api/mangas" || r.URL.Query().Get("filters[slug][$eq]") != "balaclover" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(hyakuroMangasJSON))
	}))
	t.Cleanup(srv.Close)

	g := NewHyakuro(&Grabber{URL: srv.URL + "/manga/balaclover/read/1/1", Settings: &Settings{}})
	if title, err := g.FetchTitle(); err != nil || title != "BALACLOVER" {
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
	want := []string{
		srv.URL + "/backend/uploads/Balaclover_Page_002_aa.png",
		srv.URL + "/backend/uploads/Balaclover_Page_010_ff.png",
	}
	if len(chapter.Pages) != len(want) || chapter.Volume != 2 || chapter.Number != 1 {
		t.Fatalf("chapter = %+v", chapter)
	}
	for i, p := range chapter.Pages {
		if p.URL != want[i] {
			t.Errorf("page %d = %q, want %q", i+1, p.URL, want[i])
		}
	}
}
