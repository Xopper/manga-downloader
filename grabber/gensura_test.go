// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGensuraTest(t *testing.T) {
	cases := map[string]bool{
		"https://gensura.net/manga/haha-wa-onna-deshita-5/":   true,
		"https://gensura.net/manga/haha-wa-onna-deshita-5/3/": true,
		"https://gensura.net/tags/x/":                         false,
		"https://example.com/manga/x/":                        false,
	}
	for u, want := range cases {
		if got, _ := NewGensura(&Grabber{URL: u}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestGensuraFetch(t *testing.T) {
	// a trimmed copy of a gallery page; thumbnails after the 16th are lazy-loaded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manga/g/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `<h1 class="text-2xl font-semibold">Haha wa Onna Deshita 5 | 母は女でした 5</h1>`+
			`<div class="img-thumb"><a href="/manga/g/1/"><div><img class="w-full" src="/media/uploads/images/1/2/aaa-t.webp"> 1</div></a></div>`+
			`<div class="hidden lazy-img-thumb"><a href="/manga/g/2/"><div><img class="w-full" data-src="/media/uploads/images/1/2/bbb-t.jpg"> 2</div></a></div>`)
	}))
	defer srv.Close()

	g := NewGensura(&Grabber{URL: srv.URL + "/manga/g/2/", Settings: &Settings{}})
	chapters, errs := g.FetchChapters()
	if len(errs) > 0 || len(chapters) != 1 || chapters[0].GetTitle() != "Haha wa Onna Deshita 5 | 母は女でした 5" {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	chapter, err := g.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{srv.URL + "/media/uploads/images/1/2/aaa.webp", srv.URL + "/media/uploads/images/1/2/bbb.jpg"}
	if len(chapter.Pages) != 2 || chapter.Pages[0].URL != want[0] || chapter.Pages[1].URL != want[1] {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
