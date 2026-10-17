// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"testing"
)

// threeHentaiGalleryHTML is a trimmed gallery page: every thumbnail sits next
// to its full picture, plus a <noscript> copy of it
const threeHentaiGalleryHTML = `<html><body>
<h1 class="text-left font-weight-bold">[Isamu] Jikangai Kinmu</h1>
<a class="main-cover" href="https://3hentai.net/d/52321/1"><img class="lazy" data-src="https://s9.3hentai.net/d52321/cover.jpg"/></a>
<div class="large-container bg-container container-xl" id="thumbnail-gallery">
<div class="single-thumb-col"><div class="single-thumb"><a href="https://3hentai.net/d/52321/1" rel="nofollow">
<img class="lazy small-bg-load" data-src="https://s9.3hentai.net/d52321/1t.jpg" width="200" height="276" />
<noscript><img src="https://s9.3hentai.net/d52321/1t.jpg" width="200" height="276"/></noscript></a></div></div>
<div class="single-thumb-col"><div class="single-thumb"><a href="https://3hentai.net/d/52321/2" rel="nofollow">
<img class="lazy small-bg-load" data-src="https://s9.3hentai.net/d52321/2t.png" width="200" height="87" /></a></div></div>
</div></body></html>`

func TestThreeHentaiPages(t *testing.T) {
	got := threeHentaiPages(docFromHTML(t, threeHentaiGalleryHTML))
	want := []string{"https://s9.3hentai.net/d52321/1.jpg", "https://s9.3hentai.net/d52321/2.png"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("threeHentaiPages() = %v, want %v", got, want)
	}
}

func TestThreeHentaiTest(t *testing.T) {
	for in, want := range map[string]string{
		"https://3hentai.net/d/52321":        "https://3hentai.net/d/52321",
		"https://3hentai.net/d/52321/12":     "https://3hentai.net/d/52321",
		"https://3hentai.net/d/52321?x=1":    "https://3hentai.net/d/52321",
		"https://3hentai.net/tags/anal":      "",
		"https://example.com/d/52321":        "",
		"https://3hentai.net/d/52321abc/foo": "",
	} {
		h := NewThreeHentai(&Grabber{URL: in, Settings: &Settings{}})
		ok, _ := h.Test()
		if ok != (want != "") || (ok && h.URL != want) {
			t.Errorf("Test(%q) = %v, URL %q; want %q", in, ok, h.URL, want)
		}
	}
}
