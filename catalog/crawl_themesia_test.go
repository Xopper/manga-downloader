// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"testing"
)

var themesiaSite = SiteInfo{Site: "a.test", Home: "https://a.test", List: "https://a.test/manga/list-mode/"}

func TestThemesiaListMode(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://a.test/manga/list-mode/": read(t, "themesia/list-mode.html"),
	})
	got, err := crawlers["themesia"].Crawl(context.Background(), themesiaSite, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	if got[0].Title != "??? Rank no Ura Soubi Musou" || got[0].URL != "https://kingofshojo.com/manga/rank-no-ura-soubi-musou/" {
		t.Errorf("first: %+v", got[0])
	}
	if len(f.calls) != 1 {
		t.Errorf("list-mode is one page, fetched %v", f.calls)
	}
}

func TestThemesiaGridFallback(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		// list-mode 404s; the grid serves two pages, the third is empty
		"https://a.test/manga/?page=1": read(t, "themesia/grid-1.html"),
		"https://a.test/manga/?page=2": read(t, "themesia/grid-2.html"),
		"https://a.test/manga/?page=3": read(t, "themesia/grid-empty.html"),
	})
	got, err := crawlers["themesia"].Crawl(context.Background(), themesiaSite, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 40 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].Title == "" || got[0].URL != "https://kingofshojo.com/manga/rank-no-ura-soubi-musou/" {
		t.Errorf("first: %+v", got[0])
	}
	if len(f.calls) != 4 {
		t.Errorf("fetched %v", f.calls)
	}
}

func TestThemesiaGridStopsWhenNothingNew(t *testing.T) {
	same := read(t, "themesia/grid-1.html")
	withFakeFetch(t, map[string]string{
		"https://a.test/manga/?page=1": same, "https://a.test/manga/?page=2": same, "https://a.test/manga/?page=3": same,
	})
	got, err := crawlers["themesia"].Crawl(context.Background(), themesiaSite, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 20 {
		t.Fatalf("a site ignoring page= must stop after one repeat, got %d", len(got))
	}
}

func TestThemesiaLimit(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://a.test/manga/?page=1": read(t, "themesia/grid-1.html"),
		"https://a.test/manga/?page=2": read(t, "themesia/grid-2.html"),
	})
	got, err := crawlers["themesia"].Crawl(context.Background(), themesiaSite, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 20 || len(f.calls) != 2 {
		t.Fatalf("got %d entries after %v", len(got), f.calls)
	}
}

func TestThemesiaFirstPageErrorIsReturned(t *testing.T) {
	withFakeFetch(t, nil)
	if _, err := crawlers["themesia"].Crawl(context.Background(), themesiaSite, 0); err == nil {
		t.Fatal("no list page at all must be an error")
	}
}

func TestThemesiaCancelled(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://a.test/manga/list-mode/": read(t, "themesia/list-mode.html")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := crawlers["themesia"].Crawl(ctx, themesiaSite, 0); err == nil {
		t.Fatal("a cancelled context must stop the crawl")
	}
}
