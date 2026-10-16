// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"testing"
)

func TestUtoonTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://www.utoon.us/en/manga/gachiakuta.html", true},
		{"https://utoon.us/en/manga/gachiakuta.html", true},
		{"https://www.utoon.net/manga/gachiakuta", false}, // hijacked/offline domain, not supported
		{"https://mangak.io/some-manga", false},
	}

	for _, c := range cases {
		u := Utoon{Grabber: &Grabber{URL: c.url}}
		got, err := u.Test()
		if err != nil {
			t.Errorf("Test(%q) returned unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestUtoonResolveSeriesURL(t *testing.T) {
	const chapterPath = "/en/gachiakuta/gachiakuta-chapter-167.html"

	mux := http.NewServeMux()
	// Minimal chapter page with manga-series link (goquery reverses attr order on output
	// so the resolver uses Find("a.manga-series") rather than regex on raw HTML)
	respond(mux, chapterPath, `<html><body><a class="manga-series" href="https://www.utoon.us/en/manga/gachiakuta.html">Gachiakuta</a></body></html>`)
	srv := serve(t, mux)

	seriesURL, err := utoonResolveSeriesURL(srv.URL + chapterPath)
	if err != nil {
		t.Fatalf("utoonResolveSeriesURL() error = %v", err)
	}
	if want := "https://www.utoon.us/en/manga/gachiakuta.html"; seriesURL != want {
		t.Errorf("utoonResolveSeriesURL() = %q, want %q", seriesURL, want)
	}
}

func TestUtoonAjaxConfigRe(t *testing.T) {
	html := `<script id="mangaverse-load-more-js-extra">
var mangaverse_ajax = {"ajax_url":"https://www.utoon.us/wp-admin/admin-ajax.php","nonce":"223204e60f","current_lang":"en"};
//# sourceURL=mangaverse-load-more-js-extra
</script>`

	m := utoonAjaxConfigRe.FindStringSubmatch(html)
	if len(m) < 3 {
		t.Fatalf("expected ajax config to be found, got %v", m)
	}
	if want := "https://www.utoon.us/wp-admin/admin-ajax.php"; m[1] != want {
		t.Errorf("ajax_url = %q, want %q", m[1], want)
	}
	if want := "223204e60f"; m[2] != want {
		t.Errorf("nonce = %q, want %q", m[2], want)
	}
}

// The image cdn refuses bursts, so the configured concurrency is capped
func TestUtoonGetMaxConcurrency(t *testing.T) {
	cases := []struct {
		set, want MaxConcurrency
	}{
		{MaxConcurrency{Chapters: 5, Pages: 10}, utoonMaxConcurrency},
		{MaxConcurrency{Chapters: 1, Pages: 2}, MaxConcurrency{Chapters: 1, Pages: 2}},
	}
	for _, c := range cases {
		u := NewUtoon(&Grabber{Settings: &Settings{MaxConcurrency: c.set}})
		if got := u.GetMaxConcurrency(); got != c.want {
			t.Errorf("GetMaxConcurrency() with %+v = %+v, want %+v", c.set, got, c.want)
		}
	}
}

// Utoon paces its pages, through the interface the downloader looks for
func TestUtoonIsPagePacer(t *testing.T) {
	var site Site = NewUtoon(&Grabber{Settings: &Settings{}})
	if _, ok := site.(PagePacer); !ok {
		t.Fatal("*Utoon doesn't implement PagePacer")
	}
}
