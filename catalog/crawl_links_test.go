// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"testing"
)

func TestLinksWalksPagesUntilEmpty(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://k.test/advanced.php?page=1": read(t, "links/komikmanga-1.html"),
		"https://k.test/advanced.php?page=2": read(t, "links/komikmanga-2.html"),
		"https://k.test/advanced.php?page=3": read(t, "links/komikmanga-3.html"),
	})
	site := SiteInfo{Site: "k.test", Home: "https://k.test", List: "https://k.test/advanced.php?page={page}",
		Links: "#advanced-content .advanced-element > a"}
	got, err := crawlers["links"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || len(f.calls) != 3 {
		t.Fatalf("got %d entries in %d pages: %+v", len(got), len(f.calls), got)
	}
	// the title attribute, not the cut text ("Star Martial God Tec...")
	if got[1].Title != "Star Martial God Technique" || got[1].URL != "https://k.test/manga/Star-Martial-God-Technique" {
		t.Errorf("second: %+v", got[1])
	}
	for _, e := range got {
		if e.Title == "Not a row" {
			t.Error("a link outside the selector was kept")
		}
	}
}

func TestLinksOnePageListAndSeriesFilter(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://a.test/changeMangaList?type=text": read(t, "links/anzmanga-list.html"),
	})
	site := SiteInfo{Site: "a.test", Home: "https://a.test", List: "https://a.test/changeMangaList?type=text",
		Links: "ul.manga-list a.alpha-link", Series: `/manga/a[a-z]{3,}-`}
	got, err := crawlers["links"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Errorf("a list without {page} is one page: %v", f.calls)
	}
	// the text, as the links have no title; Series drops "ao-no-hako"
	if len(got) != 2 || got[0].Title != "Aharen-san wa Hakarenai" ||
		got[0].URL != "https://www.anzmanga25.com/manga/aharensan-wa-hakarenai" {
		t.Fatalf("got %+v", got)
	}
}

func TestLinksNeedsSelector(t *testing.T) {
	withFakeFetch(t, nil)
	if _, err := crawlers["links"].Crawl(context.Background(), SiteInfo{Site: "a.test", List: "https://a.test/"}, 0); err == nil {
		t.Error("a links site without links must fail")
	}
}
