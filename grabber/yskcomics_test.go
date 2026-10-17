// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"testing"
)

func TestYskComicsTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.ysk-comics.com/en/comic/Ghost-Rider-2016", true},
		{"https://ysk-comics.com/ar/comic/aasor-mthlm-2021", true},
		{"https://www.ysk-comics.com/en/chapter/ghost-rider-2016-2", true},
		{"https://www.ysk-comics.com/en", false},
		{"https://example.com/en/comic/Ghost-Rider-2016", false},
	}
	for _, tt := range tests {
		got, _ := NewYskComics(&Grabber{URL: tt.url}).Test()
		if got != tt.want {
			t.Errorf("Test(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func yskcomicsServer(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/comic/chapter/Ghost-Rider-2016", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-localization") != "en" {
			t.Errorf("x-localization = %q, want en", r.Header.Get("x-localization"))
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"status":true,"data":{"data_messages":[{"id":6618,"name":"Ghost Rider (2016) 3#","slug":"ghost-rider-2016-3","rank":"3"}],"meta":{"link_next":null,"total_pages":2}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":true,"data":{"data_messages":[{"id":6616,"name":"Ghost Rider (2016) 1#","slug":"ghost-rider-2016-1","rank":"1"},{"id":6617,"name":"Ghost Rider (2016) 2#","slug":"ghost-rider-2016-2","rank":"2"}],"meta":{"link_next":"https://api.ysk-comics.com/x?page=2","total_pages":2}}}`))
	})
	respond(mux, "/api/comic/Ghost-Rider-2016", `{"status":true,"data":{"id":264,"full_name":"Ghost Rider (2016)","slug":"Ghost-Rider-2016","language_code":"en"}}`)
	respond(mux, "/api/chapters/images/ghost-rider-2016-2", `{"status":true,"data":["https://cdn.ysk-comics.com/comics/264/en/2/6908/images/1.jpg","https://cdn.ysk-comics.com/comics/264/en/2/6908/images/2.jpg"]}`)
	respond(mux, "/en/chapter/ghost-rider-2016-2", `<html><body><a href="/en/comic/Ghost-Rider-2016">Ghost Rider (2016)</a></body></html>`)

	return serve(t, mux).URL
}

func TestYskComicsFetch(t *testing.T) {
	base := yskcomicsServer(t)
	y := NewYskComics(&Grabber{URL: base + "/en/comic/Ghost-Rider-2016", Settings: &Settings{}})

	title, err := y.FetchTitle()
	if err != nil || title != "Ghost Rider (2016)" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := y.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3 (both list pages)", len(chapters))
	}
	second := chapters[1].(*YskComicsChapter)
	if second.Number != 2 || second.ID != "6617" || second.Slug != "ghost-rider-2016-2" || second.Language != "en" {
		t.Errorf("second chapter = %+v", second)
	}
	if got := ChapterLabel(second); got != "Chapter 2" {
		t.Errorf("label = %q", got)
	}

	chapter, err := y.FetchChapter(second)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://cdn.ysk-comics.com/comics/264/en/2/6908/images/2.jpg" || chapter.ID != "6617" {
		t.Errorf("chapter = %+v", chapter)
	}
}

func TestYskComicsResolveSeriesURL(t *testing.T) {
	base := yskcomicsServer(t)
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"series URL is kept", base + "/en/comic/Ghost-Rider-2016", base + "/en/comic/Ghost-Rider-2016"},
		{"chapter URL goes to its series", base + "/en/chapter/ghost-rider-2016-2", base + "/en/comic/Ghost-Rider-2016"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y := NewYskComics(&Grabber{URL: tt.url, Settings: &Settings{}})
			if err := y.resolveSeriesURL(); err != nil {
				t.Fatal(err)
			}
			if y.URL != tt.want {
				t.Errorf("URL = %q, want %q", y.URL, tt.want)
			}
		})
	}
}
