// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestToonzSeriesPath(t *testing.T) {
	cases := map[string]string{
		"https://toonz.to/manhwa/ill-dream-of-you":              "/manhwa/ill-dream-of-you",
		"https://toonz.to/manhwa/ill-dream-of-you/":             "/manhwa/ill-dream-of-you",
		"https://toonz.to/manhwa/ill-dream-of-you/chapter/0.01": "/manhwa/ill-dream-of-you",
		"https://www.toonz.to/manhua/19-days?x=1":               "/manhwa/19-days",
	}
	for in, want := range cases {
		if got, err := toonzSeriesPath(in); err != nil || got != want {
			t.Errorf("toonzSeriesPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := toonzSeriesPath("https://toonz.to/"); err == nil {
		t.Error("toonzSeriesPath(home) found a series")
	}
}

func TestToonzChaptersAndImages(t *testing.T) {
	title := "Prologue."
	chapters := toonzChapters("https://toonz.to/manhwa/x", []toonzChapterJSON{
		{ID: 2, ChapterNumber: "1.00", Slug: "chapter-1"},
		{ID: 1, ChapterNumber: "0.01", Title: &title, Slug: "chapter-0.01"},
	})
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters", len(chapters))
	}
	c := chapters[1].(*ToonzChapter)
	if c.Number != 0.01 || c.Path != "https://toonz.to/manhwa/x/chapter/0.01" || ChapterLabel(c) != "Chapter 0.01 - Prologue." {
		t.Errorf("chapter = %+v, label %q", c, ChapterLabel(c))
	}

	payload := `1:["$","div",null,{"comicId":1,"images":[{"id":1,"filename":"chapters/1/2/001.webp","sortOrder":1},{"filename":"/chapters/1/2/002.webp"}]}]`
	files, err := toonzImages(payload)
	if err != nil || len(files) != 2 || files[0] != "chapters/1/2/001.webp" {
		t.Errorf("toonzImages() = %v, %v", files, err)
	}
	if _, err := toonzImages(`{"nothing":1}`); err == nil {
		t.Error("toonzImages() found pictures in a payload without any")
	}
}
