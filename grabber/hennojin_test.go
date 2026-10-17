// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// hennojinReaderHTML is a trimmed reader page: the pictures sit in a folder
// named after the work, written unescaped
const hennojinReaderHTML = `<html><body><img src="/home/logo.png">
<div id="readmanga" class="mySlides"><img src="/home/hennojin-manga/(C97) [askot] Lamia [English]/1.png", style="width:100%"></div>
<div id="readmanga" class="mySlides"><img src="/home/hennojin-manga/(C97) [askot] Lamia [English]/2.png", style="width:100%"></div>
</body></html>`

func TestHennojinReaderURL(t *testing.T) {
	u, _ := url.Parse("https://hennojin.com/home/manga-reader/?manga=(C97) [askot] Lamia [English]&view=page")
	got := hennojinReaderURL(u)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("hennojinReaderURL() = %q: %s", got, err)
	}
	if m := parsed.Query().Get("manga"); m != "(C97) [askot] Lamia [English]" || parsed.Query().Get("view") != "page" {
		t.Errorf("hennojinReaderURL() = %q, query manga %q", got, m)
	}
}

func TestHennojinFetchChapter(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("manga"))
		fmt.Fprint(w, hennojinReaderHTML)
	}))
	t.Cleanup(srv.Close)

	h := NewHennojin(&Grabber{URL: srv.URL + "/home/manga-reader/?manga=(C97) [askot] Lamia [English]", Settings: &Settings{}})
	if title, err := h.FetchTitle(); err != nil || title != "(C97) [askot] Lamia [English]" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := h.FetchChapters()
	if len(errs) > 0 || len(chapters) != 1 {
		t.Fatalf("FetchChapters() = %v, %v; want the work as one chapter", chapters, errs)
	}
	chapter, err := h.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error: %s", err)
	}
	want := srv.URL + "/home/hennojin-manga/%28C97%29%20%5Baskot%5D%20Lamia%20%5BEnglish%5D/2.png"
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != want {
		t.Errorf("pages = %+v, want 2, the last %q", chapter.Pages, want)
	}
	if len(asked) != 1 || asked[0] != "(C97) [askot] Lamia [English]" {
		t.Errorf("reader asked for %q", asked)
	}
}

func TestHennojinTest(t *testing.T) {
	for in, want := range map[string]bool{
		"https://hennojin.com/home/manga/some-work/":      true,
		"https://hennojin.com/home/manga-reader/?manga=x": true,
		"https://hennojin.com/home/tags/vanilla/":         false,
		"https://example.com/home/manga/some-work/":       false,
	} {
		if ok, _ := NewHennojin(&Grabber{URL: in, Settings: &Settings{}}).Test(); ok != want {
			t.Errorf("Test(%q) = %v, want %v", in, ok, want)
		}
	}
}
