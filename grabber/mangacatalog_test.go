// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func mangaCatalogDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestMangaCatalogTest matches the network's domains with any wN. subdomain
func TestMangaCatalogTest(t *testing.T) {
	cases := map[string]bool{
		"https://ww13.readonepiece.com/manga/monsters/":     true,
		"https://ww8.read7deadlysins.com/manga/x/":          true,
		"https://ww12.tokyoghoulre.com/chapter/x-chapter-1": true,
		"https://readchainsawman.com/manga/look-back":       true,
		"https://readberserk.com/":                          false,
		"https://example.com/?u=readonepiece.com":           false,
	}
	for u, want := range cases {
		got, err := NewMangaCatalog(&Grabber{URL: u, Settings: &Settings{}}).Test()
		if err != nil || got != want {
			t.Errorf("Test(%q) = %v, %v; want %v", u, got, err, want)
		}
	}
}

// TestMangaCatalogSeries parses the title (not the banner h1) and the rows
func TestMangaCatalogSeries(t *testing.T) {
	doc := mangaCatalogDoc(t, `<html><head><meta property="og:title" content="MONSTERS"></head><body>
<div class="container"><a href="/"><h1>Read One Piece Manga Online</h1></a></div>
<div class="container"><h1 class="my-3">MONSTERS</h1></div>
<div class="bg-bg-secondary p-3"><div class="grid grid-cols-4">
 <div class="col-span-4 lg:col-span-3">
  <a href="https://ww13.readonepiece.com/chapter/monsters-chapter-1-5/">MONSTERS Chapter 1.5</a>
  <div class="text-xs font-semibold">Hundred Dragon</div>
 </div>
 <div class="lg:col-span-1"><a href="https://ww13.readonepiece.com/chapter/monsters-chapter-1-5/">Read</a></div>
</div></div>
<div class="bg-bg-secondary p-3"><div class="grid grid-cols-4">
 <div class="col-span-4"><a href="https://ww13.readonepiece.com/chapter/monsters-chapter-0/">MONSTERS Chapter 0</a></div>
</div></div>
</body></html>`)
	if got := mangaCatalogTitle(doc); got != "MONSTERS" {
		t.Errorf("title = %q", got)
	}
	chapters := mangaCatalogChapters(doc)
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	want := []struct {
		num   float64
		label string
	}{{1.5, "Chapter 1.5 - Hundred Dragon"}, {0, "Chapter 0"}}
	for i, w := range want {
		if chapters[i].GetNumber() != w.num || ChapterLabel(chapters[i]) != w.label {
			t.Errorf("chapter %d = %v %q, want %v %q", i, chapters[i].GetNumber(), ChapterLabel(chapters[i]), w.num, w.label)
		}
	}
	if id := chapters[1].(*MangaCatalogChapter).ID; id != "/chapter/monsters-chapter-0/" {
		t.Errorf("ID = %q", id)
	}
}

// TestMangaCatalogPages reads src for the first page and data-src for the rest
func TestMangaCatalogPages(t *testing.T) {
	doc := mangaCatalogDoc(t, `<div class="js-pages-container">
<img src="https://cdn.example/1.jpeg" class="js-page">
<img data-src="https://cdn.example/2.jpeg" src="data:image/gif;base64,R0lG" class="js-page lazy-load">
<noscript><img src="https://cdn.example/2.jpeg" class="js-page"></noscript>
<img src="data:image/gif;base64,R0lG" class="js-page">
</div>`)
	pages := mangaCatalogPages(doc)
	if len(pages) != 2 || pages[0].URL != "https://cdn.example/1.jpeg" || pages[1].URL != "https://cdn.example/2.jpeg" {
		t.Errorf("pages = %+v", pages)
	}
}

// TestMangaCatalogSeriesLink finds the reader's "View all chapters" option
func TestMangaCatalogSeriesLink(t *testing.T) {
	cases := []struct{ html, want string }{
		{`<select><option value="https://ww13.readonepiece.com/manga/monsters/">View all chapters</option>
<option value="https://ww13.readonepiece.com/chapter/monsters-chapter-0/" selected>CH# 0</option></select>`,
			"https://ww13.readonepiece.com/manga/monsters/"},
		{`<select><option value="https://ww13.readonepiece.com/chapter/monsters-chapter-0/">CH# 0</option></select>`, ""},
	}
	for _, c := range cases {
		if got := mangaCatalogSeriesLink(mangaCatalogDoc(t, c.html)); got != c.want {
			t.Errorf("series link = %q, want %q", got, c.want)
		}
	}
}
