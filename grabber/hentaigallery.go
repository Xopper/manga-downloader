// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// HentaiGallery is a grabber for the doujinshi gallery sites of one network
// (imhentai.xxx, hentaiera.com, hentaizap.com, nhentai.xxx, asmhentai.com),
// plain HTTP all of them. A gallery is one work, listed as a single chapter.
// The gallery page names the image server, folder and page count, and its
// thumbnails ({n}t.jpg) sit next to the pages ({n}.{ext}); the extension of
// each page comes from the page's g_th list where there is one, from the
// first reader page's JSON on hentaizap, and from each reader page on
// asmhentai, which states neither.
type HentaiGallery struct {
	*Grabber
	host hentaiGalleryHost
	id   string
	doc  *goquery.Document
}

// hentaiGalleryHost is how a site of the network lays out its URLs: the
// gallery page and the reader page of page {n}, both with {id}
type hentaiGalleryHost struct {
	gallery, reader string
}

var hentaiGalleryHosts = map[string]hentaiGalleryHost{
	"imhentai.xxx":  {"/gallery/{id}/", "/view/{id}/{n}/"},
	"hentaiera.com": {"/gallery/{id}/", "/view/{id}/{n}/"},
	"hentaizap.com": {"/gallery/{id}/", "/g/{id}/{n}/"},
	"nhentai.xxx":   {"/g/{id}/", "/g/{id}/{n}/"},
	"asmhentai.com": {"/g/{id}/", "/gallery/{id}/{n}/"},
}

// HentaiGalleryChapter is the gallery, as the only chapter
type HentaiGalleryChapter struct {
	Chapter
	URL string
}

func NewHentaiGallery(g *Grabber) *HentaiGallery {
	return &HentaiGallery{Grabber: g}
}

var hentaiGalleryIDRe = regexp.MustCompile(`^/(?:gallery|g|view)/(\d+)(?:/|$)`)

// Test returns true for a gallery or reader URL of one of the network's
// sites, which it rewrites to the gallery page
func (h *HentaiGallery) Test() (bool, error) {
	u, err := url.Parse(h.URL)
	if err != nil {
		return false, nil
	}
	host, ok := hentaiGalleryHosts[strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")]
	if !ok {
		return false, nil
	}
	m := hentaiGalleryIDRe.FindStringSubmatch(u.Path)
	if m == nil {
		return false, nil
	}
	h.host, h.id = host, m[1]
	h.URL = "https://" + u.Hostname() + strings.ReplaceAll(host.gallery, "{id}", m[1])
	return true, nil
}

func (h *HentaiGallery) page() (*goquery.Document, error) {
	if h.doc != nil {
		return h.doc, nil
	}
	body, err := http.Get(http.RequestParams{URL: h.URL, Referer: h.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	h.doc, err = goquery.NewDocumentFromReader(body)
	return h.doc, err
}

// FetchTitle returns the gallery's title
func (h *HentaiGallery) FetchTitle() (string, error) {
	doc, err := h.page()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find("h1").First().Text()), nil
}

// FetchChapters returns the gallery as its only chapter
func (h *HentaiGallery) FetchChapters() (Filterables, []error) {
	if _, err := h.page(); err != nil {
		return nil, []error{err}
	}
	return Filterables{&HentaiGalleryChapter{
		Chapter: Chapter{Number: 1, Label: Label{Style: LabelNumbered}, ID: h.id},
		URL:     h.URL,
	}}, nil
}

// FetchChapter lists the gallery's pages
func (h *HentaiGallery) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := h.page()
	if err != nil {
		return nil, err
	}
	html, _ := doc.Html()

	var urls []string
	switch {
	case strings.Contains(html, "g_th"):
		urls, err = hentaiGalleryFromThumbs(html, doc)
	case doc.Find("#js-thumbs-grid").Length() > 0:
		urls, err = h.hentaizapPages()
	default:
		urls, err = h.readerPages(doc)
	}
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Label:      labelOf(f),
		ID:         h.id,
		Language:   "en",
		PagesCount: int64(len(urls)),
	}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	return chapter, nil
}

var (
	hentaiGalleryThRe    = regexp.MustCompile(`g_th\s*=\s*\$\.parseJSON\('(.*?)'\)`)
	hentaiGalleryThumbRe = regexp.MustCompile(`(https?://[^"'\s]+/)1t\.\w+`)
	hentaiGalleryExts    = map[string]string{"j": "jpg", "p": "png", "b": "bmp", "g": "gif", "w": "webp"}
)

// hentaiGalleryFromThumbs builds the page URLs from the first thumbnail's
// folder and the g_th list, which gives each page's type as its first letter
// ({"1":"w,1280,1829"}, nested under "fl" on nhentai.xxx)
func hentaiGalleryFromThumbs(html string, doc *goquery.Document) ([]string, error) {
	th := hentaiGalleryThRe.FindStringSubmatch(html)
	base := hentaiGalleryThumbRe.FindStringSubmatch(html)
	if th == nil || base == nil {
		return nil, errors.New("could not find the gallery's page list")
	}
	var list map[string]json.RawMessage
	if err := json.Unmarshal([]byte(th[1]), &list); err != nil {
		return nil, err
	}
	if nested, ok := list["fl"]; ok {
		list = nil
		if err := json.Unmarshal(nested, &list); err != nil {
			return nil, err
		}
	}
	total, _ := strconv.Atoi(doc.Find("#load_pages").AttrOr("value", ""))
	if total == 0 {
		total = len(list)
	}
	urls := make([]string, 0, total)
	for n := 1; n <= total; n++ {
		var info string
		if err := json.Unmarshal(list[strconv.Itoa(n)], &info); err != nil {
			return nil, fmt.Errorf("page %d is missing from the gallery's page list", n)
		}
		ext, ok := hentaiGalleryExts[strings.SplitN(info, ",", 2)[0]]
		if !ok {
			return nil, fmt.Errorf("page %d has an unknown image type %q", n, info)
		}
		urls = append(urls, fmt.Sprintf("%s%d.%s", base[1], n, ext))
	}
	return urls, nil
}

// hentaizapPages reads the page list hentaizap's reader embeds as JSON
func (h *HentaiGallery) hentaizapPages() ([]string, error) {
	doc, err := h.readerPage(1)
	if err != nil {
		return nil, err
	}
	base := doc.Find("[data-reader-image-base]").AttrOr("data-reader-image-base", "")
	var pages []struct {
		Page int    `json:"page"`
		Ext  string `json:"ext"`
	}
	if err := json.Unmarshal([]byte(doc.Find("script#readerPagesJson").Text()), &pages); err != nil || base == "" {
		return nil, errors.New("could not find the gallery's page list")
	}
	urls := make([]string, 0, len(pages))
	for _, p := range pages {
		urls = append(urls, fmt.Sprintf("%s/%d.%s", strings.TrimSuffix(base, "/"), p.Page, p.Ext))
	}
	return urls, nil
}

// readerPages reads the picture of every reader page, for asmhentai, whose
// gallery page only gives the page count (#t_pages)
func (h *HentaiGallery) readerPages(doc *goquery.Document) ([]string, error) {
	total, _ := strconv.Atoi(doc.Find("#t_pages, #load_pages").First().AttrOr("value", ""))
	if total == 0 {
		return nil, errors.New("could not find the gallery's page count")
	}
	urls := make([]string, 0, total)
	for n := 1; n <= total; n++ {
		page, err := h.readerPage(n)
		if err != nil {
			return nil, err
		}
		img := page.Find("img#fimg, img#gimg").First()
		u := img.AttrOr("data-src", img.AttrOr("src", ""))
		if u == "" || strings.HasPrefix(u, "data:") {
			return nil, fmt.Errorf("page %d has no picture", n)
		}
		if strings.HasPrefix(u, "//") {
			u = "https:" + u
		}
		urls = append(urls, u)
	}
	return urls, nil
}

func (h *HentaiGallery) readerPage(n int) (*goquery.Document, error) {
	path := strings.ReplaceAll(strings.ReplaceAll(h.host.reader, "{id}", h.id), "{n}", strconv.Itoa(n))
	body, err := http.Get(http.RequestParams{URL: h.BaseUrl() + path, Referer: h.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}
