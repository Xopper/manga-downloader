// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func webcomicDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestWebcomicListURL maps any link of each site to its archive
func TestWebcomicListURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://www.collectedcurios.com/sequentialart.php", "https://www.collectedcurios.com/sequentialart.php"},
		{"https://collectedcurios.com/BattleBunnies.php?s=12", "https://www.collectedcurios.com/battlebunnies.php"},
		{"https://www.collectedcurios.com/index.html", ""},
		{"https://www.darklegacycomics.com/900", "https://www.darklegacycomics.com/archive"},
		{"https://www.sabrina-online.com/2015-01.html", "https://www.sabrina-online.com/archive.html"},
		{"https://www.blastwave-comic.com/index.php?p=comic&nro=89", "https://www.blastwave-comic.com/index.php"},
		{"https://smbc-comics.com/comic/hun", "https://www.smbc-comics.com/comic/archive"},
		{"https://politeandgood.com/comic/12", "https://politeandgood.com/comic/archive"},
	}
	for _, c := range cases {
		w := NewWebcomic(&Grabber{URL: c.in, Settings: &Settings{}})
		if ok, _ := w.Test(); !ok {
			t.Errorf("Test(%q) = false", c.in)
			continue
		}
		u, _ := url.Parse(c.in)
		got, err := w.spec.listURL(u)
		if (c.want == "") != (err != nil) || got != c.want {
			t.Errorf("listURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	if ok, _ := NewWebcomic(&Grabber{URL: "https://example.com/comic/archive", Settings: &Settings{}}).Test(); ok {
		t.Error("Test matched another domain")
	}
}

// webcomicCase is an archive and a strip page of one spec, and what they give
type webcomicCase struct {
	spec          *webcomicSpec
	list, archive string
	wantN         int
	first         webcomicEntry
	strip         string
	images        []string
}

// TestWebcomicSpecs parses a trimmed archive and strip page of every site
func TestWebcomicSpecs(t *testing.T) {
	cases := map[string]webcomicCase{
		"collectedcurios": {
			spec: webcomicCollectedCurios, list: "https://www.collectedcurios.com/spiderandscorpion.php",
			archive: `<img id="strip" src="SAS_0100_Small.jpg"><a href="/spiderandscorpion.php?s=99"><img src="Nav_BackOne.gif" title="Back one"></a>`,
			wantN:   100, first: webcomicEntry{Number: 1, URL: "https://www.collectedcurios.com/spiderandscorpion.php?s=1"},
			strip: `<img class="w3-image" src="SA_0005_small.jpg"><img src="Nav_First.gif">`, images: []string{"SA_0005_small.jpg"},
		},
		"darklegacy": {
			spec: webcomicDarkLegacy, list: "https://www.darklegacycomics.com/archive",
			archive: `<div class="ascending"><a href='1'><div class='archive_link'><span class='index'>1</span><span class='name'>I'm a good healer! </span></div></a></div>
<div class="descending"><a href='1'><div class='archive_link'><span class='index'>1</span><span class='name'>I'm a good healer! </span></div></a></div>`,
			wantN: 1, first: webcomicEntry{Number: 1, Title: "I'm a good healer! ", URL: "https://www.darklegacycomics.com/1", Prefixed: true},
			strip: `<div class="navigation"><img src="images/nav_1.png"></div><div id="gallery" class="comic narrow"><img src="comics/900.jpg" /></div>`, images: []string{"comics/900.jpg"},
		},
		"sabrina": {
			spec: webcomicSabrina, list: "https://www.sabrina-online.com/archive.html",
			archive: `<A HREF="index.html">home</A><H3><A HREF="1996-01.html">All</A></H3><H3><A HREF="1997-01.html">1st quarter </A></H3>`,
			wantN:   2, first: webcomicEntry{Number: 1, Title: "1996 All", URL: "https://www.sabrina-online.com/1996-01.html"},
			strip:  `<IMG SRC="sabtitle2.gif"><center><A HREF="strips/SabOnline10.GIF"><IMG SRC="strips/SO_sm10.GIF"></A><IMG SRC="pages/p1.gif"></center>`,
			images: []string{"strips/SabOnline10.GIF", "pages/p1.gif"},
		},
		"blastwave": {
			spec: webcomicBlastwave, list: "https://www.blastwave-comic.com/index.php",
			archive: `<select name="nro"><option value="">Jump to a strip...</option><option value="93">#81 Knife Fight</option><option value="89">GWTB Legends - Light Magic</option></select>`,
			wantN:   2, first: webcomicEntry{Number: 93, Title: "#81 Knife Fight", URL: "https://www.blastwave-comic.com/index.php?p=comic&nro=93"},
			strip: `<img src="images/page/default/first.jpg"><img src="comics/legends_002.jpg" alt="x">`, images: []string{"comics/legends_002.jpg"},
		},
		"smbc": {
			spec: webcomicSMBC, list: "https://www.smbc-comics.com/comic/archive",
			archive: `<select><option value="">Select a comic...</option><option value="comic/2002-09-05">September 5, 2002 - 2002-09-05</option><option value="comic/hun">October 16, 2026 - Hun</option></select>`,
			wantN:   2, first: webcomicEntry{Number: 1, Title: "2002-09-05", URL: "https://www.smbc-comics.com/comic/2002-09-05"},
			strip:  `<img title="x" src="https://www.smbc-comics.com/comics/20020905-2.gif" id="cc-comic" /><div id="aftercomic"><img src='https://www.smbc-comics.com/comics/after.png'></div>`,
			images: []string{"https://www.smbc-comics.com/comics/20020905-2.gif", "https://www.smbc-comics.com/comics/after.png"},
		},
		"broccolisoup": {
			spec: webcomicBroccoliSoup, list: "https://politeandgood.com/comic/archive",
			archive: `<ul><li class="archive-page"><a href="/comic/1"><span class="page-title">Hello </span></a></li><li class="archive-page"><a href="/comic-characters">Characters</a></li></ul>`,
			wantN:   1, first: webcomicEntry{Number: 1, Title: "Hello ", URL: "https://politeandgood.com/comic/1", Prefixed: true},
			strip:  `<figure id="comic"><ol><li><img src="/assets/images/comics/1/a.jpg"></li><li><img src="/assets/images/comics/1/b.jpg"></li></ol></figure>`,
			images: []string{"/assets/images/comics/1/a.jpg", "/assets/images/comics/1/b.jpg"},
		},
	}
	for name, c := range cases {
		base, _ := url.Parse(c.list)
		entries := c.spec.entries(webcomicDoc(t, c.archive), base)
		if len(entries) != c.wantN {
			t.Errorf("%s: got %d entries, want %d", name, len(entries), c.wantN)
			continue
		}
		if entries[0] != c.first {
			t.Errorf("%s: first entry = %+v, want %+v", name, entries[0], c.first)
		}
		images := c.spec.images(webcomicDoc(t, c.strip))
		if strings.Join(images, " ") != strings.Join(c.images, " ") {
			t.Errorf("%s: images = %q, want %q", name, images, c.images)
		}
	}
}

// TestWebcomicPages resolves relative images and drops repeats
func TestWebcomicPages(t *testing.T) {
	pages := webcomicPages([]string{"comics/900.jpg", "", "/comics/900.jpg", "data:image/gif;base64,R0"}, "https://www.darklegacycomics.com/900")
	if len(pages) != 1 || pages[0].URL != "https://www.darklegacycomics.com/comics/900.jpg" {
		t.Errorf("pages = %+v", pages)
	}
}

// TestWebcomicGetAnyStatus reads a page sent with a 500, but not a 404
func TestWebcomicGetAnyStatus(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`<option value="comic/hun">October 16, 2026 - Hun</option>`))
	}))
	defer srv.Close()
	doc, err := webcomicGetAnyStatus(srv.URL + "/comic/archive")
	if err != nil || doc.Find("option").Length() != 1 {
		t.Errorf("archive = %v, %v", doc, err)
	}
	if _, err := webcomicGetAnyStatus(srv.URL + "/missing"); err == nil {
		t.Error("want an error for a 404")
	}
}
