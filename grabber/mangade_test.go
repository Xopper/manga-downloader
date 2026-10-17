// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMangaDEComicID(t *testing.T) {
	cases := map[string]string{
		"https://mangade.io/comic/i-didnt-mean-to-seduce-the-male-lead-pid36543":          "36543",
		"https://mangade.io/comic/i-didnt-mean-to-seduce-the-male-lead-36543/chapter-120": "36543",
		"https://mangade.io/comic/ao-no-exorcist-pid48716/":                               "48716",
		"https://mangade.io/genres/action":                                                "",
	}
	for u, want := range cases {
		if got := NewMangaDE(&Grabber{URL: u}).comicID(); got != want {
			t.Errorf("comicID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestMangaDEFetch(t *testing.T) {
	// trimmed copies of api.mangade.io's answers
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/comics/36543/view":
			fmt.Fprint(w, `{"data":{"name":"I didn’t mean to seduce the male lead","news_chapters":[`+
				`{"id":"360339","name":"Chapter 120","slug":"chapter-120","chapter_number":"120"},`+
				`{"id":"360203","name":"Prologue","slug":"chapter-0","chapter_number":"0"}]}}`)
		case "/chapters/360339/view":
			fmt.Fprint(w, `{"data":{"chapter_images":[{"image":"https://s3.example.com/1.jpg"},{"image":"https://s3.example.com/2.jpg"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := mangadeAPI
	mangadeAPI = srv.URL
	defer func() { mangadeAPI = old }()

	m := NewMangaDE(&Grabber{URL: "https://mangade.io/comic/i-didnt-mean-to-seduce-the-male-lead-pid36543", Settings: &Settings{}})
	title, err := m.FetchTitle()
	if err != nil || title != "I didn’t mean to seduce the male lead" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 || chapters[0].GetNumber() != 120 || chapters[0].GetTitle() != "" || chapters[1].GetTitle() != "Prologue" {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	chapter, err := m.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://s3.example.com/2.jpg" || chapter.ID != "360339" {
		t.Errorf("chapter = %+v", chapter)
	}
}
