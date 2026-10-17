// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTwicomiTest(t *testing.T) {
	cases := map[string]bool{
		"https://twicomi.com/manga/sho_ho_Yamasan/2077417983929147523#1784130018000": true,
		"https://twicomi.com/manga/sho_ho_Yamasan":                                   true,
		"https://twicomi.com/ranking":                                                false,
		"https://example.com/manga/sho_ho_Yamasan":                                   false,
	}
	for u, want := range cases {
		if got, _ := NewTwicomi(&Grabber{URL: u}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestTwicomiFetch(t *testing.T) {
	// the author list over two pages of a 101-tweet author; tweet 2 has no
	// picture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/author/manga/list" || q.Get("screen_name") != "sho_ho_Yamasan" || q.Get("order") != "asc" {
			http.NotFound(w, r)
			return
		}
		first, last := 1, 100
		if q.Get("page_no") == "2" {
			first, last = 101, 101
		}
		var list []string
		for n := first; n <= last; n++ {
			images := fmt.Sprintf(`["https://pbs.twimg.com/media/m%d.jpg","https://pbs.twimg.com/media/n%d.jpg"]`, n, n)
			if n == 2 {
				images = `[]`
			}
			list = append(list, fmt.Sprintf(`{"author":{"name":"山本山","screen_name":"sho_ho_Yamasan"},"tweet":{"tweet_id":"%d","tweet_text":"Title %d\nmore","attach_image_urls":%s}}`, 1000+n, n, images))
		}
		fmt.Fprintf(w, `{"response":{"total_count":101,"manga_list":[%s]}}`, strings.Join(list, ","))
	}))
	defer srv.Close()
	old := twicomiAPI
	twicomiAPI = srv.URL
	defer func() { twicomiAPI = old }()

	tw := NewTwicomi(&Grabber{URL: "https://twicomi.com/manga/sho_ho_Yamasan/1003", Settings: &Settings{}})
	title, err := tw.FetchTitle()
	if err != nil || title != "山本山" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := tw.FetchChapters()
	if len(errs) > 0 || len(chapters) != 100 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	second := chapters[1].(*TwicomiChapter)
	if second.Number != 2 || second.Title != "Title 3" || second.ID != "1003" {
		t.Errorf("second = %+v", second.Chapter)
	}
	chapter, err := tw.FetchChapter(second)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://pbs.twimg.com/media/n3.jpg?name=orig" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
