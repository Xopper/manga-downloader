// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSingleGalleryTest(t *testing.T) {
	for _, c := range []struct {
		url  string
		want bool
	}{
		{"https://www.metarthunter.com/some-set-75962/", true},
		{"https://foamgirl.net/1838407.html", true},
		{"https://bestporncomix.com/gallery/lucky-7/", true},
		{"https://example.com/gallery/lucky-7/", false},
	} {
		s := NewSingleGallery(&Grabber{URL: c.url})
		if got, _ := s.Test(); got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

// A gallery split over numbered pages whose pager skips some (1 2 … 4) and
// links another gallery: pages come in order, the other gallery is left out
func TestSingleGalleryPagination(t *testing.T) {
	page := func(n string, pics ...string) string {
		html := `<html><body><h1>Ina Enohara – Young Animal<span>(5P)</span></h1>`
		for _, p := range pics {
			html += `<a class="imageclick-imgbox" href="https://cdn.example.com/` + p + `.webp"><img src="https://cdn.example.com/` + p + `.webp"></a>`
		}
		html += `<a class="page-numbers next" href="/1838408.html">another set</a>`
		switch n {
		case "1":
			html += `<a class="page-numbers" href="/1838407_2.html">2</a><a class="page-numbers" href="/1838407_4.html">4</a>`
		case "2":
			html += `<a class="page-numbers" href="/1838407_3.html">3</a>`
		}
		return html + `</body></html>`
	}
	mux := http.NewServeMux()
	respond(mux, "/1838407.html", page("1", "a", "b"))
	respond(mux, "/1838407_2.html", page("2", "c"))
	respond(mux, "/1838407_3.html", page("3", "d"))
	respond(mux, "/1838407_4.html", page("4", "e"))
	respond(mux, "/1838408.html", page("x", "other"))
	srv := serve(t, mux)

	s := NewSingleGallery(&Grabber{URL: srv.URL + "/1838407.html"})
	for _, site := range singleGallerySites {
		if site.Domains[0] == "foamgirl.net" {
			s.site = site
		}
	}
	if title, _ := s.FetchTitle(); title != "Ina Enohara – Young Animal" {
		t.Errorf("FetchTitle() = %q", title)
	}
	chapters, _ := s.FetchChapters()
	chapter, err := s.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(pageURLs(chapter), " ")
	want := "https://cdn.example.com/a.webp https://cdn.example.com/b.webp https://cdn.example.com/c.webp https://cdn.example.com/d.webp https://cdn.example.com/e.webp"
	if got != want {
		t.Errorf("pages = %s\nwant %s", got, want)
	}
}

func TestSingleGalleryPictures(t *testing.T) {
	cases := []struct {
		domain, html string
		want         []string
	}{
		{
			"onlythebesthentai.com",
			`<figure class="wp-block-gallery"><figure><a href="/?attachment_id=1940"><img src="https://onlythebesthentai.com/wp-content/uploads/2025/12/2025-12-12-11.17.43-739x1024.jpg.webp"></a></figure></figure>`,
			[]string{"https://onlythebesthentai.com/wp-content/uploads/2025/12/2025-12-12-11.17.43.jpg"},
		},
		{
			"eggporncomics.com",
			`<div class="preview"><a href="/comics/241550/guilty-gear-strive?page=1&per-page=1"><div class="image"><picture><img alt="Guilty Gear Strive - Page 1" src="/images/postImg/241550/thumb300_6464293.webp"></picture></div></a></div>
<a href="/comics/302/pussy-lessons"><img alt="Pussy Lessons" src="/images/postImg/302/thumb300_5970.jpg"></a>`,
			[]string{"https://eggporncomics.com/images/postImg/241550/6464293.webp"},
		},
		{
			"hentalk.pw",
			`<img src="/image/99851e580b915ec3/1?type=thumb"><script>const data = {hash:"99851e580b915ec3",pages:3,title:"x"};</script>`,
			[]string{"https://hentalk.pw/image/99851e580b915ec3/1", "https://hentalk.pw/image/99851e580b915ec3/2", "https://hentalk.pw/image/99851e580b915ec3/3"},
		},
	}
	for _, c := range cases {
		s := NewSingleGallery(&Grabber{URL: "https://" + c.domain + "/gallery/"})
		if ok, _ := s.Test(); !ok {
			t.Fatalf("Test() declined %s", c.domain)
		}
		base := mustParseURL(t, s.URL)
		got := strings.Join(s.pictures(docFromHTML(t, "<html><body>"+c.html+"</body></html>"), base), " ")
		if want := strings.Join(c.want, " "); got != want {
			t.Errorf("%s: pictures = %s, want %s", c.domain, got, want)
		}
	}
}

func TestSingleGalleryPageIndex(t *testing.T) {
	for u, want := range map[string]int{
		"https://foamgirl.net/1838407.html":                1,
		"https://foamgirl.net/1838407_3.html":              3,
		"https://www.4khd.com/content/02/some-post.html/5": 5,
		"https://xiutaku.com/18729?page=4":                 4,
		"https://buondua.com/some-album-56974":             1,
		"https://buondua.com/some-album-56974?page=2":      2,
		"https://rokuhentai.com/6w0vec/0":                  0,
	} {
		stem := strings.TrimSuffix(mustParseURL(t, u).Path, ".html")
		for _, first := range []string{"/1838407", "/content/02/some-post", "/18729", "/some-album-56974", "/6w0vec"} {
			if strings.HasPrefix(stem, first) {
				stem = first
			}
		}
		if got := singleGalleryPageIndex(u, stem); got != want {
			t.Errorf("singleGalleryPageIndex(%q, %q) = %d, want %d", u, stem, got, want)
		}
	}
}

func TestNHentaiCom(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/api/comics/guardian-nikola-chapter-5/images", `{"comic":{"id":702353,"title":"Guardian Nikola [Kinkymation] - Chapter 5","slug":"guardian-nikola-chapter-5","pages":2},"chapter":null,"next_chapter":null,"images":[{"id":1,"page":1,"source_url":"https://cdn.nhentai.com/nhentai/storage/images/702353/1.webp","thumbnail_url":"https://cdn.nhentai.com/nhentai/storage/thumbnails/702353/1.webp"},{"id":2,"page":2,"source_url":"https://cdn.nhentai.com/nhentai/storage/images/702353/2.webp","thumbnail_url":"https://cdn.nhentai.com/nhentai/storage/thumbnails/702353/2.webp"}]}`)
	srv := serve(t, mux)

	n := NewNHentaiCom(&Grabber{URL: "https://nhentai.com/en/comic/guardian-nikola-chapter-5/reader"})
	if ok, _ := n.Test(); !ok || n.slug != "guardian-nikola-chapter-5" {
		t.Fatalf("Test() = %v, slug %q", ok, n.slug)
	}
	n.URL = srv.URL + "/en/comic/guardian-nikola-chapter-5"
	if title, err := n.FetchTitle(); err != nil || title != "Guardian Nikola [Kinkymation] - Chapter 5" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}
	chapters, _ := n.FetchChapters()
	chapter, err := n.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(pageURLs(chapter), " "), "https://cdn.nhentai.com/nhentai/storage/images/702353/1.webp https://cdn.nhentai.com/nhentai/storage/images/702353/2.webp"; got != want {
		t.Errorf("pages = %s, want %s", got, want)
	}
}

func TestSimplyHentai(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/9-original-work/hyoui-lover", `<html><body><h1>hyoui lover</h1>
<a href="/9-original-work/hyoui-lover/page/13724330"><img src="https://images.sh-cdn.com/2/a/4/e/365236/thumb_6c801cc0.jpg"></a></body></html>`)
	// the payload lists the pages out of order, and some twice
	respond(mux, "/9-original-work/hyoui-lover/page/13724330", `<html><body><script>self.__next_f.push([1,"{\"id\":13724345,\"pageNum\":2,\"sizes\":{\"full\":\"https://images.sh-cdn.com/2/a/4/e/365236/27cf7645.jpg\",\"thumb\":\"x\"}},{\"id\":13724330,\"pageNum\":1,\"sizes\":{\"full\":\"https://images.sh-cdn.com/2/a/4/e/365236/6c801cc0.jpg\",\"thumb\":\"x\"}},{\"id\":13724345,\"pageNum\":2,\"sizes\":{\"full\":\"https://images.sh-cdn.com/2/a/4/e/365236/27cf7645.jpg\",\"thumb\":\"x\"}}"])</script></body></html>`)
	srv := serve(t, mux)

	s := NewSimplyHentai(&Grabber{URL: "https://www.simply-hentai.com/9-original-work/hyoui-lover/page/13724330"})
	if ok, _ := s.Test(); !ok || s.URL != "https://www.simply-hentai.com/9-original-work/hyoui-lover" {
		t.Fatalf("Test() = %v, URL %q", ok, s.URL)
	}
	s.URL = srv.URL + "/9-original-work/hyoui-lover"
	chapters, _ := s.FetchChapters()
	chapter, err := s.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(pageURLs(chapter), " "), "https://images.sh-cdn.com/2/a/4/e/365236/6c801cc0.jpg https://images.sh-cdn.com/2/a/4/e/365236/27cf7645.jpg"; got != want {
		t.Errorf("pages = %s, want %s", got, want)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
