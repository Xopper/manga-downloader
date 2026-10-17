// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestLolobunSeriesURL cuts a reader URL back to its comic
func TestLolobunSeriesURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://www.lolobun.com/c/20106", "https://www.lolobun.com/c/20106"},
		{"https://lolobun.com/c/20106/71/?x=1", "https://www.lolobun.com/c/20106"},
		{"https://www.lolobun.com/", ""},
	}
	for _, c := range cases {
		got, err := NewLolobun(&Grabber{URL: c.in, Settings: &Settings{}}).seriesURL()
		if (c.want == "") != (err != nil) || got != c.want {
			t.Errorf("seriesURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

// TestLolobunChapters numbers extras after their chapter and flags locks
func TestLolobunChapters(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div class="catalog-list">
<div class="catalog-item"><div class="title"><a href="/c/20106/71">chapter 1</a></div><div class="icon-box"></div></div>
<div class="catalog-item"><div class="title"><a href="/c/20106/80">Extra 01</a></div><div class="icon-box"></div></div>
<div class="catalog-item"><div class="title"><a href="/c/20106/746">chapter173</a></div>
<div class="icon-box"><img class="icon" src="/image/catalog/icon_lock.png"></div></div>
</div>`))
	chapters := lolobunChapters(doc)
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters", len(chapters))
	}
	want := []struct {
		num    float64
		label  string
		locked bool
	}{{1, "Chapter 1", false}, {1.01, "Extra 01", false}, {173, "Chapter 173", true}}
	for i, w := range want {
		c := chapters[i].(*LolobunChapter)
		if c.Number != w.num || ChapterLabel(c) != w.label || c.Locked != w.locked {
			t.Errorf("chapter %d = %v %q %v, want %+v", i, c.Number, ChapterLabel(c), c.Locked, w)
		}
	}
}

// TestLolobunPages reads getChapterPic, and its empty list for locked ones
func TestLolobunPages(t *testing.T) {
	pages, err := lolobunPages(strings.NewReader(`{"status":{"errorCode":200,"msg":null},"data":["https://c/1.jpg","https://c/2.jpg"]}`))
	if err != nil || len(pages) != 2 || pages[1].URL != "https://c/2.jpg" {
		t.Errorf("pages = %+v, %v", pages, err)
	}
	pages, err = lolobunPages(strings.NewReader(`{"status":{"errorCode":200,"msg":null},"data":[]}`))
	if err != nil || len(pages) != 0 {
		t.Errorf("locked pages = %+v, %v", pages, err)
	}
	if _, err = lolobunPages(strings.NewReader(`{"status":{"errorCode":500,"msg":"nope"},"data":null}`)); err == nil {
		t.Error("want an error for a failed status")
	}
}
