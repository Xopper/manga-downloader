// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestManga18me runs manga18.me's YiiMadara pages through the PlainHTML
// selector that claims them
func TestManga18me(t *testing.T) {
	read := func(name string) *goquery.Document {
		b, err := os.ReadFile("testdata/manga18me/" + name)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(b)))
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}

	series := read("series.html")
	var site SiteSelector
	found := false
	for _, s := range plainHTMLSelectors {
		if series.Find(s.Rows).Length() > 0 {
			site, found = s, true
			break
		}
	}
	if !found || site.Image != ".read-content img" {
		t.Fatalf("the first selector matching the page is %+v, want manga18.me's", site)
	}
	m := PlainHTML{Grabber: &Grabber{URL: "https://manga18.me/manga/i-have-to-sleep-with-a-stranger"}, doc: series, rows: series.Find(site.Rows), site: site}

	if title, _ := m.FetchTitle(); title != "I Have To Sleep With A Stranger?" {
		t.Errorf("FetchTitle() = %q", title)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 || chapters[0].GetNumber() != 95 || chapters[1].GetNumber() != 94.5 {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	if u := chapters[0].(*PlainHTMLChapter).URL; u != "https://manga18.me/manga/i-have-to-sleep-with-a-stranger/chapter-95" {
		t.Errorf("chapter URL = %q", u)
	}

	chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter: Chapter{Number: 95}}, read("chapter.html"))
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://img-r2.manga18.me/uploads/3927/95/2-0b7ab.jpg" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
