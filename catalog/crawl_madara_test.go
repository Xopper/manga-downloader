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
	// the latest chapter is the row's first listed one; a row without a
	// chapter list has none
	for i, want := range map[int]float64{0: 26, 1: 100, 2: 12.5, 3: 0} {
		if got[i].Latest != want {
			t.Errorf("entry %d latest = %v, want %v", i, got[i].Latest, want)
		}
	}
}

func TestChapterNumber(t *testing.T) {
	cases := []struct {
		label, href string
		want        float64
	}{
		{"Chapter 1120", "", 1120},
		{" Chapter 26.5 ", "", 26.5},
		{"Ch. 7", "", 7},
		{"Capítulo 12", "", 12},
		{"Bölüm 3", "", 3},
		{"Chapter 100 - Part 2", "", 100},
		{"Vol.2 Chapter 15", "", 15},
		{"Episode 9", "", 9},
		{"", "https://m.test/manga/x/chapter-12-5/", 12.5},
		{"", "https://m.test/manga/x/chapter-40/", 40},
		{"Extra", "https://m.test/manga/x/extra/", 0},
		{"", "", 0},
	}
	for _, c := range cases {
		if got := chapterNumber(c.label, c.href); got != c.want {
			t.Errorf("chapterNumber(%q, %q) = %v, want %v", c.label, c.href, got, c.want)
		}
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
