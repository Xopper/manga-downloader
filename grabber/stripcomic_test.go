// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestStripComic serves trimmed copies of a webcomic's pages, read as the
// stripSite of the given domain
func newTestStripComic(t *testing.T, domain string, pages map[string]string) *StripComic {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		body, ok := pages[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	s := NewStripComic(&Grabber{URL: "https://www." + domain + "/", Settings: &Settings{}})
	if ok, _ := s.Test(); !ok {
		t.Fatalf("%s not recognised", domain)
	}
	s.URL = srv.URL + "/"
	return s
}

func TestStripComicSuperMega(t *testing.T) {
	s := newTestStripComic(t, "supermegacomics.com", map[string]string{
		"/":              `<a href="index.php?i=2"><img src="bigbuttonprevious.png" name="bigbuttonprevious"></a><img src="http://www.supermegacomics.com/images/3.png" border=4>`,
		"/index.php?i=2": `<img src="http://www.supermegacomics.com/images/2.png" border=4>`,
	})
	chapters, errs := s.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	chapter, err := s.FetchChapter(chapters[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 1 || chapter.Pages[0].URL != "http://www.supermegacomics.com/images/2.png" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}

func TestStripComicPatchFriday(t *testing.T) {
	s := newTestStripComic(t, "patchfriday.com", map[string]string{
		"/":    `<div id="strip_image"><img src="/patches/163.png" width="700"/></div><div id="strip_image_dark"><img src="/patches/dark/163.png"/></div>`,
		"/50/": `<div id="strip_image"><img src="/patches/50.png" width="700"/></div>`,
	})
	chapters, errs := s.FetchChapters()
	if len(errs) > 0 || len(chapters) != 163 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	chapter, err := s.FetchChapter(chapters[49])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 1 || chapter.Pages[0].URL != s.BaseUrl()+"/patches/50.png" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}

func TestStripComicLoadingArtist(t *testing.T) {
	s := newTestStripComic(t, "loadingartist.com", map[string]string{
		"/search.json": `[{"url":"/comic/be-write-back/","title":"Be Write Back","date":"2026-03-24","section":"comic"},` +
			`{"url":"/news/x/","title":"News","date":"2020-01-01","section":"news"},` +
			`{"url":"/art/old/","title":"Old","date":"2011-01-01","section":"art"}]`,
		"/comic/be-write-back/": `<div class=main-image-container><img src=/comic/be-write-back/be-write-back_hu1.jpg alt=x></div>`,
	})
	chapters, errs := s.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 || chapters[0].GetTitle() != "Old" || chapters[1].GetNumber() != 2 {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	chapter, err := s.FetchChapter(chapters[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 1 || chapter.Pages[0].URL != s.BaseUrl()+"/comic/be-write-back/be-write-back_hu1.jpg" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
