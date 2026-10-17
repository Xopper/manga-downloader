// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJunmeituTest(t *testing.T) {
	cases := map[string]bool{
		"https://meijuntu.com/beauty/03ji_wodeliangongfang.html":   true,
		"https://meijuntu.com/beauty/03ji_wodeliangongfang-2.html": true,
		"https://junmeitu.com/beauty/x.html":                       true,
		"https://meijuntu.com/beauty/":                             false,
		"https://example.com/beauty/x.html":                        false,
	}
	for u, want := range cases {
		if got, _ := NewJunmeitu(&Grabber{URL: u}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestJunmeituFetch(t *testing.T) {
	// trimmed copies of a gallery's three pages
	page := func(n int) string {
		return fmt.Sprintf(`<h1 class="title">MTZ 03輯</h1><div class="pictures"><img src="https://s.example.com/%d.webp" /></div>`+
			`<div class="pages"><a href="/beauty/g.html">1</a> <a href="/beauty/g-2.html">2</a> <a href="/beauty/g-3.html">3</a> <a href="/beauty/g-2.html">下一頁</a></div>`, n)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/beauty/g.html":
			fmt.Fprint(w, page(1))
		case "/beauty/g-2.html":
			fmt.Fprint(w, page(2))
		case "/beauty/g-3.html":
			fmt.Fprint(w, page(3))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	j := NewJunmeitu(&Grabber{URL: srv.URL + "/beauty/g-2.html", Settings: &Settings{}})
	chapters, errs := j.FetchChapters()
	if len(errs) > 0 || len(chapters) != 1 || chapters[0].GetTitle() != "MTZ 03輯" {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	chapter, err := j.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 3 || chapter.Pages[2].URL != "https://s.example.com/3.webp" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
