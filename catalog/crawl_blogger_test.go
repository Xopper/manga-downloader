// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"testing"
)

func TestBloggerSeriesFromPostTitles(t *testing.T) {
	f := withFakeBodies(t, map[string]string{
		"https://www.b.test/feeds/posts/summary?alt=json&max-results=150&start-index=1": read(t, "blogger/feed-1.json"),
	})
	site := SiteInfo{Site: "b.test", Home: "https://www.b.test", List: "https://www.b.test/feeds/posts/summary"}
	got, err := crawlers["blogger"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Title: "Madan no Ichi", URL: "https://www.b.test/search?q=Madan+no+Ichi"},
		{Title: "One Piece", URL: "https://www.b.test/search?q=One+Piece"},
		{Title: "Kenichi 2", URL: "https://www.b.test/search?q=Kenichi+2"},
		{Title: "Fairy Tail 100 Years Quest", URL: "https://www.b.test/search?q=Fairy+Tail+100+Years+Quest"},
		{Title: "Blue Lock", URL: "https://www.b.test/search?q=Blue+Lock"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Title != want[i].Title || got[i].URL != want[i].URL {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// a page shorter than 150 posts is the last
	if len(f.calls) != 1 {
		t.Errorf("calls %+v", f.calls)
	}
}

func TestBloggerLabelFeed(t *testing.T) {
	f := withFakeBodies(t, map[string]string{
		"https://b.test/feeds/posts/summary/-/Chapter?alt=json&max-results=150&start-index=1": read(t, "blogger/feed-1.json"),
	})
	site := SiteInfo{Site: "b.test", Home: "https://b.test", List: "https://b.test/feeds/posts/summary/-/Chapter"}
	if _, err := crawlers["blogger"].Crawl(context.Background(), site, 0); err != nil || len(f.calls) != 1 {
		t.Fatalf("err %v, calls %+v", err, f.calls)
	}
}

func TestBloggerSeries(t *testing.T) {
	cases := map[string]string{
		"Blue Lock Manga 364 Parte 2 [Español]": "Blue Lock",
		"Kenichi 2 | Manga 27":                  "Kenichi 2",
		"Shangri-La Frontier Chapter 283":       "Shangri-La Frontier",
		"Dragon Ball Super Manga Capítulo 102":  "Dragon Ball Super",
		"Noticias":                              "",
	}
	for title, want := range cases {
		if got, _ := bloggerSeries(title); got != want {
			t.Errorf("%q: got %q, want %q", title, got, want)
		}
	}
}
