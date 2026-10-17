// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNuviatoonResolveSeriesURL(t *testing.T) {
	cases := []struct{ input, want string }{
		{"https://nuviatoon.net/comics/garden-of-may/chapter/10", "https://nuviatoon.net/comics/garden-of-may"},
		{"https://nuviatoon.net/comics/garden-of-may/chapter/10?x=1#p", "https://nuviatoon.net/comics/garden-of-may"},
		{"https://nuviatoon.net/comics/garden-of-may", "https://nuviatoon.net/comics/garden-of-may"},
		{"https://nuviatoon.net/novels/garden-of-may", "https://nuviatoon.net/novels/garden-of-may"},
	}
	for _, c := range cases {
		g := &Nuviatoon{Grabber: &Grabber{URL: c.input}}
		g.resolveSeriesURL()
		if g.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, g.URL, c.want)
		}
	}
}

func TestNuviatoonFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/comics/garden-of-may", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"s1","kind":"comic","slug":"garden-of-may","title":"Garden of May","chapters":[
			{"id":"c2","number":2,"title":null,"priceCoins":30},
			{"id":"c1","number":1,"title":"Spring","priceCoins":0}]}}`))
	})
	mux.HandleFunc("/api/comic-chapters/c1/content", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"kind":"comic","pages":[{"index":0,"url":"https://r2.example/0001.webp?X-Amz-Signature=a"},{"index":1,"url":"https://r2.example/0002.webp?X-Amz-Signature=b"}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	g := NewNuviatoon(&Grabber{URL: srv.URL + "/comics/garden-of-may/chapter/1", Settings: &Settings{}})
	if title, err := g.FetchTitle(); err != nil || title != "Garden of May" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := g.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v", len(chapters), errs)
	}
	if !LockedOf(chapters[0]) || LockedOf(chapters[1]) {
		t.Errorf("locked = %v, %v; want the coin chapter locked", LockedOf(chapters[0]), LockedOf(chapters[1]))
	}
	chapter, err := g.FetchChapter(chapters[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Title != "Spring" || chapter.Pages[0].URL != "https://r2.example/0001.webp?X-Amz-Signature=a" {
		t.Errorf("chapter = %+v", chapter)
	}
}
