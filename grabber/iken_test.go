// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ikenSeriesHTML is a trimmed Astro series page (kencomics.com): the newest
// chapters as rows the sanascans PlainHTML entry would match, and the config
// naming the API (%s is the test server)
const ikenSeriesHTML = `<html><body>
<script>(()=>{const API="%[1]s";})()</script>
<script>window.__ENV={"NEXT_PUBLIC_DISQUS_SHORTNAME":"kencomics.com","NEXT_PUBLIC_API_URL":"%[1]s","PUBLIC_API_URL":"%[1]s"}</script>
<h1 class="text-2xl" itemProp="name">Jiyujiyo</h1>
<a href="/series/jiyujiyo/chapter-72" data-astro-prefetch="hover"><span class="font-medium">Chapter 72</span></a>
<a href="/series/jiyujiyo/chapter-71" data-astro-prefetch="hover"><span class="font-medium">Chapter 71</span></a>
</body></html>`

// ikenReaderHTML is a trimmed reader page
const ikenReaderHTML = `<html><body>
<img src="https://storage.kencomics.com/logo.webp" alt="Kenscans logo">
<div class="comic-images-wrapper reader-mode-strip">
<img src="https://storage.kencomics.com/public/upload/series/jiyujiyo/x/page-0001.webp" alt="Jiyujiyo Chapter 1 Page 1" data-reader-page-image="true" data-reader-index="0"/>
<img src="https://storage.kencomics.com/public/upload/series/jiyujiyo/x/page-0002.webp" alt="Jiyujiyo Chapter 1 Page 2" data-reader-page-image="true" data-reader-index="1"/>
</div></body></html>`

func newIkenServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/series/jiyujiyo", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, ikenSeriesHTML, srv.URL)
	})
	mux.HandleFunc("/series/jiyujiyo/chapter-1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, ikenReaderHTML)
	})
	mux.HandleFunc("/api/post", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("postSlug") != "jiyujiyo" {
			fmt.Fprint(w, `{"post":null}`)
			return
		}
		fmt.Fprint(w, `{"totalChapterCount":3,"post":{"id":162,"slug":"jiyujiyo","postTitle":"Jiyujiyo"}}`)
	})
	mux.HandleFunc("/api/chapters", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("postId") != "162" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"post":{"chapters":[
{"id":5054,"slug":"chapter-1","number":1,"title":"","isShortLinkLocked":false,"isLocked":false},
{"id":5055,"slug":"chapter-2","number":2,"title":"The bridge","isShortLinkLocked":true,"isLocked":true},
{"id":10761,"slug":"chapter-72","number":72,"title":"","isShortLinkLocked":false,"isLocked":true}]},
"totalChapterCount":3}`)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestIken(t *testing.T) {
	srv := newIkenServer(t)

	k := NewIken(&Grabber{URL: srv.URL + "/series/jiyujiyo", Settings: &Settings{}})
	if ok, err := k.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if title, _ := k.FetchTitle(); title != "Jiyujiyo" {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := k.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want the API's 3", len(chapters))
	}
	// a short-link lock may still open, a coin lock doesn't
	for i, want := range []bool{false, false, true} {
		if got := LockedOf(chapters[i]); got != want {
			t.Errorf("chapter %v locked = %v, want %v", chapters[i].GetNumber(), got, want)
		}
	}
	if got := chapters[1].GetTitle(); got != "The bridge" {
		t.Errorf("chapter 2 title = %q", got)
	}

	chapter, err := k.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error: %s", err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[0].URL != "https://storage.kencomics.com/public/upload/series/jiyujiyo/x/page-0001.webp" {
		t.Errorf("pages = %+v, want the two reader pages and not the logo", chapter.Pages)
	}
}

// A reader URL is reduced to its series' slug.
func TestIkenReaderURL(t *testing.T) {
	srv := newIkenServer(t)

	// the reader fixture doesn't print the config (nyxscans' readers don't)
	k := NewIken(&Grabber{URL: srv.URL + "/series/jiyujiyo/chapter-1", Settings: &Settings{}})
	if ok, err := k.Test(); err != nil || !ok || k.slug != "jiyujiyo" {
		t.Fatalf("Test() = %v, %v, slug %q", ok, err, k.slug)
	}
}

// The Iken series page also matches a generic PlainHTML entry, which would
// list only its newest rows, so IdentifySite must try Iken first.
func TestIkenIdentifiedBeforePlainHTML(t *testing.T) {
	srv := newIkenServer(t)

	site, errs := NewSite(srv.URL+"/series/jiyujiyo", &Settings{})
	if _, ok := site.(*Iken); !ok {
		t.Fatalf("IdentifySite() = %T (%v), want *Iken", site, errs)
	}
}

// Pages that don't name the API, or a series it doesn't know, aren't Iken's.
func TestIkenDeclines(t *testing.T) {
	srv := newIkenServer(t)

	for _, path := range []string{"/series/unknown", "/manga/jiyujiyo"} {
		k := NewIken(&Grabber{URL: srv.URL + path, Settings: &Settings{}})
		if ok, _ := k.Test(); ok {
			t.Errorf("Test() matched %s", path)
		}
	}
}
