// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"
)

const bakkinMainFixture = `{
"1_YRYR":{"dir":"1_YRYR","name":"Yuru Yuri","volumes":[
 {"dir":"v01","name":"Volume 1","chapters":[
  {"dir":"aCover","name":"Cover","pages":["manga/1_YRYR/v01/aCover/001.jpg"]},
  {"dir":"c001","name":"Chapter 1","pages":["manga/1_YRYR/v01/c001/01 [x].png","manga/1_YRYR/v01/c001/02.png"]},
  {"dir":"int01","name":"Interlude","pages":["manga/1_YRYR/v01/int01/1.png"]}]},
 {"dir":"v02","name":"Volume 2","chapters":[
  {"dir":"c002","name":"Chapter 2","pages":["manga/1_YRYR/v02/c002/1.png"]}]}]},
"3_Misc":{"dir":"3_Misc","name":"Doujins and One-shots","volumes":[
 {"dir":"10_Anthology1","name":"Anthology 1","chapters":[{"dir":"c000","name":"A","pages":["a.jpg"]}]},
 {"dir":"10_Anthology2","name":"Anthology 2","chapters":[{"dir":"c000","name":"B","pages":["b.jpg"]}]}]}
}`

// TestBakkinSeriesDir reads the series from series and chapter links
func TestBakkinSeriesDir(t *testing.T) {
	cases := map[string]string{
		"https://bakkin.moe/reader/#m=1_YRYR":                 "1_YRYR",
		"https://bakkin.moe/reader/#m=4_YYS&v=v01&c=c001":     "4_YYS",
		"https://bakkin.moe/reader/":                          "",
		"https://bakkin.moe/reader/?m=1_YRYR#v=v01&c=c001xyz": "",
	}
	for in, want := range cases {
		if got := bakkinSeriesDir(in); got != want {
			t.Errorf("bakkinSeriesDir(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestBakkinChapters numbers chapters by folder, extras after them, and by
// position when folders repeat
func TestBakkinChapters(t *testing.T) {
	s, err := bakkinFind(strings.NewReader(bakkinMainFixture), "1_YRYR")
	if err != nil {
		t.Fatal(err)
	}
	chapters := bakkinChapters(s)
	want := []struct {
		num   float64
		vol   int
		label string
	}{{0.01, 1, "Volume 1 - Cover"}, {1, 1, "Volume 1 - Chapter 1"}, {1.01, 1, "Volume 1 - Interlude"}, {2, 2, "Volume 2 - Chapter 2"}}
	if len(chapters) != len(want) {
		t.Fatalf("got %d chapters", len(chapters))
	}
	for i, w := range want {
		c := chapters[i].(*BakkinChapter)
		if c.Number != w.num || c.Volume != w.vol || ChapterLabel(c) != w.label {
			t.Errorf("chapter %d = %v v%d %q, want %+v", i, c.Number, c.Volume, ChapterLabel(c), w)
		}
	}

	misc, err := bakkinFind(strings.NewReader(bakkinMainFixture), "3_Misc")
	if err != nil {
		t.Fatal(err)
	}
	mc := bakkinChapters(misc)
	if len(mc) != 2 || mc[0].GetNumber() != 1 || mc[1].GetNumber() != 2 {
		t.Errorf("misc numbers = %v, %v", mc[0].GetNumber(), mc[1].GetNumber())
	}
	if _, err := bakkinFind(strings.NewReader(bakkinMainFixture), "9_None"); err == nil {
		t.Error("want an error for an unknown series")
	}
}

// TestBakkinPages builds escaped page URLs under /reader/
func TestBakkinPages(t *testing.T) {
	s, _ := bakkinFind(strings.NewReader(bakkinMainFixture), "1_YRYR")
	b := NewBakkin(&Grabber{URL: "https://bakkin.moe/reader/#m=1_YRYR", Settings: &Settings{}})
	ch, err := b.FetchChapter(bakkinChapters(s)[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Pages) != 2 || ch.Pages[0].URL != "https://bakkin.moe/reader/manga/1_YRYR/v01/c001/01%20%5Bx%5D.png" {
		t.Errorf("pages = %+v", ch.Pages)
	}
}
