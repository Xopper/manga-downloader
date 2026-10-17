// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// lilianaSeriesHTML is a trimmed series page (manhuaplus.org)
const lilianaSeriesHTML = `<html><head>
<link rel="stylesheet" href="/themes/liliana/css/style.css"></head><body>
<h1 class="mt-0 mb-6 fs-20">
            I Regressed, but the World didn’t End
        </h1>
<ul id="myUL">
<li data="I Regressed - Chapter 22" class="chapter">
<a href="https://manhuaplus.org/manga/i-regressed/chapter-22">
                Chapter 22
</a><time class="timeago" datetime="1749861609">1y ago</time></li>
<li data="I Regressed - Chapter 21.5" class="chapter">
<a href="/manga/i-regressed/chapter-21-5">Chapter 21.5 Side story</a></li>
</ul></body></html>`

// lilianaReaderHTML is a trimmed reader page: the pages aren't in it
const lilianaReaderHTML = `<html><body><script>
        document.body.classList.add('pv');
        const MANGA_ID = 498;
        const CHAPTER_ID = 77541;
</script>
<div class="check-box" id="chapterContent"><div class="separator" style="display: none">
<a href="#" class="readImg"><img alt="Lazy" src="/themes/liliana/images/Loading/loading.gif"></a></div></div>
</body></html>`

// lilianaImagesJSON is a trimmed image list: the blocks come shuffled
const lilianaImagesJSON = `{"status":true,"html":"<div class=\"separator\" data-index=\"2\"> <a href=\"https:\/\/cdn.manhuaplus.cc\/c.webp\" class=\"readImg\"> <img src=\"https:\/\/cdn.manhuaplus.cc\/c.webp\"> <\/a> <\/div><div class=\"separator\" data-index=\"0\"> <a href=\"https:\/\/cdn.manhuaplus.cc\/a.webp\" class=\"readImg\"> <img src=\"https:\/\/cdn.manhuaplus.cc\/a.webp\"> <\/a> <\/div><div class=\"separator\" data-index=\"1\"> <a href=\"#\" class=\"readImg\"> <img src=\"https:\/\/cdn.manhuaplus.cc\/b.webp\"> <\/a> <\/div>"}`

func newLilianaServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/i-regressed", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, lilianaSeriesHTML)
	})
	mux.HandleFunc("/manga/i-regressed/chapter-22", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, lilianaReaderHTML)
	})
	mux.HandleFunc("/ajax/image/list/chap/77541", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, lilianaImagesJSON)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLiliana(t *testing.T) {
	srv := newLilianaServer(t)

	// a reader URL is reduced to its series
	l := NewLiliana(&Grabber{URL: srv.URL + "/manga/i-regressed/chapter-22", Settings: &Settings{}})
	if ok, err := l.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if title, _ := l.FetchTitle(); title != "I Regressed, but the World didn’t End" {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := l.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v; want 2", len(chapters), errs)
	}
	first, second := chapters[0].(*LilianaChapter), chapters[1].(*LilianaChapter)
	if first.Number != 22 || first.Title != "" {
		t.Errorf("first chapter = %v %q", first.Number, first.Title)
	}
	if second.Number != 21.5 || second.Title != "Chapter 21.5 Side story" || second.URL != srv.URL+"/manga/i-regressed/chapter-21-5" {
		t.Errorf("second chapter = %v %q %q", second.Number, second.Title, second.URL)
	}

	// the fixture's absolute link points at the real site; read the reader
	// from the test server
	first.URL = srv.URL + "/manga/i-regressed/chapter-22"
	chapter, err := l.FetchChapter(first)
	if err != nil {
		t.Fatalf("FetchChapter() error: %s", err)
	}
	var got []string
	for _, p := range chapter.Pages {
		got = append(got, p.URL)
	}
	want := []string{"https://cdn.manhuaplus.cc/a.webp", "https://cdn.manhuaplus.cc/b.webp", "https://cdn.manhuaplus.cc/c.webp"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pages = %v, want %v (data-index order)", got, want)
	}
}

// Other themes' /manga/ pages aren't Liliana's.
func TestLilianaDeclines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><ul id="myUL"><li class="chapter"><a href="/manga/x/chapter-1">Chapter 1</a></li></ul></body></html>`)
	}))
	t.Cleanup(srv.Close)

	l := NewLiliana(&Grabber{URL: srv.URL + "/manga/x", Settings: &Settings{}})
	if ok, _ := l.Test(); ok {
		t.Error("Test() matched a page without the Liliana theme")
	}
}
