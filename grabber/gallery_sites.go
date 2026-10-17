// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// gallerySiteEngines are the single-gallery scripts that run on one site
// each (so far): their own markup, a page list in a script, or a reader
// that has to be decoded
var gallerySiteEngines = []galleryEngine{
	{
		// hentaipill.com: the gallery page is the reader
		name: "hentaipill",
		match: func(doc *goquery.Document) bool {
			return doc.Find("div.reading-pages-content img.reading-pages-single-page").Length() > 0
		},
		title: "h1",
		pages: func(g *Gallery, _ *GalleryChapter) ([]string, error) {
			return g.images(g.doc, "div.reading-pages-content img.reading-pages-single-page", "data-src", "src"), nil
		},
	},
	{
		// doujins.com: every page's signed full-size URL (valid for about
		// an hour) in data-file
		name: "doujins",
		match: func(doc *goquery.Document) bool {
			return doc.Find("img.doujin[data-file]").Length() > 0
		},
		title: "div.folder-title a:last-of-type",
		pages: func(g *Gallery, _ *GalleryChapter) ([]string, error) {
			return g.images(g.doc, "img.doujin[data-file]", "data-file"), nil
		},
	},
	{
		// xyzcomics.com (WordPress, PhotoSwipe): the full-size pages are the
		// gallery's links, the <img>s inside them thumbnails
		name: "pswp-gallery",
		match: func(doc *goquery.Document) bool {
			return doc.Find(".pswp-gallery figure a[itemprop=contentUrl]").Length() > 0
		},
		title: "h1.post-title",
		pages: func(g *Gallery, _ *GalleryChapter) ([]string, error) {
			return g.images(g.doc, ".pswp-gallery figure a[itemprop=contentUrl]", "href"), nil
		},
	},
	{
		// ohjoysextoy.com (WordPress, ComicPress): one comic per post, a
		// pane per page, apart from the archive's thumbnails
		name: "comicpress",
		match: func(doc *goquery.Document) bool {
			return doc.Find("#comic .comicpane img").Length() > 0
		},
		title: "h2.post-title",
		pages: func(g *Gallery, _ *GalleryChapter) ([]string, error) {
			return g.images(g.doc, "#comic .comicpane img", "src"), nil
		},
	},
	{
		// multporn.net (Drupal, Juicebox): the "alternative gallery" page
		// links every original
		name: "multporn",
		match: func(doc *goquery.Document) bool {
			return doc.Find("a.simple-mode-switcher[href]").Length() > 0
		},
		title: "h1#page-title",
		pages: func(g *Gallery, _ *GalleryChapter) ([]string, error) {
			simple, err := g.docOf(g.resolve(g.doc.Find("a.simple-mode-switcher").AttrOr("href", "")))
			if err != nil {
				return nil, err
			}
			return g.images(simple, "a.mfp-item[href]", "href"), nil
		},
	},
	{
		// hentailoop.com (WordPress, mangaloop theme): the reader's script
		// lists every page as an <img> string
		name: "mangaloop",
		match: func(doc *goquery.Document) bool {
			return doc.Find("body.wp-theme-mangaloop.single-manga").Length() > 0
		},
		title: "div.manga-title h1",
		pages: hentaiLoopPages,
	},
	{
		// hentaifc.com: the reader (/e/{id}/c0) writes each page's URL as
		// character codes
		name: "hentaifc",
		match: func(doc *goquery.Document) bool {
			return doc.Find("#single_book").Length() > 0
		},
		title: "h1.heading",
		pages: hentaiFCPages,
	},
	{
		// hentaikisu.com: the item page shows only the first pages; the
		// reader (/read/{id}) holds them all as base64
		name: "hentaikisu",
		match: func(doc *goquery.Document) bool {
			return doc.Find("#info-block").Length() > 0 && doc.Find("img.thum_pre").Length() > 0
		},
		title: "#info h1",
		pages: hentaiKisuPages,
	},
	{
		// myadultcomics.com: a script template holds every page, only ten
		// of which are on the page
		name: "myadultcomics",
		match: func(doc *goquery.Document) bool {
			return doc.Find("h1#TOP").Length() > 0 && myAdultComicsPageRe.MatchString(doc.Find("script").Text())
		},
		title: "h1#TOP",
		pages: func(g *Gallery, _ *GalleryChapter) ([]string, error) {
			var urls []string
			seen := map[string]bool{}
			for _, m := range myAdultComicsPageRe.FindAllString(g.doc.Find("script").Text(), -1) {
				if u := g.resolve("/" + m); !seen[u] {
					seen[u] = true
					urls = append(urls, u)
				}
			}
			return urls, nil
		},
	},
	{
		// hentainexus.com: the reader's page list is enciphered with the
		// site's host as part of the key
		name: "hentainexus",
		match: func(doc *goquery.Document) bool {
			return doc.Find("h1.title").Length() > 0 && doc.Find(`a[href^="/read/"]`).Length() > 0
		},
		title: "h1.title",
		pages: hentaiNexusPages,
	},
}

func init() {
	galleryEngines = append(galleryEngines, gallerySiteEngines...)
}

// hentaiLoopPagesRe is the reader script's page list
var hentaiLoopPagesRe = regexp.MustCompile(`var ajax\s*=\s*(\{.*?\});`)

// hentaiLoopSrcRe is a page in one of the list's <img> strings
var hentaiLoopSrcRe = regexp.MustCompile(`data-src="([^"]+)"`)

// hentaiLoopPages reads the first reader page's list of every page
func hentaiLoopPages(g *Gallery, _ *GalleryChapter) ([]string, error) {
	reader, err := galleryDoc(strings.TrimSuffix(g.URL, "/")+"/read/p/1/", g.URL)
	if err != nil {
		return nil, err
	}
	m := hentaiLoopPagesRe.FindStringSubmatch(reader.Find("script").Text())
	if m == nil {
		return nil, errors.New("the reader's page list wasn't found")
	}
	var data struct {
		Pages []string `json:"pages"`
	}
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return nil, fmt.Errorf("reading the reader's page list: %w", err)
	}

	var urls []string
	for _, p := range data.Pages {
		if src := hentaiLoopSrcRe.FindStringSubmatch(p); src != nil {
			urls = append(urls, html.UnescapeString(src[1]))
		}
	}
	return urls, nil
}

var (
	// hentaiFCReaderRe is the reader's path (/e/{id}/c{n})
	hentaiFCReaderRe = regexp.MustCompile(`/e/\d+/c\d+$`)
	// hentaiFCPagesRe is the reader's list of pages as character codes
	hentaiFCPagesRe = regexp.MustCompile(`var ytaw=\[(.*?)\];`)
	// hentaiFCQuotedRe is one page of the list
	hentaiFCQuotedRe = regexp.MustCompile(`'([\d ]+)'`)
)

// hentaiFCPages decodes the reader's character-code list
func hentaiFCPages(g *Gallery, _ *GalleryChapter) ([]string, error) {
	readerURL := ""
	g.doc.Find("a[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		href := g.resolve(s.AttrOr("href", ""))
		if hentaiFCReaderRe.MatchString(href) {
			readerURL = href
			return false
		}
		return true
	})
	if readerURL == "" {
		return nil, errors.New("the gallery's reader link wasn't found")
	}
	reader, err := galleryDoc(readerURL, g.URL)
	if err != nil {
		return nil, err
	}
	m := hentaiFCPagesRe.FindStringSubmatch(reader.Find("script").Text())
	if m == nil {
		return nil, errors.New("the reader's page list wasn't found")
	}

	return hentaiFCDecode(m[1]), nil
}

// hentaiFCDecode turns the quoted lists of character codes into URLs
func hentaiFCDecode(list string) []string {
	var urls []string
	for _, q := range hentaiFCQuotedRe.FindAllStringSubmatch(list, -1) {
		var b strings.Builder
		for _, code := range strings.Fields(q[1]) {
			if n, err := strconv.Atoi(code); err == nil {
				b.WriteRune(rune(n))
			}
		}
		urls = append(urls, b.String())
	}
	return urls
}

// hentaiKisuPagesRe is the reader's base64 list of pages
var hentaiKisuPagesRe = regexp.MustCompile(`\bla\s*=\s*'([A-Za-z0-9+/=]+)'`)

// hentaiKisuIDRe is the gallery's id in its URL (/g/{id})
var hentaiKisuIDRe = regexp.MustCompile(`/g/(\d+)`)

// hentaiKisuPages decodes the reader's comma separated page list
func hentaiKisuPages(g *Gallery, _ *GalleryChapter) ([]string, error) {
	m := hentaiKisuIDRe.FindStringSubmatch(g.URL)
	if m == nil {
		return nil, fmt.Errorf("no gallery id in %s", g.URL)
	}
	reader, err := galleryDoc(g.BaseUrl()+"/read/"+m[1], g.URL)
	if err != nil {
		return nil, err
	}
	list := hentaiKisuPagesRe.FindStringSubmatch(reader.Find("script").Text())
	if list == nil {
		return nil, errors.New("the reader's page list wasn't found")
	}
	decoded, err := base64.StdEncoding.DecodeString(list[1])
	if err != nil {
		return nil, fmt.Errorf("decoding the reader's page list: %w", err)
	}

	var urls []string
	for _, u := range strings.Split(string(decoded), ",") {
		if u = strings.TrimSpace(u); u != "" {
			urls = append(urls, u)
		}
	}
	return urls, nil
}

// myAdultComicsPageRe is a page in the page template script
var myAdultComicsPageRe = regexp.MustCompile(`books/\d+/\d+\.\w+`)

// hentaiNexusReaderRe is the reader's enciphered page list
var hentaiNexusReaderRe = regexp.MustCompile(`initReader\("([^"]+)"`)

// hentaiNexusIDRe is the gallery's id in its URL (/view/{id})
var hentaiNexusIDRe = regexp.MustCompile(`/(?:view|read)/(\d+)`)

// hentaiNexusPages deciphers the reader's page list
func hentaiNexusPages(g *Gallery, _ *GalleryChapter) ([]string, error) {
	m := hentaiNexusIDRe.FindStringSubmatch(g.URL)
	if m == nil {
		return nil, fmt.Errorf("no gallery id in %s", g.URL)
	}
	reader, err := galleryDoc(g.BaseUrl()+"/read/"+m[1], g.URL)
	if err != nil {
		return nil, err
	}
	blob := hentaiNexusReaderRe.FindStringSubmatch(reader.Find("script").Text())
	if blob == nil {
		return nil, errors.New("the reader's page list wasn't found")
	}
	u, _ := url.Parse(g.URL)
	plain, err := hentaiNexusDecipher(blob[1], u.Hostname())
	if err != nil {
		return nil, err
	}

	var pages []struct {
		Type     string `json:"type"`
		Fallback string `json:"image_fallback"`
		Image    string `json:"image"`
	}
	if err := json.Unmarshal(plain, &pages); err != nil {
		return nil, fmt.Errorf("reading the reader's page list: %w", err)
	}
	var urls []string
	for _, p := range pages {
		if p.Type != "" && p.Type != "image" {
			continue
		}
		if p.Fallback != "" {
			urls = append(urls, p.Fallback)
		} else if p.Image != "" {
			urls = append(urls, p.Image)
		}
	}
	return urls, nil
}

// hentaiNexusPrimes are the stream's possible steps
var hentaiNexusPrimes = [16]int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47, 53}

// hentaiNexusDecipher undoes the reader's cipher (from its reader.min.js):
// the first 64 bytes, XORed with the host, are the key of an RC4-like
// stream whose step is a prime picked from a checksum of the key
func hentaiNexusDecipher(blob, host string) ([]byte, error) {
	d, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("decoding the reader's page list: %w", err)
	}
	if len(d) < 64 {
		return nil, errors.New("the reader's page list is too short")
	}
	for i := 0; i < len(host) && i < 64; i++ {
		d[i] ^= host[i]
	}

	x := 0
	for i := range 64 {
		x ^= int(d[i])
		for range 8 {
			if x&1 == 1 {
				x = x>>1 ^ 0xc
			} else {
				x >>= 1
			}
		}
	}
	x &= 7

	var s [256]int
	for i := range s {
		s[i] = i
	}
	for i, j := 0, 0; i < 256; i++ {
		j = (j + s[i] + int(d[i%64])) % 256
		s[i], s[j] = s[j], s[i]
	}

	p := hentaiNexusPrimes[x]
	out := make([]byte, 0, len(d)-64)
	for n, i, j, k, b := 0, 0, 0, 0, 0; n < len(d)-64; n++ {
		i = (i + p) % 256
		j = (b + s[(j+s[i])%256]) % 256
		b = (b + i + s[i]) % 256
		s[i], s[j] = s[j], s[i]
		k = s[(j+s[(i+s[(k+b)%256])%256])%256]
		out = append(out, d[n+64]^byte(k))
	}
	return out, nil
}
