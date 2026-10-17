// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"reflect"
	"testing"
)

func TestBbatoTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://bato1.com/manga/hey-nana", true},
		{"https://bbato.com/read/hey-nana/chapter-5", true},
		{"https://bato1.com/home", false},
		{"https://bato.to/series/123", false},
	}
	for _, tt := range tests {
		got, _ := NewBbato(&Grabber{URL: tt.url}).Test()
		if got != tt.want {
			t.Errorf("Test(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestBbatoResolveSeriesURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://bato1.com/manga/hey-nana", "https://bato1.com/manga/hey-nana"},
		{"https://bato1.com/manga/hey-nana/", "https://bato1.com/manga/hey-nana"},
		{"https://bato1.com/read/hey-nana/chapter-5?x=1", "https://bato1.com/manga/hey-nana"},
	}
	for _, tt := range tests {
		b := NewBbato(&Grabber{URL: tt.url})
		if err := b.resolveSeriesURL(); err != nil {
			t.Fatal(err)
		}
		if b.URL != tt.want || b.slug != "hey-nana" {
			t.Errorf("resolveSeriesURL(%q) = %q (slug %q), want %q", tt.url, b.URL, b.slug, tt.want)
		}
	}
}

func TestBbatoFetch(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/manga/hey-nana", `<h1 itemprop="name">Hey, Nana</h1>`)
	mux.HandleFunc("/get-chapter-list", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Requested-With") != "XMLHttpRequest" || r.URL.Query().Get("slug") != "hey-nana" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[
{"comic_id":77758,"chapter_id":22,"chapter_name":"Chapter 23","chapter_slug":"chapter-23","chapter_num":23},
{"comic_id":77758,"chapter_id":3,"chapter_name":"Chapter 1.5: Extra","chapter_slug":"chapter-1-5"}],"total":2}`))
	})
	respond(mux, "/read/hey-nana/chapter-23", `<div class="chapter-images-container"><div class="pages longstrip">
<div class="page fit-w notice-page"><div class="img notice-img"><a href="https://discord.com/invite/x"><img src='/assets/banner/short_720_v2.gif'></a></div></div>
<div class="page fit-w"><div class="img"><img data-number='0' class='fit-w image-thumb' src='https://cdn5.sags2mobile.com/hey-nana/22/0.webp'></div></div>
<div class="page fit-w"><div class="img"><img data-number='1' class='fit-w image-thumb' data-src='https://cdn5.sags2mobile.com/hey-nana/22/1.webp' src='data:image/gif;base64,R0lGOD'></div></div>
</div></div>`)
	srv := serve(t, mux)

	b := NewBbato(&Grabber{URL: srv.URL + "/read/hey-nana/chapter-5", Settings: &Settings{}})
	title, err := b.FetchTitle()
	if err != nil || title != "Hey, Nana" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := b.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	first := chapters[0].(*BbatoChapter)
	if first.Number != 23 || first.Title != "" || first.ID != "chapter-23" {
		t.Errorf("first chapter = %+v", first)
	}
	if second := chapters[1].(*BbatoChapter); second.Number != 1.5 || second.Title != "Extra" {
		t.Errorf("second chapter = %+v", second)
	}

	chapter, err := b.FetchChapter(first)
	if err != nil {
		t.Fatal(err)
	}
	var urls []string
	for _, p := range chapter.Pages {
		urls = append(urls, p.URL)
	}
	want := []string{"https://cdn5.sags2mobile.com/hey-nana/22/0.webp", "https://cdn5.sags2mobile.com/hey-nana/22/1.webp"}
	if !reflect.DeepEqual(urls, want) {
		t.Errorf("pages = %v, want %v", urls, want)
	}
}
