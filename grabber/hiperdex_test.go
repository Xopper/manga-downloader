// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

func TestHiperdexSlug(t *testing.T) {
	cases := map[string]string{
		"https://hiperdex.tv/manga/im-no-heroine":       "im-no-heroine",
		"https://hiperdex.tv/manga/im-no-heroine/":      "im-no-heroine",
		"https://hiperdex.tv/manga/im-no-heroine/77":    "im-no-heroine",
		"https://www.hiperdex.tv/manga/im-no-heroine?x": "im-no-heroine",
	}
	for in, want := range cases {
		if got, err := hiperdexSlug(in); err != nil || got != want {
			t.Errorf("hiperdexSlug(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := hiperdexSlug("https://hiperdex.tv/"); err == nil {
		t.Error("hiperdexSlug(home) found a series")
	}
}

func TestHiperdexBatch(t *testing.T) {
	results, err := hiperdexResults(`[{"result":{"data":{"json":null}}},{"result":{"data":{"json":[
		{"id":2,"number":77,"title":null,"language":"en","status":"published","isDeleted":false},
		{"id":1,"number":76.5,"title":"Side story","language":"en","status":"published","isDeleted":false},
		{"id":3,"number":78,"title":null,"language":"en","status":"scheduled","isDeleted":false}]}}}]`)
	if err != nil {
		t.Fatal(err)
	}
	list := []hiperdexChapterJSON{}
	if err = json.Unmarshal(results[1], &list); err != nil {
		t.Fatal(err)
	}
	chapters := hiperdexChapters(list)
	if len(chapters) != 2 || chapters[1].GetNumber() != 76.5 || ChapterLabel(chapters[1]) != "Chapter 76.5 - Side story" {
		t.Errorf("chapters = %v", chapters)
	}

	pages := hiperdexPages([]json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`{"id":9999}`),
		json.RawMessage(`[{"pageOrder":2,"webpUrl":"https://i1.example.com/002.webp","avifUrl":null},{"pageOrder":1,"webpUrl":"https://i1.example.com/001.webp","avifUrl":"https://i1.example.com/001.avif"}]`),
	})
	if len(pages) != 2 || pages[0].URL != "https://i1.example.com/001.avif" || pages[1].URL != "https://i1.example.com/002.webp" {
		t.Errorf("pages = %v", pages)
	}

	if _, err := hiperdexResults(`{"error":"Unauthorized"}`); err == nil {
		t.Error("hiperdexResults() accepted an error answer")
	}
}
