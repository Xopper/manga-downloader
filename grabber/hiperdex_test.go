// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// hiperdexBundleJS is a trimmed piece of the SPA bundle that builds the
// client header ("x-cfg-auth": atob("c2VjcmV0") + "42")
const hiperdexBundleJS = `const lC=[120,45,99,102,103,45,97,117,116,104].map(n=>String.fromCharCode(n)).join(""),sC=atob("c2VjcmV0")+"42",oC=vi.createClient({})`

func TestHiperdexClientHeader(t *testing.T) {
	name, value, err := hiperdexClientHeader(hiperdexBundleJS)
	if err != nil {
		t.Fatal(err)
	}
	if name != "x-cfg-auth" || value != "secret42" {
		t.Errorf("hiperdexClientHeader() = %q, %q", name, value)
	}
	if _, _, err := hiperdexClientHeader("const a=1"); err == nil {
		t.Error("expected an error for a bundle without the header")
	}
}

func TestHiperdexResolveSeriesURL(t *testing.T) {
	cases := []struct{ input, want string }{
		{"https://hiperdex.tv/manga/im-no-heroine/76", "https://hiperdex.tv/manga/im-no-heroine"},
		{"https://hiperdex.tv/manga/im-no-heroine/76.5/?a=1", "https://hiperdex.tv/manga/im-no-heroine"},
		{"https://hiperdex.tv/manga/im-no-heroine", "https://hiperdex.tv/manga/im-no-heroine"},
		{"https://hiperdex.tv/search", "https://hiperdex.tv/search"},
	}
	for _, c := range cases {
		h := &Hiperdex{Grabber: &Grabber{URL: c.input}}
		h.resolveSeriesURL()
		if h.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, h.URL, c.want)
		}
	}
}

func TestHiperdexFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/im-no-heroine", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><script type="module" crossorigin src="/assets/index-AbC.js"></script></head></html>`))
	})
	mux.HandleFunc("/assets/index-AbC.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(hiperdexBundleJS))
	})
	mux.HandleFunc("/api/trpc/", func(w http.ResponseWriter, r *http.Request) {
		input := struct {
			JSON map[string]any `json:"json"`
		}{}
		_ = json.Unmarshal([]byte(r.URL.Query().Get("input")), &input)
		switch r.URL.Path {
		case "/api/trpc/series.bySlug":
			_, _ = w.Write([]byte(`{"result":{"data":{"json":{"id":9999,"slug":"im-no-heroine","title":"I’m No Heroine!"}}}}`))
		case "/api/trpc/series.chapters":
			_, _ = w.Write([]byte(`{"result":{"data":{"json":[
				{"id":2,"number":76.5,"volume":2,"title":"Extra","language":"en","status":"published","pagesCount":2,"isDeleted":false},
				{"id":1,"number":1,"volume":null,"title":null,"language":"en","status":"published","pagesCount":3,"isDeleted":false},
				{"id":3,"number":2,"volume":null,"title":null,"language":"en","status":"published","pagesCount":3,"isDeleted":true}]}}}`))
		case "/api/trpc/reader.chapterPages":
			if r.Header.Get("x-cfg-auth") != "secret42" {
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"Use the official app","code":-32003}}}`))
				return
			}
			if input.JSON["chapterNumber"] != 76.5 || input.JSON["chapterId"] != float64(2) {
				t.Errorf("chapterPages input = %v", input.JSON)
			}
			_, _ = w.Write([]byte(`{"result":{"data":{"json":[
				{"pageOrder":2,"webpUrl":"https://i1.example/2.webp","avifUrl":null},
				{"pageOrder":1,"webpUrl":"https://i1.example/1.webp","avifUrl":null}]}}}`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	h := NewHiperdex(&Grabber{URL: srv.URL + "/manga/im-no-heroine/76.5", Settings: &Settings{}})
	if title, err := h.FetchTitle(); err != nil || title != "I’m No Heroine!" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := h.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v (the deleted one must be left out)", len(chapters), errs)
	}
	first := chapters[0].(*HiperdexChapter)
	if first.Number != 76.5 || first.Title != "Extra" || first.Volume != 2 || first.ID != "2" || first.Language != "en" {
		t.Errorf("chapter[0] = %+v", first.Chapter)
	}

	chapter, err := h.FetchChapter(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[0].URL != "https://i1.example/1.webp" || chapter.Pages[1].URL != "https://i1.example/2.webp" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
