// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// zinMangaSeriesHTML is a trimmed series page: the list is empty until the
// script loads it from window.getChaptersUrl ({{.}} is the test server)
const zinMangaSeriesHTML = `<html><body>
<div class="post-title"><h1>Misi</h1></div>
<div class="chapter-list main version-chap no-volumn">
<div class="chapters-loading">Loading chapters...</div>
</div>
<script>
        window.comicSlug = 'misi';
        window.chapterCurrentId = null;
        window.getChaptersUrl = '%s/api/comics/misi/chapters';
</script></body></html>`

// zinMangaReaderHTML is a trimmed reader page: the same script, no title
const zinMangaReaderHTML = `<html><body>
<div class="reading-content" id="reading-content-image">
<div class='page-break no-gaps' ><img id='manga-image-0' class='wp-manga-chapter-img'
 src='https://cdn3.zinmanga1.com/misi/38/0.webp'
 data-src='https://cdn3.zinmanga1.com/misi/38/0.webp'
 data-backup='https://cdn2.zinmanga1.com/misi/38/0.webp'></div>
<div class='page-break no-gaps' ><img id='manga-image-1' class='wp-manga-chapter-img'
 data-src='https://cdn3.zinmanga1.com/misi/38/1.webp'></div>
</div>
<script>
    window.getChaptersUrl = '%s/api/comics/misi/chapters';
    window.comicSlug = 'misi';
</script></body></html>`

func newZinMangaServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/misi", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, zinMangaSeriesHTML, srv.URL)
	})
	mux.HandleFunc("/manga/misi/chapter-38", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, zinMangaReaderHTML, srv.URL)
	})
	mux.HandleFunc("/api/comics/misi/chapters", func(w http.ResponseWriter, r *http.Request) {
		// two pages: 38 and 37.5 on the first, 1 on the last
		switch r.URL.Query().Get("page") {
		case "1":
			fmt.Fprint(w, `{"success":true,"data":{"chapters":[
{"comic_id":1,"chapter_id":40,"chapter_num":38,"chapter_name":"Chapter 38","chapter_slug":"chapter-38"},
{"comic_id":1,"chapter_id":39,"chapter_num":37.5,"chapter_name":"Chapter 37.5 - Special","chapter_slug":"chapter-37-5"}],
"total":3,"current_page":1,"per_page":100,"last_page":2}}`)
		case "2":
			fmt.Fprint(w, `{"success":true,"data":{"chapters":[
{"comic_id":1,"chapter_id":1,"chapter_num":1,"chapter_name":"Chapter 1","chapter_slug":"chapter-1"}],
"total":3,"current_page":2,"per_page":100,"last_page":2}}`)
		default:
			http.NotFound(w, r)
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestZinManga(t *testing.T) {
	srv := newZinMangaServer(t)

	z := NewZinManga(&Grabber{URL: srv.URL + "/manga/misi", Settings: &Settings{}})
	if ok, err := z.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if title, err := z.FetchTitle(); err != nil || title != "Misi" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := z.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3 (both API pages)", len(chapters))
	}
	first := chapters[0].(*ZinMangaChapter)
	if first.Number != 38 || first.Title != "" || first.URL != srv.URL+"/manga/misi/chapter-38" {
		t.Errorf("first chapter = %v %q %q", first.Number, first.Title, first.URL)
	}
	if second := chapters[1].(*ZinMangaChapter); second.Number != 37.5 || second.Title != "Chapter 37.5 - Special" {
		t.Errorf("second chapter = %v %q, want its own title kept", second.Number, second.Title)
	}

	chapter, err := z.FetchChapter(first)
	if err != nil {
		t.Fatalf("FetchChapter() error: %s", err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://cdn3.zinmanga1.com/misi/38/1.webp" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}

// A reader page names the API too; the title comes from the series page.
func TestZinMangaReaderURL(t *testing.T) {
	srv := newZinMangaServer(t)

	z := NewZinManga(&Grabber{URL: srv.URL + "/manga/misi/chapter-38", Settings: &Settings{}})
	if ok, err := z.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the reader to match", ok, err)
	}
	if title, err := z.FetchTitle(); err != nil || title != "Misi" {
		t.Errorf("FetchTitle() = %q, %v; want the series title", title, err)
	}
}

// Madara's own pages don't name the API.
func TestZinMangaDeclinesMadara(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.ReplaceAll(zinMangaSeriesHTML, "window.getChaptersUrl", "var other")))
	}))
	t.Cleanup(srv.Close)

	z := NewZinManga(&Grabber{URL: srv.URL + "/manga/misi", Settings: &Settings{}})
	if ok, _ := z.Test(); ok {
		t.Error("Test() matched a page without window.getChaptersUrl")
	}
}
