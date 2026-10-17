// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestKemono serves a Kemono instance whose creator has 52 posts, two
// listing pages: post N has a picture when N is even, a video otherwise
func newTestKemono(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/css" {
			http.Error(w, "bad accept", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/api/v1/fanbox/user/123/profile":
			// gzipped whatever the request asked, as kemono.cr does
			zw := gzip.NewWriter(w)
			fmt.Fprint(zw, `{"id":"123","name":"innereye","service":"fanbox"}`)
			zw.Close()
		case "/api/v1/fanbox/user/123/posts":
			offset := r.URL.Query().Get("o")
			var posts []string
			// newest first: post 52 down to post 1
			first, last := 52, 3
			if offset == "50" {
				first, last = 2, 1
			}
			for n := first; n >= last; n-- {
				file := fmt.Sprintf(`{"name":"v.mp4","path":"/aa/bb/%d.mp4"}`, n)
				attachments := `[]`
				if n%2 == 0 {
					file = fmt.Sprintf(`{"name":"cover.jpeg","path":"/aa/bb/%d.jpeg"}`, n)
					attachments = fmt.Sprintf(`[{"name":"cover.jpeg","path":"/aa/bb/%d.jpeg"},{"name":"b.zip","path":"/aa/bb/%d.zip"},{"name":"c.PNG","path":"/aa/bb/%dc.PNG"}]`, n, n, n)
				}
				posts = append(posts, fmt.Sprintf(`{"id":"%d","title":"Post %d","file":%s,"attachments":%s}`, 1000+n, n, file, attachments))
			}
			fmt.Fprint(w, "["+strings.Join(posts, ",")+"]")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	kemonoHosts["127.0.0.1"] = kemonoHost{userAgent: "test-agent"}
	t.Cleanup(func() { delete(kemonoHosts, "127.0.0.1") })

	return srv
}

func TestKemonoTest(t *testing.T) {
	cases := map[string]bool{
		"https://coomer.st/onlyfans/user/grantducati":              true,
		"https://kemono.cr/fanbox/user/10951069/post/12521461":     true,
		"https://pawchive.pw/fanbox/user/10951069?o=50":            true,
		"https://kemono.cr/artists":                                false,
		"https://example.com/fanbox/user/10951069":                 false,
		"https://example.com/?u=https://coomer.st/onlyfans/user/x": false,
	}
	for u, want := range cases {
		got, err := NewKemono(&Grabber{URL: u}).Test()
		if err != nil || got != want {
			t.Errorf("Test(%q) = %v, %v, want %v", u, got, err, want)
		}
	}
}

func TestKemonoFetch(t *testing.T) {
	srv := newTestKemono(t)
	// a post URL lists the whole creator
	k := NewKemono(&Grabber{URL: srv.URL + "/fanbox/user/123/post/1052", Settings: &Settings{}})

	title, err := k.FetchTitle()
	if err != nil || title != "innereye" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := k.FetchChapters()
	if len(errs) > 0 || len(chapters) != 26 {
		t.Fatalf("FetchChapters() = %d chapters, %v", len(chapters), errs)
	}
	// the oldest post with pictures is chapter 1
	first := chapters[0].(*KemonoChapter)
	last := chapters[25].(*KemonoChapter)
	if first.Number != 1 || first.Title != "Post 2" || first.ID != "1002" || last.Number != 26 || last.ID != "1052" {
		t.Errorf("first = %+v, last = %+v", first.Chapter, last.Chapter)
	}

	chapter, err := k.FetchChapter(last)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{srv.URL + "/data/aa/bb/52.jpeg", srv.URL + "/data/aa/bb/52c.PNG"}
	if len(chapter.Pages) != len(want) {
		t.Fatalf("pages = %+v, want %v", chapter.Pages, want)
	}
	for i, p := range chapter.Pages {
		if p.URL != want[i] || p.Number != int64(i+1) || p.Headers["User-Agent"] != "test-agent" {
			t.Errorf("page %d = %+v, want %s", i, p, want[i])
		}
	}
}

func TestKemonoMaxConcurrency(t *testing.T) {
	k := NewKemono(&Grabber{URL: "https://coomer.st/onlyfans/user/x", Settings: &Settings{MaxConcurrency: MaxConcurrency{Chapters: 5, Pages: 10}}})
	if mc := k.GetMaxConcurrency(); mc.Chapters != 1 || mc.Pages != 4 {
		t.Errorf("GetMaxConcurrency() = %+v", mc)
	}
}
