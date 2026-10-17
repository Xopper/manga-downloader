// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/base64"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestGallerySites reads a trimmed gallery page (and reader, when the pages
// are there) of each single-gallery site engine
func TestGallerySites(t *testing.T) {
	// a URL as hentaifc's reader writes it, its characters' codes
	codes := func(s string) string {
		var parts []string
		for _, r := range s {
			parts = append(parts, strconv.Itoa(int(r)))
		}
		return strings.Join(parts, " ")
	}
	kisu := base64.StdEncoding.EncodeToString([]byte("https://h1.example.com/en/chapters/48536/0.jpg,https://h1.example.com/en/chapters/48536/1.jpg"))

	tests := []struct {
		name  string
		path  string
		pages map[string]func(string) string
		title string
		want  func(base string) []string
	}{
		{
			name: "hentaipill",
			path: "/gallery/g74548",
			pages: map[string]func(string) string{
				"/gallery/g74548": func(string) string {
					return `<h1>[Hagoromo] Iinari [English]</h1><div class="reading-pages-content">
<img class="lazy reading-pages-single-page" data-src="https://b1.example.com/picture/2467918-1.jpg" width="720">
<img class="lazy reading-pages-single-page" data-src="https://b1.example.com/picture/2467918-2.png" width="720"></div>`
				},
			},
			title: "[Hagoromo] Iinari [English]",
			want: func(string) []string {
				return []string{"https://b1.example.com/picture/2467918-1.jpg", "https://b1.example.com/picture/2467918-2.png"}
			},
		},
		{
			name: "doujins",
			path: "/series/how-i-met-83916",
			pages: map[string]func(string) string{
				"/series/how-i-met-83916": func(string) string {
					return `<div class="folder-title"><a href="/series-93083">Original Doujins Series</a> / <a href="/series/how-i-met-83916">How I Met</a></div>
<img class="doujin swiper-lazy" data-file="https://static.example.com/n-3tu8.jpg?st=Cg&amp;e=1792211156" data-extension="jpg">
<img class="doujin swiper-lazy" data-file="https://static.example.com/n-4ab2.jpg?st=Dh&amp;e=1792211156" data-extension="jpg">`
				},
			},
			title: "How I Met",
			want: func(string) []string {
				return []string{"https://static.example.com/n-3tu8.jpg?st=Cg&e=1792211156", "https://static.example.com/n-4ab2.jpg?st=Dh&e=1792211156"}
			},
		},
		{
			name: "photoswipe",
			path: "/bunny-night-club/",
			pages: map[string]func(string) string{
				"/bunny-night-club/": func(string) string {
					return `<h1 class="post-title entry-title"><a href="/bunny-night-club/">[Ghostlessm] Mission: Bunny Night Club</a></h1>
<div class="pswp-gallery"><figure class="pswp-gallery__item"><a href="https://cdn.example.com/2024/06/Bunny-1.webp" itemprop="contentUrl"><img src="https://cdn.example.com/2024/06/Bunny-1-240x360.webp"></a></figure>
<figure class="pswp-gallery__item"><a href="https://cdn.example.com/2024/06/Bunny-2.webp" itemprop="contentUrl"><img src="https://cdn.example.com/2024/06/Bunny-2-240x360.webp"></a></figure></div>`
				},
			},
			title: "[Ghostlessm] Mission: Bunny Night Club",
			want: func(string) []string {
				return []string{"https://cdn.example.com/2024/06/Bunny-1.webp", "https://cdn.example.com/2024/06/Bunny-2.webp"}
			},
		},
		{
			name: "comicpress",
			path: "/gender-goals/",
			pages: map[string]func(string) string{
				"/gender-goals/": func(string) string {
					return `<div id="comic-wrap"><div id="comic"><div id="comic-1" class="comicpane"><img src="https://cdn.example.com/comics/Gender-Goals-1.png"></div>
<div id="comic-2" class="comicpane"><img src="https://cdn.example.com/comics/Gender-Goals-2.jpg"></div></div></div>
<h2 class="post-title"><a href="/gender-goals/">Gender Goals by Jey Pawlik</a></h2><img class="comicthumbnail" src="https://cdn.example.com/hotlink-ok/other-sm.png">`
				},
			},
			title: "Gender Goals by Jey Pawlik",
			want: func(string) []string {
				return []string{"https://cdn.example.com/comics/Gender-Goals-1.png", "https://cdn.example.com/comics/Gender-Goals-2.jpg"}
			},
		},
		{
			name: "multporn",
			path: "/comics/guardian_nikola_6",
			pages: map[string]func(string) string{
				"/comics/guardian_nikola_6": func(string) string {
					return `<h1 id="page-title"> Guardian Nikola 6 </h1><a class="simple-mode-switcher" href="/node/36438/8/simple">Switch to alternative gallery</a>`
				},
				"/node/36438/8/simple": func(base string) string {
					return `<a href="` + base + `/sites/default/files/comics/gn_cover.png" class="mfp-custom-thumbnail mfp-item"><img src="x"></a>
<a href="` + base + `/sites/default/files/comics/gn_pg01.png" class="mfp-custom-thumbnail mfp-item"><img src="y"></a>`
				},
			},
			title: "Guardian Nikola 6",
			want: func(base string) []string {
				return []string{base + "/sites/default/files/comics/gn_cover.png", base + "/sites/default/files/comics/gn_pg01.png"}
			},
		},
		{
			name: "mangaloop",
			path: "/manga/preggo/",
			pages: map[string]func(string) string{
				"/manga/preggo/": func(string) string {
					return `<html><body class="manga-template-default single single-manga wp-theme-mangaloop"><div class="manga-title"><h1>I Gave My Cousin</h1></div></body></html>`
				},
				"/manga/preggo/read/p/1/": func(string) string {
					return `<script>var ajax = {"url":"https://example.com/wp-admin/admin-ajax.php","mangaid":"2641207","pages":["<img class=\"swiper-lazy\" data-src=\"https://i5.example.com/2026/06/preggo-page-1.jpg\">","<img class=\"swiper-lazy\" data-src=\"https://i5.example.com/2026/06/preggo-page-2.jpg\">"]};</script>`
				},
			},
			title: "I Gave My Cousin",
			want: func(string) []string {
				return []string{"https://i5.example.com/2026/06/preggo-page-1.jpg", "https://i5.example.com/2026/06/preggo-page-2.jpg"}
			},
		},
		{
			name: "hentaifc",
			path: "/e/83600",
			pages: map[string]func(string) string{
				"/e/83600": func(base string) string {
					return `<div id="single_book"><h1 class="heading">Iinari Seijo</h1><a href="` + base + `/e/83600/c0">Read Online</a><a href="` + base + `/e/83600/download">Download</a></div>`
				},
				"/e/83600/c0": func(string) string {
					return `<div id="imgs"><div class="wrap_img"><img data-src="#"></div></div><script>var num_page = 2; var ytaw=['` +
						codes("https://img.example.com/a/0.jpg") + `','` + codes("https://img.example.com/a/1.jpg") + `'];function ytc(){}</script>`
				},
			},
			title: "Iinari Seijo",
			want: func(string) []string {
				return []string{"https://img.example.com/a/0.jpg", "https://img.example.com/a/1.jpg"}
			},
		},
		{
			name: "hentaikisu",
			path: "/g/15823",
			pages: map[string]func(string) string{
				"/g/15823": func(string) string {
					return `<a href="/"><h1>HentaiKisu</h1></a><div id="info-block"><div id="info"><h1>Ichigo Cake to Mont Blanc</h1><div>2 Pages</div></div></div>
<img class="thum_pre lozad" src="assets/img/image_error.jpg" data-src="https://h1.example.com/en/chapters/48536/0.jpg">`
				},
				"/read/15823": func(string) string {
					return `<script>la = '` + kisu + `';</script>`
				},
			},
			title: "Ichigo Cake to Mont Blanc",
			want: func(string) []string {
				return []string{"https://h1.example.com/en/chapters/48536/0.jpg", "https://h1.example.com/en/chapters/48536/1.jpg"}
			},
		},
		{
			name: "myadultcomics",
			path: "/read.php",
			pages: map[string]func(string) string{
				"/read.php": func(string) string {
					return `<h1 align='center' id="TOP">Girl Complex 2022</h1>
<script>let template = ['<div class="itempage"><img src="books/1271/1.jpg" alt=""></div>', '<div class="itempage"><img src="books/1271/2.png" alt=""></div>'];</script>`
				},
			},
			title: "Girl Complex 2022",
			want: func(base string) []string {
				return []string{base + "/books/1271/1.jpg", base + "/books/1271/2.png"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := galleryServe(t, tt.pages)
			title, urls := galleryPageURLs(t, srv.URL+tt.path)
			if title != tt.title {
				t.Errorf("title = %q, want %q", title, tt.title)
			}
			if want := tt.want(srv.URL); !slices.Equal(urls, want) {
				t.Errorf("pages = %v, want %v", urls, want)
			}
		})
	}
}

// TestHentaiNexusDecipher deciphers a page list enciphered by the site's
// algorithm (as the reader's script runs it) with the host as part of the key
func TestHentaiNexusDecipher(t *testing.T) {
	blob := "Y1U7Dv6th2tLLQ6MpIN8NluApcrvFDleg6jN8hc8YYar0PUaP2SJrtP4HUJnjLHW+yBFao+02f4jSG2St9wBJudxT4V7LQnkgYy4JWcCY1ecXEQhYCQdKT8QovuDK8fWHVFf//u312rMKFlOgT0no8f8tIvhcv9pC2mAFgBwmfrSh5Ie/utQMuCvlWOYd2IZdkQhOGO79MwsiU1go/zLGYX7B4iXpJ6XuCR+cd3QnO17oZCmcQyjX8vJvLGcfFMFX3twWMomC2KsDETe/NNvjxM="
	want := `[{"type":"image","label":"1","image":"https://images.example.com/v2/abc/001.avif","image_fallback":"https://images.example.com/v2/abc/001.webp"}]`

	got, err := hentaiNexusDecipher(blob, "hentainexus.com")
	if err != nil || string(got) != want {
		t.Errorf("hentaiNexusDecipher() = %q, %v; want %q", got, err, want)
	}
	if got, _ := hentaiNexusDecipher(blob, "example.com"); string(got) == want {
		t.Error("another host deciphered the list, but the host is part of the key")
	}
}
