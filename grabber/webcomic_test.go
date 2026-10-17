// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// wcTestServer serves pages by path (the query included when the key has
// one), 404 for any other. SRV in a page is the server's own URL.
func wcTestServer(t *testing.T, pages map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.RequestURI()]
		if !ok {
			body, ok = pages[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(strings.TrimSpace(body), "[") || strings.HasPrefix(strings.TrimSpace(body), "{") {
			w.Header().Set("Content-Type", "application/json")
		}
		w.Write([]byte(strings.ReplaceAll(body, "SRV", "http://"+r.Host)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWcNumbers(t *testing.T) {
	cases := []struct {
		names []string
		want  []float64
	}{
		{[]string{"Chapter 1", "Chapter 2", "Chapter 3"}, []float64{1, 2, 3}},
		{[]string{"14. Zam's Amporiam", "15. Trunkajunk"}, []float64{14, 15}},
		// unnumbered ones among mostly numbered ones are 0
		{[]string{"Chapter 1", "Misc.", "Chapter 2"}, []float64{1, 0, 2}},
		// a number given twice numbers by position
		{[]string{"1", "2", "1"}, []float64{1, 2, 3}},
		{[]string{"Book 1: Foo", "Book 2: Bar"}, []float64{1, 2}},
		{[]string{"Misc.", "Guest", "Chapter 3"}, []float64{1, 2, 3}},
	}
	for _, c := range cases {
		if got := wcNumbers(c.names); !slices.Equal(got, c.want) {
			t.Errorf("wcNumbers(%q) = %v, want %v", c.names, got, c.want)
		}
	}
}

func TestWcChapterLabel(t *testing.T) {
	cases := []struct {
		name   string
		number float64
		want   string
	}{
		{"Chapter 3: The Fall", 3, "Chapter 3 - The Fall"},
		{"Chapter 3", 3, "Chapter 3"},
		{"9: Counting Down", 9, "Chapter 9 - Counting Down"},
		{"Episode 672: This is Madness!", 672, "Chapter 672 - This is Madness!"},
		// a name whose number isn't the chapter's keeps it
		{"Chapter 3", 7, "Chapter 7 - Chapter 3"},
		{"Seeker of Thrones 2-10", 12, "Chapter 12 - Seeker of Thrones 2-10"},
		{"", 5, "Chapter 5"},
		{"Misc.", 0, "Misc."},
	}
	for _, c := range cases {
		ch := wcChapter(c.name, c.number)
		if got := ChapterLabel(&ch); got != c.want {
			t.Errorf("wcChapter(%q, %v) label = %q, want %q", c.name, c.number, got, c.want)
		}
	}
}

func TestWcComicImages(t *testing.T) {
	doc := docFromHTML(t, `<div id="comic">
		<img src="data:image/gif;base64,R0lG" data-src="/a.png">
		<img src="/cdn-cgi/image/width=1440,format=auto/media/b.png">
		<img src="/blank.gif" file="https://img.test/c.webp">
		<img src="/a.png">
		<img src="/pixel.png" width="0">
	</div>`)
	doc.Url, _ = url.Parse("https://comic.test/strip/1/")
	got := wcComicImages(doc, "#comic img")
	want := []string{"https://comic.test/a.png", "https://comic.test/media/b.png", "https://img.test/c.webp"}
	if !slices.Equal(got, want) {
		t.Errorf("wcComicImages = %q, want %q", got, want)
	}

	// a strip drawn in several languages keeps its default one
	doc = docFromHTML(t, `<div id="comic"><span class="default-lang"><img src="/en.png"></span><span class="es-lang"><img src="/es.png"></span></div>`)
	doc.Url, _ = url.Parse("https://comic.test/")
	if got := wcComicImages(doc, "#comic img"); !slices.Equal(got, []string{"https://comic.test/en.png"}) {
		t.Errorf("wcComicImages with languages = %q", got)
	}
}

func TestWcLargestImage(t *testing.T) {
	doc := docFromHTML(t, `<img src="/x-500x773.jpg" srcset="/x.jpg 800w, /x-194x300.jpg 194w, /x-768x1187.jpg 768w">`)
	base, _ := url.Parse("https://comic.test/")
	if got := wcLargestImage(doc.Find("img"), base); got != "https://comic.test/x.jpg" {
		t.Errorf("wcLargestImage = %q", got)
	}
	doc = docFromHTML(t, `<img src="/y.jpg">`)
	if got := wcLargestImage(doc.Find("img"), base); got != "https://comic.test/y.jpg" {
		t.Errorf("wcLargestImage without srcset = %q", got)
	}
}

func TestWpSiteName(t *testing.T) {
	cases := []struct{ head, want string }{
		{`<link rel="alternate" type="application/rss+xml" title="Kill Six Billion Demons &raquo; Feed" href="/feed/">
		  <link rel="alternate" type="application/rss+xml" title="Kill Six Billion Demons &raquo; Comments Feed" href="/comments/feed/">`, "Kill Six Billion Demons"},
		{`<link rel="alternate" type="application/rss+xml" title="Sandra and Woo RSS Feed" href="/feed/">`, "Sandra and Woo"},
		// an empty site name falls through to Open Graph, then the title
		{`<link rel="alternate" type="application/rss+xml" title=" &raquo; Feed" href="/feed"><meta property="og:site_name" content="Buttsmithy">`, "Buttsmithy"},
		{`<title>Archive | Grrl Power</title>`, "Grrl Power"},
	}
	for _, c := range cases {
		doc := docFromHTML(t, "<html><head>"+c.head+"</head><body></body></html>")
		if got := wpSiteName(doc); got != c.want {
			t.Errorf("wpSiteName(%s) = %q, want %q", c.head, got, c.want)
		}
	}
	if got := hostName("https://www.buttsmithy.com/archives/chapter/chapter-5"); got != "Buttsmithy" {
		t.Errorf("hostName = %q", got)
	}
}

func TestWcOldestFirst(t *testing.T) {
	list := []wcStrip{
		{URL: "https://s.test/2024/01/29/c/"},
		{URL: "https://s.test/2008/10/27/b/"},
		{URL: "https://s.test/2000/01/01/a/"},
	}
	got := wcOldestFirst(list)
	if got[0].URL != "https://s.test/2000/01/01/a/" || got[2].URL != "https://s.test/2024/01/29/c/" {
		t.Errorf("wcOldestFirst = %v", got)
	}
	undated := []wcStrip{{URL: "https://s.test/comic/2/"}, {URL: "https://s.test/comic/1/"}}
	if got := wcOldestFirst(undated); got[0].URL != "https://s.test/comic/2/" {
		t.Errorf("wcOldestFirst reordered undated links: %v", got)
	}
}

func TestNoReferrer(t *testing.T) {
	if !noReferrer(docFromHTML(t, `<html><head><meta name="referrer" content="no-referrer"></head></html>`)) {
		t.Error("noReferrer missed the meta tag")
	}
	if noReferrer(docFromHTML(t, `<html><head><meta name="referrer" content="origin"></head></html>`)) {
		t.Error("noReferrer took another policy")
	}
}
