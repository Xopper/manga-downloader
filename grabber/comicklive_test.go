// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"reflect"
	"testing"
)

func TestComickLiveTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://comick.live/comic/cinderelle", true},
		{"https://comick.art/comic/dai-dark/SA1d7x3-chapter-57-en", true},
		{"https://comick.io/comic/dai-dark", false},
	}
	for _, tt := range tests {
		got, _ := NewComickLive(&Grabber{URL: tt.url}).Test()
		if got != tt.want {
			t.Errorf("Test(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestComickLiveResolveSeriesURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://comick.live/comic/cinderelle", "https://comick.live/comic/cinderelle"},
		{"https://comick.live/comic/cinderelle/", "https://comick.live/comic/cinderelle"},
		{"https://comick.live/comic/cinderelle/SA1d7x3-chapter-57-en", "https://comick.live/comic/cinderelle"},
		{"https://comick.art/comic/cinderelle/SA1d7x3-chapter-57-en?x=1#top", "https://comick.art/comic/cinderelle"},
	}
	for _, tt := range tests {
		c := NewComickLive(&Grabber{URL: tt.url})
		if err := c.resolveSeriesURL(); err != nil {
			t.Fatal(err)
		}
		if c.URL != tt.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", tt.url, c.URL, tt.want)
		}
	}

	if err := NewComickLive(&Grabber{URL: "https://comick.live/search"}).resolveSeriesURL(); err == nil {
		t.Error("a non-series URL resolved")
	}
}

func TestComickLiveFetch(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/comic/cinderelle", `<html><body><script type="application/json" id="comic-data">{"title":"I Woke Up As a Notorious Troublemaker!","slug":"cinderelle","langList":[{"label":"Select language","value":"settings"},{"label":"All languages","value":"all"},{"label":"English","value":"en"},{"label":"French","value":"fr"}]}</script></body></html>`)
	mux.HandleFunc("/api/comics/cinderelle/chapter-list", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("lang") != "en" {
			t.Errorf("lang = %q, want en", r.URL.Query().Get("lang"))
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"data":[{"hid":"JU6XGML","chap":"48","title":"","vol":null,"lang":"en","group_name":["Mangakakalot"]}],"pagination":{"current_page":2,"last_page":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"hid":"SA1d7x3","chap":"57","title":"The Ball","vol":"3","lang":"en","group_name":["Mangakakalot","Asura"]}],"pagination":{"current_page":1,"last_page":2}}`))
	})
	respond(mux, "/comic/cinderelle/SA1d7x3-chapter-57-en", `<html><body><script type="application/json" id="sv-data">{"chapter":{"images":[{"h":1280,"w":900,"url":"https://cdn1.comicknew.pictures/cinderelle/0_57/en/64be7829/0.webp"},{"url":"https://cdn1.comicknew.pictures/cinderelle/0_57/en/64be7829/1.webp"}]}}</script></body></html>`)
	srv := serve(t, mux)

	c := NewComickLive(&Grabber{URL: srv.URL + "/comic/cinderelle/SA1d7x3-chapter-57-en", Settings: &Settings{}})
	title, err := c.FetchTitle()
	if err != nil || title != "I Woke Up As a Notorious Troublemaker!" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	if got := c.LanguageOptions().Values; !reflect.DeepEqual(got, []string{"en", "fr"}) {
		t.Errorf("languages = %v", got)
	}

	chapters, errs := c.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2 (both list pages)", len(chapters))
	}
	first := chapters[0].(*ComickLiveChapter)
	if first.Number != 57 || first.Volume != 3 || first.ID != "SA1d7x3" || first.Title != "The Ball" ||
		!reflect.DeepEqual(first.GetGroups(), []string{"Mangakakalot", "Asura"}) || first.Path != "/comic/cinderelle/SA1d7x3-chapter-57-en" {
		t.Errorf("first chapter = %+v", first)
	}

	chapter, err := c.FetchChapter(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://cdn1.comicknew.pictures/cinderelle/0_57/en/64be7829/1.webp" ||
		chapter.ID != "SA1d7x3" || chapter.Group != first.Group {
		t.Errorf("chapter = %+v", chapter)
	}
}
