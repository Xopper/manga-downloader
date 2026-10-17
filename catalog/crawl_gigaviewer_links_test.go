// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"testing"
)

// comic-ogyaaa.com has no series list: its home page's series cards carry
// data-series-name, among banners and news that also link episodes
func TestGigaviewerLinksSelector(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://comic-ogyaaa.com/": read(t, "gigaviewer/ogyaaa-home.html")})
	site := SiteInfo{Site: "comic-ogyaaa.com", Home: "https://comic-ogyaaa.com", List: "https://comic-ogyaaa.com/",
		Links: "a[data-series-name]"}
	got, err := crawlers["gigaviewer"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "(元)奴隷エルフを幸せにするいくつかの方法" ||
		got[0].URL != "https://comic-ogyaaa.com/episode/12207421983742958974" || got[1].Title != "壊していいよ、すいかちゃん" {
		t.Fatalf("got %+v", got)
	}
}
