// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func novelcoolDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestNovelcoolTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.novelcool.com/novel/I-m-The-Chief-Of-The-Harem.html", true},
		{"https://es.novelcool.com/novel/Solo-Leveling.html", true},
		{"https://www.novelcool.com/chapter/Chapter-100/8476753/", true},
		{"https://www.novelcool.com/category/latest.html", false},
		{"https://example.com/novel/x.html", false},
	}
	for _, tt := range tests {
		got, _ := NewNovelcool(&Grabber{URL: tt.url}).Test()
		if got != tt.want {
			t.Errorf("Test(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestNovelcoolLanguage(t *testing.T) {
	tests := map[string]string{
		"https://www.novelcool.com/novel/x.html": "en",
		"https://es.novelcool.com/novel/x.html":  "es",
		"https://br.novelcool.com/novel/x.html":  "pt-br",
	}
	for u, want := range tests {
		if got := novelcoolLanguage(u); got != want {
			t.Errorf("novelcoolLanguage(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestNovelcoolChapters(t *testing.T) {
	doc := novelcoolDoc(t, `<div class="chapter-item-list">
<div class="chp-item"><a href="https://www.novelcool.com/chapter/Chapter-112/8550525/" title="Chapter 112">
<div class="chapter-item"><div class="chapter-item-title"><span class="chapter-item-headtitle"> Chapter 112</span></div></div></a></div>
<div class="chp-item"><a href="https://www.novelcool.com/chapter/Chapter-10-5/8448277/" title="Chapter 10.5">
<div class="chapter-item"><div class="chapter-item-title"><span class="chapter-item-headtitle"> Chapter 10.5: The Bath</span></div></div></a></div>
</div>`)

	chapters, errs := novelcoolChapters(doc, "https://www.novelcool.com/novel/I-m-The-Chief-Of-The-Harem.html")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	first := chapters[0].(*NovelcoolChapter)
	if first.Number != 112 || first.ID != "8550525" || first.Title != "" || first.Language != "en" {
		t.Errorf("first chapter = %+v", first.Chapter)
	}
	if got := ChapterLabel(first); got != "Chapter 112" {
		t.Errorf("label = %q", got)
	}
	second := chapters[1].(*NovelcoolChapter)
	if second.Number != 10.5 || second.Title != "The Bath" || second.URL != "https://www.novelcool.com/chapter/Chapter-10-5/8448277/" {
		t.Errorf("second chapter = %+v", second)
	}
}

func TestNovelcoolChaptersCloaked(t *testing.T) {
	doc := novelcoolDoc(t, `<div class="chapter-item-list"><div class="chp-item">
<a href="https://explorefunctionality.com/go/rds/?data=abc" title="Chapter 112"><span class="chapter-item-headtitle">Chapter 112</span></a>
</div></div>`)

	chapters, errs := novelcoolChapters(doc, "https://www.novelcool.com/novel/x.html")
	if len(chapters) != 0 || len(errs) != 1 {
		t.Fatalf("got %d chapters and errors %v, want none and one error", len(chapters), errs)
	}
}

func TestNovelcoolReaderPage(t *testing.T) {
	doc := novelcoolDoc(t, `<div class="mangaread-pagenav"><select class="sl-page">
<option value="https://www.novelcool.com/chapter/Chapter-100/8476753-1.html" selected>1/3</option>
<option value="https://www.novelcool.com/chapter/Chapter-100/8476753-2.html">2/3</option>
<option value="https://www.novelcool.com/chapter/Chapter-100/8476753-3.html">3/3</option>
</select></div>
<img class="mangaread-manga-pic manga_pic" src="https://en5.movietop.cc/comics/pic6/1.webp?acc=x&exp=1"/>
<div class="mangaread-pagenav"><select class="sl-page"><option value="https://www.novelcool.com/chapter/Chapter-100/8476753-1.html">1/3</option></select></div>`)

	pages := novelcoolPageURLs(doc, "https://www.novelcool.com/chapter/Chapter-100/8476753/")
	if len(pages) != 3 || pages[2] != "https://www.novelcool.com/chapter/Chapter-100/8476753-3.html" {
		t.Errorf("pages = %v", pages)
	}
	if img := novelcoolPageImage(doc); img != "https://en5.movietop.cc/comics/pic6/1.webp?acc=x&exp=1" {
		t.Errorf("image = %q", img)
	}
}

func TestNovelcoolResolveSeriesURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/chapter/Chapter-100/8476753/", func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); strings.Contains(ua, "Mozilla") {
			t.Errorf("sent a browser User-Agent %q", ua)
		}
		_, _ = w.Write([]byte(`<div class="chapter-top-nav-left"><a href="/novel/I-m-The-Chief-Of-The-Harem.html">I'm The Chief Of The Harem</a></div>`))
	})
	srv := serve(t, mux)

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"series URL is kept", srv.URL + "/novel/I-m-The-Chief-Of-The-Harem.html", srv.URL + "/novel/I-m-The-Chief-Of-The-Harem.html"},
		{"chapter URL goes to its series", srv.URL + "/chapter/Chapter-100/8476753/", srv.URL + "/novel/I-m-The-Chief-Of-The-Harem.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNovelcool(&Grabber{URL: tt.url, Settings: &Settings{}})
			if err := n.resolveSeriesURL(); err != nil {
				t.Fatal(err)
			}
			if n.URL != tt.want {
				t.Errorf("URL = %q, want %q", n.URL, tt.want)
			}
		})
	}
}
