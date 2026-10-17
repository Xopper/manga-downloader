// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

// TestMonochromeParseURL tells series links from chapter links
func TestMonochromeParseURL(t *testing.T) {
	cases := []struct{ in, kind, id string }{
		{"https://manga.d34d.one/manga/e7199099-60b0-49e9-a22e-998e748d705e", "manga", "e7199099-60b0-49e9-a22e-998e748d705e"},
		{"https://manga.d34d.one/chapters/0216A631-5c59-4b40-8730-12bb4c54f37c/?x=1", "chapters", "0216a631-5c59-4b40-8730-12bb4c54f37c"},
		{"https://manga.d34d.one/chapters/0216a631-5c59-4b40-8730-12bb4c54f37c/edit", "", ""},
		{"https://manga.d34d.one/", "", ""},
	}
	for _, c := range cases {
		kind, id, ok := monochromeParseURL(c.in)
		if ok != (c.kind != "") || kind != c.kind || id != c.id {
			t.Errorf("monochromeParseURL(%q) = %q %q %v", c.in, kind, id, ok)
		}
	}
	m := NewMonochrome(&Grabber{URL: "https://manga.d34d.one/manga/x", Settings: &Settings{}})
	if ok, _ := m.Test(); !ok || m.apiURL() != "https://api.manga.d34d.one" {
		t.Errorf("Test = %v, apiURL = %q", ok, m.apiURL())
	}
}

// TestMonochromeChapters keeps group, volume and builds the media URLs
func TestMonochromeChapters(t *testing.T) {
	var rows []monochromeChapterDTO
	if err := json.Unmarshal([]byte(`[{"name":"Blossom and Bear Fruit","volume":5,"number":64.5,"scanGroup":"Monochrome Scans","id":"c1","version":3,"mangaId":"m1","length":2},
{"name":"","volume":null,"number":65,"scanGroup":"","id":"c2","version":1,"mangaId":"m1","length":1}]`), &rows); err != nil {
		t.Fatal(err)
	}
	chapters := monochromeChapters(rows, "m1")
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters", len(chapters))
	}
	c := chapters[0].(*MonochromeChapter)
	if ChapterLabel(c) != "Chapter 64.5 - Blossom and Bear Fruit" || c.Volume != 5 || c.Group != "Monochrome Scans" || c.ID != "c1" {
		t.Errorf("chapter = %+v", c)
	}
	if ChapterLabel(chapters[1]) != "Chapter 65" {
		t.Errorf("label = %q", ChapterLabel(chapters[1]))
	}
	pages := monochromePages("https://api.manga.d34d.one", c)
	if len(pages) != 2 || pages[1].URL != "https://api.manga.d34d.one/media/m1/c1/2.jpg?version=3" {
		t.Errorf("pages = %+v", pages)
	}
}
