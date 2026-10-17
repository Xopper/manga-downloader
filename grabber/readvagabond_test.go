// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadVagabondTest(t *testing.T) {
	cases := map[string]bool{
		"https://readbagabondo.com/#1":                     true,
		"https://readbagabondo.com/volume-37/chapter-322/": true,
		"https://example.com/readbagabondo.com":            false,
	}
	for u, want := range cases {
		if got, _ := NewReadVagabond(&Grabber{URL: u}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestReadVagabondFetch(t *testing.T) {
	// trimmed copies of the site's API answers
	files := map[string]string{
		"/api/mihon/mangas":            `[{"id":1,"title":"Vagabond","cover":"https://pub.example.net/covers/volume-37.jpg"}]`,
		"/api/mihon/mangas/1/chapters": `[{"id":322,"number":322,"title":"Vol. 37 Ch. 322 - Dragonflies","volume":37,"pageCount":3,"mangaId":1}]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	r := NewReadVagabond(&Grabber{URL: srv.URL + "/volume-37/chapter-322/", Settings: &Settings{}})
	title, err := r.FetchTitle()
	if err != nil || title != "Vagabond" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := r.FetchChapters()
	if len(errs) > 0 || len(chapters) != 1 || chapters[0].GetNumber() != 322 {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	chapter, err := r.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 3 || chapter.Pages[2].URL != "https://pub.example.net/chapter-322/page-3.png" || chapter.Volume != 37 {
		t.Errorf("chapter = %+v", chapter)
	}
}
