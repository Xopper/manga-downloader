// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestPhotoGalleryTest(t *testing.T) {
	cases := map[string]bool{
		"https://www.xasiat.com/albums/17316/cosplay747/":                true,
		"https://www.xasiat.com/albums/":                                 false,
		"https://baobua.net/spot/aWhSTXN4VjFKUUU0SUthZnJjU3JvUT09.html":  true,
		"https://baobua.net/category/x":                                  false,
		"https://example.com/spot/aWhSTXN4VjFKUUU0SUthZnJjU3JvUT09.html": false,
	}
	for in, want := range cases {
		if got, _ := NewPhotoGallery(&Grabber{URL: in}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPhotoGalleryPage(t *testing.T) {
	pageURL := "https://baobua.net/spot/abc.html"
	p := NewPhotoGallery(&Grabber{URL: pageURL})
	p.Test()
	html := `<title>BaoBua.Net: Some album | Page 1/2</title>
		<div class="contentme">
			<a href="/img.html?url=a"><img src="https://blogger.googleusercontent.com/a.jpg"></a>
			<a href="/img.html?url=b"><img src="https://i0.wp.com/go.example.fun/s26/b.jpg?w=640"></a>
			<a href="/other"><img src="https://example.com/ad.jpg"></a>
		</div>
		<a class="page-numbers" href="?page=2">Next</a>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))

	if got := photoGalleryTitle(doc, p.site); got != "Some album" {
		t.Errorf("title = %q", got)
	}
	want := []string{"https://blogger.googleusercontent.com/a.jpg", "https://go.example.fun/s26/b.jpg"}
	if got := photoGalleryImages(doc, p.site, pageURL); !reflect.DeepEqual(got, want) {
		t.Errorf("images = %v, want %v", got, want)
	}
	if got := photoGalleryNext(doc, p.site, pageURL); got != pageURL+"?page=2" {
		t.Errorf("next = %q", got)
	}
}
