// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestKemonoCreator(t *testing.T) {
	cases := []struct {
		in, service, id string
		wantErr         bool
	}{
		{"https://kemono.cr/patreon/user/141173059", "patreon", "141173059", false},
		{"https://kemono.cr/patreon/user/141173059/", "patreon", "141173059", false},
		{"https://kemono.cr/fanbox/user/49494721/post/1234?o=50", "fanbox", "49494721", false},
		{"https://kemono.su/fantia/user/77", "fantia", "77", false},
		{"https://kemono.cr/artists", "", "", true},
	}
	for _, c := range cases {
		service, id, err := (&Kemono{Grabber: &Grabber{URL: c.in}}).creator()
		if (err != nil) != c.wantErr || service != c.service || id != c.id {
			t.Errorf("creator(%q) = %q, %q, %v", c.in, service, id, err)
		}
	}
}

func TestKemonoTest(t *testing.T) {
	cases := map[string]bool{
		"https://kemono.cr/patreon/user/1":    true,
		"https://kemono.su/patreon/user/1":    true,
		"https://kemono.party/patreon/user/1": true,
		"https://coomer.st/onlyfans/user/x":   false,
		"https://kemono.cr.example/x":         false,
	}
	for u, want := range cases {
		if got, _ := (&Kemono{Grabber: &Grabber{URL: u}}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestKemonoChapters(t *testing.T) {
	posts := []kemonoPost{
		{ID: "3", Title: "Newest", Published: "2025-03-01T00:00:00", File: kemonoFile{Path: "/c/c/c.png"}},
		{ID: "2", Title: "Text only", Published: "2025-02-01T00:00:00"},
		{ID: "1", Title: " Oldest ", Published: "2025-01-01T00:00:00",
			File:        kemonoFile{Path: "/a/a/a.jpg"},
			Attachments: []kemonoFile{{Path: "/a/a/a.jpg"}, {Path: "/b/b/b.zip"}, {Path: "/d/d/d.webp"}}},
	}
	chapters := kemonoChapters(posts)
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	first := chapters[0].(*KemonoChapter)
	if first.ID != "1" || first.Number != 1 || first.Title != "Oldest" {
		t.Errorf("first = %+v", first.Chapter)
	}
	if len(first.Paths) != 2 || first.Paths[0] != "/a/a/a.jpg" || first.Paths[1] != "/d/d/d.webp" {
		t.Errorf("first paths = %v", first.Paths)
	}
	if second := chapters[1].(*KemonoChapter); second.ID != "3" || second.Number != 2 {
		t.Errorf("second = %+v", second.Chapter)
	}
}

func TestKemonoURLs(t *testing.T) {
	k := &Kemono{Grabber: &Grabber{URL: "https://kemono.cr/patreon/user/1"}}
	if got := k.fullURL("/73/b6/x.jpg"); got != "https://kemono.cr/data/73/b6/x.jpg" {
		t.Errorf("fullURL = %q", got)
	}
	if got := k.thumbnailURL("/73/b6/x.jpg"); got != "https://img.kemono.cr/thumbnail/data/73/b6/x.jpg" {
		t.Errorf("thumbnailURL = %q", got)
	}
}

// the API answers in pages of 50, sometimes gzipped although no gzip was
// asked for, and only to `Accept: text/css`
func TestKemonoFetch(t *testing.T) {
	gz := func(s string) []byte {
		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		w.Write([]byte(s))
		w.Close()
		return b.Bytes()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/patreon/user/9/profile", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/css" {
			http.Error(w, "send Accept: text/css", http.StatusForbidden)
			return
		}
		w.Write(gz(`{"id":"9","name":"Some Artist ","service":"patreon","post_count":51}`))
	})
	mux.HandleFunc("/api/v1/patreon/user/9/posts", func(w http.ResponseWriter, r *http.Request) {
		o, _ := strconv.Atoi(r.URL.Query().Get("o"))
		var b bytes.Buffer
		b.WriteString("[")
		for i := o; i < min(o+kemonoPageSize, 51); i++ {
			if i > o {
				b.WriteString(",")
			}
			// newest first: post 50 is the newest
			id := 50 - i
			fmt.Fprintf(&b, `{"id":"%d","title":"Post %d","published":"2025-01-01T00:00:%02d","file":{"name":"a.jpg","path":"/p/%d.jpg"},"attachments":[]}`, id, id, id, id)
		}
		b.WriteString("]")
		w.Write(b.Bytes())
	})
	mux.HandleFunc("/data/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	k := NewKemono(&Grabber{URL: srv.URL + "/patreon/user/9/post/12", Settings: &Settings{}})
	title, err := k.FetchTitle()
	if err != nil {
		t.Fatal(err)
	}
	if title != "Some Artist" {
		t.Errorf("title = %q", title)
	}
	chapters, errs := k.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 51 {
		t.Fatalf("got %d chapters, want 51", len(chapters))
	}
	if c := chapters[0].(*KemonoChapter); c.ID != "0" || c.Number != 1 {
		t.Errorf("first chapter = %+v", c.Chapter)
	}
	chapter, err := k.FetchChapter(chapters[50])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 1 || chapter.Pages[0].URL != srv.URL+"/data/p/50.jpg" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
