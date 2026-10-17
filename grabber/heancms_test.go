// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// heanSeriesHTML is a trimmed series page: the series id sits in the RSC
// payload, the list itself comes from the API
const heanSeriesHTML = `<html><head><title>Is This The Way You Do It? - Omega Scans</title></head><body>
<script>self.__next_f.push([1,"{\"id\":2585,\"chapter_name\":\"Chapter 25\",\"series_id\":46,\"index\":\"25\"}"])</script>
</body></html>`

func newHeanServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/series/is-this-the-way-you-do-it", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, heanSeriesHTML)
	})
	mux.HandleFunc("/api/series/is-this-the-way-you-do-it", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":46,"title":"Is This The Way You Do It?","series_slug":"is-this-the-way-you-do-it"}`)
	})
	mux.HandleFunc("/api/chapter/query", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("series_id") != "46" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"meta":{"total":2,"per_page":200,"current_page":1,"last_page":1},"data":[
{"id":2585,"chapter_name":"Chapter 25","chapter_slug":"chapter-25","price":0},
{"id":2584,"chapter_name":"Chapter 24","chapter_slug":"chapter-24","price":30}]}`)
	})
	mux.HandleFunc("/api/chapter/is-this-the-way-you-do-it/chapter-25", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"chapter":{"id":2585,"chapter_name":"Chapter 25","chapter_data":{"images":[
"https://media.omegascans.org/file/x/01.jpg","https://media.omegascans.org/file/x/02.jpg"]}},"media_token":null}`)
	})
	mux.HandleFunc("/api/chapter/is-this-the-way-you-do-it/chapter-24", func(w http.ResponseWriter, r *http.Request) {
		// a paid chapter answers without its data
		fmt.Fprint(w, `{"chapter":{"id":2584,"chapter_name":"Chapter 24","chapter_data":null}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestHeanCMS(t *testing.T) {
	srv := newHeanServer(t)

	h := NewHeanCMS(&Grabber{URL: srv.URL + "/series/is-this-the-way-you-do-it#46", Settings: &Settings{}})
	h.apiFor = func(string) string { return srv.URL + "/api" }
	if ok, err := h.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if title, _ := h.FetchTitle(); title != "Is This The Way You Do It?" {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := h.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v; want 2", len(chapters), errs)
	}
	if LockedOf(chapters[0]) || !LockedOf(chapters[1]) {
		t.Errorf("locked flags = %v, %v; want only the paid chapter locked", LockedOf(chapters[0]), LockedOf(chapters[1]))
	}

	chapter, err := h.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error: %s", err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://media.omegascans.org/file/x/02.jpg" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
	if _, err := h.FetchChapter(chapters[1]); err == nil {
		t.Error("FetchChapter() on a paid chapter: want an error, got pages")
	}
}

func TestHeanCMSDeclines(t *testing.T) {
	srv := newHeanServer(t)

	// no /series/ path, and a series page without the RSC series id
	for _, path := range []string{"/manga/is-this-the-way-you-do-it", "/series/other"} {
		h := NewHeanCMS(&Grabber{URL: srv.URL + path, Settings: &Settings{}})
		h.apiFor = func(string) string { return srv.URL + "/api" }
		if ok, _ := h.Test(); ok {
			t.Errorf("Test() matched %s", path)
		}
	}
}

func TestHeanAPIBase(t *testing.T) {
	for in, want := range map[string]string{
		"https://omegascans.org/series/x":     "https://api.omegascans.org",
		"https://www.omegascans.org/series/x": "https://api.omegascans.org",
	} {
		if got := heanAPIBase(in); got != want {
			t.Errorf("heanAPIBase(%q) = %q, want %q", in, got, want)
		}
	}
}
