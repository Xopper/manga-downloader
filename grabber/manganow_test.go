// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestManganowTest(t *testing.T) {
	for url, want := range map[string]bool{
		"https://manganow.to/manga/goblin":           true,
		"https://www.manganow.to/manga/goblin":       true,
		"https://example.com/manga/goblin":           false,
		"https://example.com/?u=manganow.to/manga/x": false,
	} {
		if ok, _ := NewManganow(&Grabber{URL: url, Settings: &Settings{}}).Test(); ok != want {
			t.Errorf("Test(%s) = %v, want %v", url, ok, want)
		}
	}
}

// TestManganowChapter lists the English chapters, then reads a chapter's
// reading id and its image list (a 202 answer), leaving out the banner card
func TestManganowChapter(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manga/goblin":
			fmt.Fprintf(w, `<div id="wrapper" data-manga-id="2152"><h2 class="manga-name"> Goblin </h2>
<ul class="ulclear reading-list lang-chapters" id="en-chapters" style="display:none;">
<li class="item reading-item chapter-item" data-number="87"><a href="%[1]s/manga/goblin/chapter-87" class="item-link" title="Chapter 87">Chapter 87</a></li>
<li class="item reading-item chapter-item" data-number="86.5"><a href="%[1]s/manga/goblin/chapter-86.5" class="item-link" title="Chapter 86.5">Chapter 86.5</a></li>
</ul></div><h2 class="manga-name">Other series</h2>`, srv.URL)
		case "/manga/goblin/chapter-87":
			fmt.Fprint(w, `<div id="wrapper" data-reading-id="171493" data-reading-by="chap" data-manga-id="2152"></div>`)
		case "/ajax/image/list/171493":
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				http.Error(w, "no", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprintf(w, `{"status":true,"html":"<div class=\"iv-card \" data-url=\"%s/manganow/images/manganow.jpg\"></div><div class=\"iv-card \" data-url=\"\t\t\nhttps://cdn.example.org/manga/3.jpeg\"></div><div class=\"iv-card\" data-url=\"https://cdn.example.org/manga/4.jpeg\"></div>"}`, srv.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := NewManganow(&Grabber{URL: srv.URL + "/manga/goblin/chapter-87", Settings: &Settings{}})
	if title, err := m.FetchTitle(); err != nil || title != "Goblin" {
		t.Errorf("FetchTitle() = %q, %v; want Goblin", title, err)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 || chapters[1].GetNumber() != 86.5 {
		t.Fatalf("FetchChapters() = %d chapters, %v; want 87 and 86.5", len(chapters), errs)
	}
	chapter, err := m.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	var urls []string
	for _, p := range chapter.Pages {
		urls = append(urls, p.URL)
		if !p.NoReferer {
			t.Errorf("page %s is sent with a Referer, which its host refuses", p.URL)
		}
	}
	if want := []string{"https://cdn.example.org/manga/3.jpeg", "https://cdn.example.org/manga/4.jpeg"}; !slices.Equal(urls, want) {
		t.Errorf("pages = %v, want %v", urls, want)
	}
}
