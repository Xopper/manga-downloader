// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestSingleMangaChapterLinks(t *testing.T) {
	// w9.kaijimanga.com: the main series has most of the links, a side
	// series a few, and the widget repeats them
	var b strings.Builder
	b.WriteString(`<html><head><link rel="stylesheet" href="/wp-content/themes/toivo-lite/style.css"></head><body>`)
	for i := 1; i <= 6; i++ {
		link := `<li><a href="https://w9.kaiji.test/manga/kaiji-chapter-` + strconv.Itoa(i) + `/">Kaiji, Chapter ` + strconv.Itoa(i) + `</a></li>`
		b.WriteString(link + link)
	}
	b.WriteString(`<li><a href="https://w9.kaiji.test/manga/tobaku-kaiji-chapter-1/">Tobaku Kaiji, Chapter 1</a></li>`)
	b.WriteString(`<li><a href="https://kaiji.test/about/">About</a></li></body></html>`)
	doc := docFromHTML(t, b.String())
	doc.Url, _ = url.Parse("https://kaiji.test/")

	links := smChapterLinks(doc)
	if len(links) != 6 {
		t.Fatalf("got %d links, want the main series' 6", len(links))
	}
	if name := smSeriesName(links); name != "Kaiji" {
		t.Errorf("series name = %q", name)
	}

	// a page listing several series' latest chapters lists none of them
	b.Reset()
	for i := 1; i <= 6; i++ {
		b.WriteString(`<a href="/manga/a/chapter-` + strconv.Itoa(i) + `/">a</a><a href="/manga/b/chapter-` + strconv.Itoa(i) + `/">b</a>`)
	}
	doc = docFromHTML(t, b.String())
	doc.Url, _ = url.Parse("https://madara.test/")
	if links := smChapterLinks(doc); links != nil {
		t.Errorf("a latest-updates page gave %v", links)
	}
}

func TestSingleMangaChapterNumber(t *testing.T) {
	cases := []struct {
		link smLink
		want float64
	}{
		// the text is right where the URL has WordPress' duplicate suffix
		{smLink{URL: "https://s.test/manga/one-punch-man-chapter-240-1/", Text: "One-Punch Man – Chapter 240"}, 240},
		// a link that says only "Read" has its number in its URL
		{smLink{URL: "https://s.test/chapter/mob-psycho-100-chapter-100-7/", Text: "Read"}, 100.7},
		{smLink{URL: "https://s.test/im-an-evil-god-manhua-chapter-567/", Text: "I'm An Evil God Manhua Chapter 567"}, 567},
	}
	for _, c := range cases {
		if got, ok := smChapterNumber(c.link); !ok || got != c.want {
			t.Errorf("smChapterNumber(%+v) = %v, %v; want %v", c.link, got, ok, c.want)
		}
	}
}

func TestSingleMangaNames(t *testing.T) {
	links := []smLink{{Text: "I'm An Evil God Manhua Chapter 2"}, {Text: "I'm An Evil God Manhua Chapter 1"}}
	if got := smSeriesName(links); got != "I'm An Evil God" {
		t.Errorf("smSeriesName = %q", got)
	}
	cases := map[string]string{
		"Mob Psycho 100 - Read One Punch Man Manga Online":    "Mob Psycho 100",
		"Read The Beginning After The End Manga Online - All": "The Beginning After The End",
		"One-Punch Man Manga Online - [All Chapters]":         "One-Punch Man",
	}
	for title, want := range cases {
		if got := smPageName(title); got != want {
			t.Errorf("smPageName(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestSingleMangaRESTList(t *testing.T) {
	// thegirlfromrandomchatting.com draws its list from a REST route, and a
	// chapter post lazy-loads its pages and asks for no referrer
	srv := wcTestServer(t, map[string]string{
		"/": `<html><head><link rel="stylesheet" href="/wp-content/themes/toivo-lite/style.css"></head>
			<body><h1>The Girl</h1><div id="mg-chapters-list-container"></div></body></html>`,
		"/wp-json/mg/v1/chapters-list": `[{"title":"The Girl, Chapter 2","url":"SRV/manga/the-girl-chapter-2/"},
			{"title":"The Girl, Chapter 1","url":"SRV/manga/the-girl-chapter-1/"}]`,
		"/manga/the-girl-chapter-1/": `<html><head><meta name="referrer" content="no-referrer"></head><body><div class="entry-content">
			<img src="data:image/svg+xml,x" data-lazy-src="https://img.test/1/001.png">
			<img src="https://img.test/1/002.png"></div></body></html>`,
	})
	s := NewSingleManga(&Grabber{URL: srv.URL + "/", Settings: &Settings{}})
	if ok, err := s.Test(); !ok || err != nil {
		t.Fatalf("Test() = %v, %v", ok, err)
	}
	title, err := s.FetchTitle()
	if err != nil || title != "The Girl" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := s.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 || chapters[0].GetNumber() != 1 {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	ch, err := s.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Pages) != 2 || ch.Pages[0].URL != "https://img.test/1/001.png" || !ch.Pages[0].NoReferer {
		t.Errorf("pages = %+v", ch.Pages)
	}
}
