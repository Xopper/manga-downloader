// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// galleryServe serves the given pages by path, and 404 for the rest
func galleryServe(t *testing.T, pages map[string]func(base string) string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, page(srv.URL))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// galleryPageURLs identifies the gallery at pageURL and returns its title and
// its one chapter's page URLs
func galleryPageURLs(t *testing.T, pageURL string) (string, []string) {
	t.Helper()
	g := NewGallery(&Grabber{URL: pageURL, Settings: &Settings{}})
	ok, err := g.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want true", ok, err)
	}
	title, err := g.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle: %v", err)
	}
	chapters, errs := g.FetchChapters()
	if len(errs) > 0 || len(chapters) != 1 {
		t.Fatalf("FetchChapters = %d chapters, %v; want 1", len(chapters), errs)
	}
	chapter, err := g.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	var urls []string
	for _, p := range chapter.Pages {
		urls = append(urls, p.URL)
	}
	return title, urls
}

// TestGalleryAdults reads the page count and each page's file type from a
// GalleryAdults gallery (hentaifox, hentairox), jpg when g_th doesn't say
func TestGalleryAdults(t *testing.T) {
	srv := galleryServe(t, map[string]func(string) string{
		"/gallery/61886/": func(string) string {
			return `<html><body><div class="info"><h1>Inma to Keiyaku</h1></div>
<div class="gallery_thumb"><div class="g_thumb"><a href="/g/61886/1/"><img class="lazy preloader" src="data:image/svg+xml,x" data-src="https://i.example.com/002/1481608/1t.jpg" /></a></div></div>
<div class="gallery_thumb"><div class="g_thumb"><a href="/g/61886/2/"><img class="lazy preloader" src="data:image/svg+xml,x" data-src="https://i.example.com/002/1481608/2t.jpg" /></a></div></div>
<input type="hidden" name="load_dir" id="load_dir" value="002" />
<input type="hidden" name="load_id" id="load_id" value="1481608" />
<input type="hidden" name="load_pages" id="load_pages" value="4" />
<script>var g_th = $.parseJSON('{"1":"j,1280,1920","2":"w,1280,1920","3":"p,800,600"}');</script>
</body></html>`
		},
	})

	title, urls := galleryPageURLs(t, srv.URL+"/gallery/61886/")
	if title != "Inma to Keiyaku" {
		t.Errorf("title = %q", title)
	}
	want := []string{
		"https://i.example.com/002/1481608/1.jpg",
		"https://i.example.com/002/1481608/2.webp",
		"https://i.example.com/002/1481608/3.png",
		"https://i.example.com/002/1481608/4.jpg",
	}
	if !slices.Equal(urls, want) {
		t.Errorf("pages = %v, want %v", urls, want)
	}
}

// TestGalleryEnvy reads the pages from the first page's reader, which lists
// each page's file type
func TestGalleryEnvy(t *testing.T) {
	srv := galleryServe(t, map[string]func(string) string{
		"/gallery/1619599/": func(string) string {
			return `<html><body><h1 id="gallery-title">[Gosaiji] Amai-aimai</h1>
<div id="js-thumbs-grid" class="hnv-gallery-thumbs" data-gallery-id="1619599" data-total-pages="3" data-thumb-template="https://m{server}.example.com/{img_dir}/{gallery_id}/{page}t.jpg">
<div class="hnv-gallery-thumb"><a href="/g/1619599/1/"><img src="https://m11.example.com/033/co7r5wmbn2/1t.jpg"></a></div>
</div></body></html>`
		},
		"/g/1619599/1/": func(string) string {
			return `<html><body><div id="reader" data-reader-id="1619599" data-reader-image-base="https://m11.example.com/033/co7r5wmbn2" data-reader-total="3">
<img id="readerImg" src="https://m11.example.com/033/co7r5wmbn2/1.webp"></div>
<script type="application/json" id="readerPagesJson">[{"page":1,"ext":"webp","width":1280,"height":1816},{"page":2,"ext":"png","width":700,"height":1000},{"page":3,"width":700,"height":1000}]</script>
</body></html>`
		},
	})

	title, urls := galleryPageURLs(t, srv.URL+"/gallery/1619599/")
	if title != "[Gosaiji] Amai-aimai" {
		t.Errorf("title = %q", title)
	}
	want := []string{
		"https://m11.example.com/033/co7r5wmbn2/1.webp",
		"https://m11.example.com/033/co7r5wmbn2/2.png",
		"https://m11.example.com/033/co7r5wmbn2/3.webp",
	}
	if !slices.Equal(urls, want) {
		t.Errorf("pages = %v, want %v", urls, want)
	}
}

// TestGalleryNhentai turns nhentai-style thumbnails into full-size pages
func TestGalleryNhentai(t *testing.T) {
	srv := galleryServe(t, map[string]func(string) string{
		"/g/672222/": func(string) string {
			return `<html><body><div id="info"><h1>[Himitsu] En no Ori Ito [English]</h1><h2></h2></div>
<div class="container" id="thumbnail-container">
<div class="thumb-container"><a class="gallerythumb" href="/g/672222/1/"><div class="thumb"><img is="lazyload-image" class="lazyload" data-src="https://cdn.example.com/galleries/4103401/1t.webp" src="data:image/gif;base64,R0lG" /></div></a></div>
<div class="thumb-container"><a class="gallerythumb" href="/g/672222/2/"><div class="thumb"><img is="lazyload-image" class="lazyload" data-src="https://cdn.example.com/galleries/4103401/2t.jpg" src="data:image/gif;base64,R0lG" /></div></a></div>
</div></body></html>`
		},
	})

	title, urls := galleryPageURLs(t, srv.URL+"/g/672222/")
	if title != "[Himitsu] En no Ori Ito [English]" {
		t.Errorf("title = %q", title)
	}
	want := []string{
		"https://cdn.example.com/galleries/4103401/1.webp",
		"https://cdn.example.com/galleries/4103401/2.jpg",
	}
	if !slices.Equal(urls, want) {
		t.Errorf("pages = %v, want %v", urls, want)
	}
}

// TestGalleryHentaiHand asks the comic's images API, in page order
func TestGalleryHentaiHand(t *testing.T) {
	srv := galleryServe(t, map[string]func(string) string{
		"/en/comic/yanmama-english": func(string) string {
			return `<html><body><h1>[Hari Poteto] Yanmama [English]</h1>
<script>window.app = { reader: null, comic: {"id":273963,"linkcode":"288813","title":"[Hari Poteto] Yanmama [English]","pages":2} };</script>
</body></html>`
		},
		"/api/comics/yanmama-english/images": func(string) string {
			return `{"comic":{"id":273963},"images":[
{"id":2,"page":2,"source_url":"https://cdn.example.com/images/273963/2.png"},
{"id":1,"page":1,"source_url":"https://cdn.example.com/images/273963/1.jpg"}]}`
		},
	})

	title, urls := galleryPageURLs(t, srv.URL+"/en/comic/yanmama-english")
	if title != "[Hari Poteto] Yanmama [English]" {
		t.Errorf("title = %q", title)
	}
	want := []string{
		"https://cdn.example.com/images/273963/1.jpg",
		"https://cdn.example.com/images/273963/2.png",
	}
	if !slices.Equal(urls, want) {
		t.Errorf("pages = %v, want %v", urls, want)
	}
}

// TestGalleryDeclinesOtherPages leaves a page of no known gallery script to
// the next grabber
func TestGalleryDeclinesOtherPages(t *testing.T) {
	srv := galleryServe(t, map[string]func(string) string{
		"/manga/foo/": func(string) string {
			return `<html><body><h1>Foo</h1><ul class="chapters"><li><a href="/manga/foo/1">Chapter 1</a></li></ul></body></html>`
		},
	})

	g := NewGallery(&Grabber{URL: srv.URL + "/manga/foo/", Settings: &Settings{}})
	if ok, err := g.Test(); ok || err != nil {
		t.Errorf("Test() = %v, %v; want false, nil", ok, err)
	}
}
