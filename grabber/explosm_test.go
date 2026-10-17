// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExplosmFetch(t *testing.T) {
	// a trimmed copy of the archive in explosm.net/comics' __NEXT_DATA__
	page := `<html><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"comicArchiveData":{` +
		`"2026":{"0":[{"file":null,"file_static":"https://static.explosm.net/2026/01/23141911/newyears2026.png","publish_at":"2026-01-01 00:01:43","slug":"new-years-2026"}]},` +
		`"2005":{"0":[{"file":"kApplesauce0001.jpg","file_static":null,"publish_at":"2005-01-26 00:01:00","slug":"kris-kapplesauce"},` +
		`{"file":"http://example.com/flash.swf","file_static":null,"publish_at":"2005-01-27 00:01:00","slug":"flash"}],` +
		`"1":[{"file":"rob.png","file_static":null,"publish_at":"2005-02-01 00:01:00","slug":"rob"}]}}}}}</script></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/comics" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(page))
	}))
	defer srv.Close()

	e := NewExplosm(&Grabber{URL: srv.URL + "/comics/new-years-2026", Settings: &Settings{}})
	chapters, errs := e.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	want := []struct {
		slug, image string
	}{
		{"kris-kapplesauce", "https://files.explosm.net/comics/kApplesauce0001.jpg"},
		{"rob", "https://files.explosm.net/comics/rob.png"},
		{"new-years-2026", "https://static.explosm.net/2026/01/23141911/newyears2026.png"},
	}
	for i, w := range want {
		chapter, err := e.FetchChapter(chapters[i])
		if err != nil {
			t.Fatal(err)
		}
		if chapter.Number != float64(i+1) || chapter.ID != w.slug || len(chapter.Pages) != 1 || chapter.Pages[0].URL != w.image {
			t.Errorf("chapter %d = %+v", i, chapter)
		}
	}
}

func TestExplosmTest(t *testing.T) {
	if ok, _ := NewExplosm(&Grabber{URL: "https://explosm.net/comics#2026-01"}).Test(); !ok {
		t.Error("explosm.net not recognised")
	}
	if ok, _ := NewExplosm(&Grabber{URL: "https://example.com/explosm.net"}).Test(); ok {
		t.Error("example.com recognised")
	}
}
