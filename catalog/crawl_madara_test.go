// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"testing"
)

var madaraSite = SiteInfo{Site: "m.test", Home: "https://m.test", List: "https://m.test/manga/page/{page}/"}

func TestMadaraPagesUntil404(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://m.test/manga/page/1/": read(t, "madara/page-1.html"),
		"https://m.test/manga/page/2/": read(t, "madara/page-2.html"),
	})
	got, err := crawlers["madara"].Crawl(context.Background(), madaraSite, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 24 {
		t.Fatalf("got %d entries", len(got))
	}
	if got[0].Title != "The Genius Assassin’s Second Life Begins at the Academy" ||
		got[0].URL != "https://www.mangaread.org/manga/the-genius-assassins-second-life-begins-at-the-academy/" {
		t.Errorf("first: %+v", got[0])
	}
	if len(f.calls) != 3 {
		t.Errorf("the 404 on page 3 should end the walk: %v", f.calls)
	}
}

func TestMadaraStopsWhenNothingNew(t *testing.T) {
	same := read(t, "madara/page-1.html")
	withFakeFetch(t, map[string]string{
		"https://m.test/manga/page/1/": same, "https://m.test/manga/page/2/": same, "https://m.test/manga/page/3/": same,
	})
	got, err := crawlers["madara"].Crawl(context.Background(), madaraSite, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 12 {
		t.Fatalf("got %d", len(got))
	}
}

func TestMadaraLimit(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://m.test/manga/page/1/": read(t, "madara/page-1.html"),
		"https://m.test/manga/page/2/": read(t, "madara/page-2.html"),
	})
	got, err := crawlers["madara"].Crawl(context.Background(), madaraSite, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 12 || len(f.calls) != 1 {
		t.Fatalf("got %d entries after %v", len(got), f.calls)
	}
}

func TestMadaraFirstPageMissing(t *testing.T) {
	withFakeFetch(t, nil)
	if _, err := crawlers["madara"].Crawl(context.Background(), madaraSite, 0); err == nil {
		t.Fatal("a missing archive must be an error")
	}
}
