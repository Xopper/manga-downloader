// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"strings"
	"testing"
)

func TestHentaiGalleryTest(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{"https://imhentai.xxx/gallery/1747833/", "https://imhentai.xxx/gallery/1747833/"},
		{"https://imhentai.xxx/view/1747833/3/", "https://imhentai.xxx/gallery/1747833/"},
		{"https://hentaiera.com/view/1636123/2/", "https://hentaiera.com/gallery/1636123/"},
		{"https://hentaizap.com/g/1657062/4/", "https://hentaizap.com/gallery/1657062/"},
		{"https://nhentai.xxx/g/699682/12/", "https://nhentai.xxx/g/699682/"},
		{"https://asmhentai.com/gallery/672825/5/", "https://asmhentai.com/g/672825/"},
		{"https://imhentai.xxx/tag/big-breasts/", ""},
		{"https://example.com/gallery/1747833/", ""},
	}
	for _, c := range cases {
		h := NewHentaiGallery(&Grabber{URL: c.url})
		ok, _ := h.Test()
		if ok != (c.want != "") || (ok && h.URL != c.want) {
			t.Errorf("Test(%q) = %v, URL %q; want %q", c.url, ok, h.URL, c.want)
		}
	}
}

// The gallery page of imhentai, hentaiera and nhentai.xxx: the thumbnails
// give the folder and g_th each page's type
func TestHentaiGalleryFromThumbs(t *testing.T) {
	cases := []struct {
		name, th string
		want     []string
	}{
		{"flat", `{"1":"w,1280,2018","2":"j,1280,1763","3":"p,1280,1763"}`, []string{"1.webp", "2.jpg", "3.png"}},
		{"nested", `{"fl":{"1":"w,1085,1500","2":"g,1024,1536","3":"j,1085,1500"},"th":{}}`, []string{"1.webp", "2.gif", "3.jpg"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			html := `<html><body><h1>[Giga] Vivi x Celestial Dragon</h1>
<input type="hidden" name="load_pages" id="load_pages" value="3" />
<div class="gthumb"><a href="/view/1747833/1/"><img class="lazy" src="data:image/svg+xml,%3Csvg%3E" data-src="https://m11.imhentai.xxx/033/rwi2asxt75/1t.jpg"></a></div>
<div class="gthumb"><a href="/view/1747833/2/"><img class="lazy" src="data:image/svg+xml,%3Csvg%3E" data-src="https://m11.imhentai.xxx/033/rwi2asxt75/2t.jpg"></a></div>
<script>var g_th = $.parseJSON('` + c.th + `');</script></body></html>`
			h := NewHentaiGallery(&Grabber{URL: "https://imhentai.xxx/gallery/1747833/"})
			h.Test()
			h.doc = docFromHTML(t, html)
			chapter, err := h.FetchChapter(&HentaiGalleryChapter{Chapter: Chapter{Number: 1}})
			if err != nil {
				t.Fatal(err)
			}
			if len(chapter.Pages) != len(c.want) {
				t.Fatalf("got %v", pageURLs(chapter))
			}
			for i, want := range c.want {
				if got := chapter.Pages[i].URL; got != "https://m11.imhentai.xxx/033/rwi2asxt75/"+want {
					t.Errorf("page %d = %q, want …/%s", i+1, got, want)
				}
			}
		})
	}
}

// hentaizap states the page types only in its reader's JSON
func TestHentaiGalleryHentaizap(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/g/1657062/1/", `<html><body><div data-reader-page="1" data-reader-total="2" data-reader-image-base="https://m11.hentaizap.com/033/lif5rxk3t8"></div>
<script type="application/json" id="readerPagesJson">[{"page":1,"ext":"webp","width":1280,"height":1829},{"page":2,"ext":"jpg","width":1280,"height":1829}]</script></body></html>`)
	srv := serve(t, mux)

	h := NewHentaiGallery(&Grabber{URL: srv.URL + "/gallery/1657062/"})
	h.host, h.id = hentaiGalleryHosts["hentaizap.com"], "1657062"
	h.doc = docFromHTML(t, `<html><body><h1>[Girin] Miss Heed (Villanos)</h1>
<div id="js-thumbs-grid" class="hz-gallery-thumbs" data-gallery-id="1657062" data-total-pages="2" data-thumb-template="https://m{server}.hentaizap.com/{img_dir}/{gallery_id}/{page}t.jpg"></div></body></html>`)
	chapter, err := h.FetchChapter(&HentaiGalleryChapter{Chapter: Chapter{Number: 1}})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(pageURLs(chapter), " ")
	if want := "https://m11.hentaizap.com/033/lif5rxk3t8/1.webp https://m11.hentaizap.com/033/lif5rxk3t8/2.jpg"; got != want {
		t.Errorf("pages = %s, want %s", got, want)
	}
}

// asmhentai states neither, so each reader page is read
func TestHentaiGalleryAsmhentai(t *testing.T) {
	mux := http.NewServeMux()
	for _, n := range []string{"1", "2"} {
		respond(mux, "/gallery/672825/"+n+"/", `<html><body><img id="fimg" class="lazy image_`+n+`" src="data:image/svg+xml,%3Csvg%3E" data-src="https://images.asmhentai.com/018/672825/`+n+`.jpg"></body></html>`)
	}
	srv := serve(t, mux)

	h := NewHentaiGallery(&Grabber{URL: srv.URL + "/g/672825/"})
	h.host, h.id = hentaiGalleryHosts["asmhentai.com"], "672825"
	h.doc = docFromHTML(t, `<html><body><h1>[Hiiragi Popura] Inokori seibutsu-shitsu</h1><input type="hidden" id="t_pages" value="2" /></body></html>`)
	chapter, err := h.FetchChapter(&HentaiGalleryChapter{Chapter: Chapter{Number: 1}})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(pageURLs(chapter), " ")
	if want := "https://images.asmhentai.com/018/672825/1.jpg https://images.asmhentai.com/018/672825/2.jpg"; got != want {
		t.Errorf("pages = %s, want %s", got, want)
	}
}

func TestMyHentai(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://myhentaigallery.com/a/33822", "https://myhentaigallery.com/a/33822"},
		{"https://myhentaigallery.com/a/33822/4", "https://myhentaigallery.com/a/33822"},
		{"https://myhentaicomics.com/gallery/show/63379/2", "https://myhentaicomics.com/gallery/thumbnails/63379"},
		{"https://myhentaigallery.com/a/category/12", ""},
	}
	for _, c := range cases {
		m := NewMyHentai(&Grabber{URL: c.url})
		ok, _ := m.Test()
		if ok != (c.want != "") || (ok && m.URL != c.want) {
			t.Errorf("Test(%q) = %v, URL %q; want %q", c.url, ok, m.URL, c.want)
		}
	}

	page := func(count string) string {
		return `<html><body><h1>Good Morning Marco</h1><div> Pages: ` + count + ` </div>
<ul class="comics-grid clear">
<li class="item"><div class="comic-inner"><a href="/a/33822/1"><div class="comic-thumb"><img src="https://cdn.myhentaicomics.com/mhc/images/Good Morning Marco/thumbnail/001.jpg?22"></div></a></div></li>
<li class="item"><div class="comic-inner"><a href="/a/33822/2"><div class="comic-thumb"><img src="https://cdn.myhentaicomics.com/mhc/images/Good Morning Marco/thumbnail/002.jpg?22"></div></a></div></li>
</ul></body></html>`
	}
	m := NewMyHentai(&Grabber{URL: "https://myhentaigallery.com/a/33822"})
	m.Test()
	m.doc = docFromHTML(t, page("2"))
	chapter, err := m.FetchChapter(&MyHentaiChapter{Chapter: Chapter{Number: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(pageURLs(chapter), " "), "https://cdn.myhentaicomics.com/mhc/images/Good%20Morning%20Marco/original/001.jpg?22 https://cdn.myhentaicomics.com/mhc/images/Good%20Morning%20Marco/original/002.jpg?22"; got != want {
		t.Errorf("pages = %s, want %s", got, want)
	}

	m.doc = docFromHTML(t, page("3"))
	if _, err := m.FetchChapter(&MyHentaiChapter{Chapter: Chapter{Number: 1}}); err == nil {
		t.Error("FetchChapter() took a gallery listing fewer pages than it states")
	}
}

func TestPlatformHosts(t *testing.T) {
	cases := []struct {
		url  string
		site Site
		want bool
	}{
		{"https://divascans.org/series/comic/some-series", NewRSCComicHosts(&Grabber{}), true},
		{"https://valirscans.org/series/comic/some-series/chapter/12", NewRSCComicHosts(&Grabber{}), true},
		{"https://example.org/divascans.org/", NewRSCComicHosts(&Grabber{}), false},
		{"https://magustoon.org/series/time-healer-ceres", NewTanstackHosts(&Grabber{}), true},
		{"https://magustoon.org.example.com/series/x", NewTanstackHosts(&Grabber{}), false},
	}
	for _, c := range cases {
		switch s := c.site.(type) {
		case *RSCComicHosts:
			s.URL = c.url
		case *TanstackHosts:
			s.URL = c.url
		}
		if got, _ := c.site.Test(); got != c.want {
			t.Errorf("%T.Test(%q) = %v, want %v", c.site, c.url, got, c.want)
		}
	}
}

func TestTcbSeriesPaths(t *testing.T) {
	cases := map[string]string{
		"https://mangapark.tv/manhwa/mr-liar/":                       "manhwa/mr-liar",
		"https://mangaowl.io/read-1/im-only-a-stepmother/chapter-3/": "read-1/im-only-a-stepmother",
		"https://mangagg.com/comic/i-tamed-the-duke/":                "comic/i-tamed-the-duke",
		"https://gedecomix.com/porncomic/fucking-mom/":               "porncomic/fucking-mom",
		"https://hentai4free.net/hentai/some-title/":                 "hentai/some-title",
		// any /<type>/<slug> is a candidate: Test() probes ajax/chapters
		"https://example.com/comics/some-title/": "comics/some-title",
	}
	for u, want := range cases {
		got := ""
		if m := tcbSeriesRe.FindStringSubmatch(u); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("tcbSeriesRe(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestMangahubMirrors(t *testing.T) {
	for _, host := range []string{"mangahub.io", "mangafox.fun", "1manga.co", "manganel.me"} {
		s, ok := matchBrowserSelector(host)
		if !ok || s.ImageWait != "img.PB0mN" {
			t.Errorf("matchBrowserSelector(%q) = %v; want the mangahub entry", host, ok)
		}
	}
}
