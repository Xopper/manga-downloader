// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestMangaboltTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://mangabolt.com/manga/gorgeous-irene/", true},
		{"https://www.mangabolt.com/chapter/gorgeous-irene-chapter-4/", true},
		{"https://mangabolt.com/latest", false},
		{"https://example.com/manga/gorgeous-irene/", false},
	}
	for _, tt := range tests {
		got, _ := NewMangabolt(&Grabber{URL: tt.url}).Test()
		if got != tt.want {
			t.Errorf("Test(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestMangaboltResolveSeriesURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://mangabolt.com/manga/gorgeous-irene/", "https://mangabolt.com/manga/gorgeous-irene/"},
		{"https://mangabolt.com/chapter/gorgeous-irene-chapter-4/", "https://mangabolt.com/manga/gorgeous-irene/"},
		{"https://mangabolt.com/chapter/blue-lock-chapter-10-5/?x=1", "https://mangabolt.com/manga/blue-lock/"},
	}
	for _, tt := range tests {
		m := NewMangabolt(&Grabber{URL: tt.url})
		if err := m.resolveSeriesURL(); err != nil {
			t.Fatal(err)
		}
		if m.URL != tt.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", tt.url, m.URL, tt.want)
		}
	}

	if err := NewMangabolt(&Grabber{URL: "https://mangabolt.com/chapter/oneshot/"}).resolveSeriesURL(); err == nil {
		t.Error("a chapter path with no number resolved")
	}
}

func mangaboltDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestMangaboltChapters(t *testing.T) {
	doc := mangaboltDoc(t, `<div id="main-content"><h1 class="my-3 font-bold">Gorgeous Irene</h1></div>
<div class="bg-bg-secondary p-3 rounded mb-3 shadow"><div class="grid grid-cols-4 items-center">
<div class="col-span-4 lg:col-span-3">
<a class="text-sm font-bold" href="https://mangabolt.com/chapter/gorgeous-irene-chapter-5/">Gorgeous Irene Chapter 5</a>
<div class="text-xs font-semibold text-text-muted uppercase">Armed Poker</div></div>
<div class="lg:col-span-1"><a class="uppercase text-xs" href="https://mangabolt.com/chapter/gorgeous-irene-chapter-5/">Read</a></div>
</div></div>
<div class="bg-bg-secondary p-3 rounded mb-3 shadow"><div class="grid grid-cols-4 items-center">
<div class="col-span-4 lg:col-span-3">
<a class="text-sm font-bold" href="https://mangabolt.com/chapter/gorgeous-irene-chapter-4/">Gorgeous Irene Chapter 4</a>
<div class="text-xs font-semibold text-text-muted uppercase"></div></div>
<div class="lg:col-span-1"><a class="uppercase text-xs" href="https://mangabolt.com/chapter/gorgeous-irene-chapter-4/">Read</a></div>
</div></div>`)

	chapters, errs := mangaboltChapters(doc, "https://mangabolt.com/manga/gorgeous-irene/")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	first := chapters[0].(*MangaboltChapter)
	if first.Number != 5 || first.Title != "Armed Poker" || first.ID != "/chapter/gorgeous-irene-chapter-5/" {
		t.Errorf("first chapter = %+v", first)
	}
	if got := ChapterLabel(chapters[1]); got != "Chapter 4" {
		t.Errorf("second label = %q", got)
	}
}

func TestMangaboltPages(t *testing.T) {
	doc := mangaboltDoc(t, `<div class="js-pages-container">
<div class="text-center"><img src="https://cdn.mangabolt.com/file/mangap/7667/10004000/1.jpeg" class="mb-3 mx-auto js-page" /></div>
<div class="text-center"><img data-src="https://cdn.mangabolt.com/file/mangap/7667/10004000/2.jpeg" src="data:image/gif;base64,R0lGOD" class="mb-3 mx-auto js-page lazy-load" />
<noscript><img src="https://cdn.mangabolt.com/file/mangap/7667/10004000/2.jpeg" class="mb-3 mx-auto js-page" /></noscript></div>
<div class="text-center"><img data-src="https://cdn.mangabolt.com/file/mangap/7667/10004000/3.jpeg" src="data:image/gif;base64,R0lGOD" class="mb-3 mx-auto js-page lazy-load" />
<noscript><img src="https://cdn.mangabolt.com/file/mangap/7667/10004000/3.jpeg" class="mb-3 mx-auto js-page" /></noscript></div>
</div>`)

	want := []string{
		"https://cdn.mangabolt.com/file/mangap/7667/10004000/1.jpeg",
		"https://cdn.mangabolt.com/file/mangap/7667/10004000/2.jpeg",
		"https://cdn.mangabolt.com/file/mangap/7667/10004000/3.jpeg",
	}
	if got := mangaboltPages(doc, "https://mangabolt.com/chapter/gorgeous-irene-chapter-4/"); !reflect.DeepEqual(got, want) {
		t.Errorf("pages = %v", got)
	}
}
