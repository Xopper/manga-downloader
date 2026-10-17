// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestHonkaiImpactSeriesURL cuts a reader URL back to its book
func TestHonkaiImpactSeriesURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://manga.honkaiimpact3.com/book/1011", "https://manga.honkaiimpact3.com/book/1011"},
		{"https://manga.honkaiimpact3.com/book/1011/3", "https://manga.honkaiimpact3.com/book/1011"},
		{"https://MANGA.honkaiimpact3.com/Book/1004/?x=1#top", "https://manga.honkaiimpact3.com/book/1004"},
		{"https://manga.honkaiimpact3.com/book", ""},
	}
	for _, c := range cases {
		h := NewHonkaiImpact(&Grabber{URL: c.in, Settings: &Settings{}})
		got, err := h.seriesURL()
		if c.want == "" {
			if err == nil {
				t.Errorf("seriesURL(%q) = %q, want an error", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("seriesURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	if ok, _ := NewHonkaiImpact(&Grabber{URL: "https://example.com/book/1", Settings: &Settings{}}).Test(); ok {
		t.Error("Test matched another domain")
	}
}

// TestHonkaiImpactChapters parses get_chapter, keeping the site's titles
func TestHonkaiImpactChapters(t *testing.T) {
	const js = `[{"id":"197","title":"13 His Reply - END","page":"11","bookid":"1011","chapterid":"14","timestamp":"2020-01-07 15:30:07"},
{"id":"184","title":"Prologue","page":"9","bookid":"1011","chapterid":"1","timestamp":"2019-12-01 15:30:07"}]`
	chapters, err := honkaiImpactParseChapters(strings.NewReader(js))
	if err != nil || len(chapters) != 2 {
		t.Fatalf("chapters = %v, %v", chapters, err)
	}
	c := chapters[0].(*HonkaiImpactChapter)
	if c.Number != 14 || ChapterLabel(c) != "13 His Reply - END" || c.URL != "https://manga.honkaiimpact3.com/book/1011/14" || c.ID != "1011/14" {
		t.Errorf("chapter = %+v", c)
	}
}

// TestHonkaiImpactPages reads data-original, not the spinner src
func TestHonkaiImpactPages(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<img class="lazy comic_img" src="/loading.gif" data-original="https://cdn.example/0001.jpg">
<img class="lazy comic_img" data-original="https://cdn.example/0002.jpg"><img class="logo" src="/logo.png">`))
	pages := honkaiImpactPages(doc)
	if len(pages) != 2 || pages[1].URL != "https://cdn.example/0002.jpg" {
		t.Errorf("pages = %+v", pages)
	}
}
