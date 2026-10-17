// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"reflect"
	"testing"
)

func TestBloggerLabels(t *testing.T) {
	withFakeFetch(t, map[string]string{
		"https://www.b.test/feeds/posts/summary?alt=json&max-results=0": read(t, "blogger/feed.json"),
	})
	site := SiteInfo{Site: "b.test", Home: "https://www.b.test", Family: "blogger", Exclude: "(?i)^(mangas|anime)$| anime$"}
	got, err := crawlers["blogger"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Title: "Blue Lock", URL: "https://www.b.test/search?q=Blue+Lock"},
		{Title: "Shangri-La Frontier: Shitty Games Hunter Challenges Godly Game", URL: "https://www.b.test/search?q=Shangri-La+Frontier%3A+Shitty+Games+Hunter+Challenges+Godly+Game"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestBloggerSeriesURLAndErrors(t *testing.T) {
	withFakeFetch(t, map[string]string{
		"https://b.test/feed": read(t, "blogger/feed.json"),
		"https://b.test/none": `{"feed":{}}`,
	})
	site := SiteInfo{Site: "b.test", Home: "https://b.test", List: "https://b.test/feed", SeriesURL: "https://b.test/search/label/{1}"}
	got, err := crawlers["blogger"].Crawl(context.Background(), site, 0)
	if err != nil || len(got) != 5 || got[1].URL != "https://b.test/search/label/Blue+Lock" {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, list := range []string{"https://b.test/none", "https://b.test/missing"} {
		site.List = list
		if _, err := crawlers["blogger"].Crawl(context.Background(), site, 0); err == nil {
			t.Errorf("%s: no error", list)
		}
	}
}
