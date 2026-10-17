// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLeslieVictimsFetch(t *testing.T) {
	// a trimmed manga.json: chapter 01 has a counted root, 2 a listed one,
	// "3-END" none (two pictures, then the HTML page)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manga.json":
			fmt.Fprintf(w, `[{"id":"Other","title":"Other","chapters":[]},{"id":"Becoming the Queen","title":"Becoming the Queen","chapters":["01","2","3-END"],"chapter_roots":{`+
				`"01":{"url":"https://r2.example.dev/Becoming the Queen/01","mode":"count","data":2},`+
				`"2":{"url":"https://r2.example.dev/Becoming the Queen/2","mode":"list","data":["1.webp","10.webp"]}}}]`)
		case "/content/Becoming the Queen/3-END/01.webp", "/content/Becoming the Queen/3-END/02.webp":
			w.Write([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "))
		default:
			fmt.Fprint(w, "<!doctype html><html></html>")
		}
	}))
	defer srv.Close()

	l := NewLeslieVictims(&Grabber{URL: srv.URL + "/?series=Becoming%20the%20Queen", Settings: &Settings{}})
	title, err := l.FetchTitle()
	if err != nil || title != "Becoming the Queen" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := l.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 || chapters[2].GetNumber() != 3 || chapters[2].GetTitle() != "END" {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	want := [][]string{
		{"https://r2.example.dev/Becoming%20the%20Queen/01/01.webp", "https://r2.example.dev/Becoming%20the%20Queen/01/02.webp"},
		{"https://r2.example.dev/Becoming%20the%20Queen/2/1.webp", "https://r2.example.dev/Becoming%20the%20Queen/2/10.webp"},
		{srv.URL + "/content/Becoming%20the%20Queen/3-END/01.webp", srv.URL + "/content/Becoming%20the%20Queen/3-END/02.webp"},
	}
	for i, w := range want {
		chapter, err := l.FetchChapter(chapters[i])
		if err != nil {
			t.Fatal(err)
		}
		if len(chapter.Pages) != len(w) {
			t.Fatalf("chapter %d pages = %+v", i, chapter.Pages)
		}
		for j, p := range chapter.Pages {
			if p.URL != w[j] {
				t.Errorf("chapter %d page %d = %s, want %s", i, j, p.URL, w[j])
			}
		}
	}
}
