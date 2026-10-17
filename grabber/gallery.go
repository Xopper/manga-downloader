// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Gallery is a grabber for doujin and hentai gallery sites, where an item is
// one gallery of pages rather than a series of chapters. A gallery is served
// as a series with one chapter holding all its pages. The sites run a few
// shared scripts (galleryEngines), told apart by the gallery page's markup,
// so a clone on a new domain works without code. Some of those scripts (and
// a few adult manhwa apps whose data sits in a script rather than the
// markup PlainHTML reads) list chapters, which an engine's chapters gives.
type Gallery struct {
	*Grabber
	doc    *goquery.Document
	engine *galleryEngine
	title  string
}

// galleryEngine is one gallery script: how its gallery page is recognised,
// titled and turned into the full-size page URLs
type galleryEngine struct {
	// name says which script it is, for errors
	name string
	// match reports whether the fetched page is this script's gallery page
	match func(doc *goquery.Document) bool
	// title is the selector of the gallery's title; titleOf, then og:title
	// are the fallbacks
	title   string
	titleOf func(g *Gallery) string
	// chapters lists a series' chapters, newest or oldest first as the site
	// does; nil serves the page as one gallery
	chapters func(g *Gallery) ([]*GalleryChapter, error)
	// pages returns a chapter's full-size page URLs in reading order (for a
	// gallery, its one chapter's URL is the gallery page's)
	pages func(g *Gallery, c *GalleryChapter) ([]string, error)
}

// galleryEngines are the gallery scripts Gallery knows, tried in order
var galleryEngines = []galleryEngine{
	{
		// GalleryAdults (hentaifox, hentairox, …, Tachiyomi's multisrc
		// "galleryadults"): thumbnails of the first pages, the page count in
		// #load_pages and every page's file type in the g_th JSON
		name: "galleryadults",
		match: func(doc *goquery.Document) bool {
			return doc.Find("#load_pages").Length() > 0 && doc.Find("#load_dir").Length() > 0
		},
		title: "h1",
		pages: galleryAdultsPages,
	},
	{
		// HentaiEnvy's rewrite of GalleryAdults: the thumbnail grid names a
		// template, and the reader page lists every page's file type
		name: "hentaienvy",
		match: func(doc *goquery.Document) bool {
			return doc.Find("#js-thumbs-grid[data-thumb-template]").Length() > 0
		},
		title: "h1#gallery-title",
		pages: galleryEnvyPages,
	},
	{
		// nhentai and its clones (nhentai.to): full-size pages are the
		// thumbnails without their "t"
		name: "nhentai",
		match: func(doc *goquery.Document) bool {
			return doc.Find("#thumbnail-container a.gallerythumb img").Length() > 0
		},
		title: "#info h1, h1",
		pages: galleryNhentaiPages,
	},
	{
		// HentaiHand's Laravel/Vue script: the comic's pages come from its
		// JSON API
		name: "hentaihand",
		match: func(doc *goquery.Document) bool {
			return hentaiHandComicRe.MatchString(doc.Find("script").Text())
		},
		title: "h1",
		pages: galleryHentaiHandPages,
	},
}

// NewGallery returns a new Gallery grabber
func NewGallery(g *Grabber) *Gallery {
	return &Gallery{Grabber: g}
}

// GalleryChapter is a gallery's one chapter
type GalleryChapter struct {
	Chapter
	URL string
}

// Test fetches the page and reports whether it is a gallery of a known
// script
func (g *Gallery) Test() (bool, error) {
	body, err := http.Get(http.RequestParams{
		URL:          g.URL,
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		return false, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return false, err
	}

	for i := range galleryEngines {
		if galleryEngines[i].match(doc) {
			g.doc = doc
			g.engine = &galleryEngines[i]
			return true, nil
		}
	}

	return false, nil
}

// load fetches the gallery page when Test didn't
func (g *Gallery) load() error {
	if g.doc != nil {
		return nil
	}
	if ok, err := g.Test(); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("%s is not a gallery page", g.URL)
	}

	return nil
}

// FetchTitle returns the gallery's title
func (g *Gallery) FetchTitle() (string, error) {
	if g.title != "" {
		return g.title, nil
	}
	if err := g.load(); err != nil {
		return "", err
	}

	if g.engine.title != "" {
		g.title = sanitizeTitle(g.doc.Find(g.engine.title).First().Text())
	}
	if g.title == "" && g.engine.titleOf != nil {
		g.title = sanitizeTitle(g.engine.titleOf(g))
	}
	if g.title == "" {
		g.title = sanitizeTitle(galleryTitleSuffixRe.ReplaceAllString(
			g.doc.Find(`meta[property="og:title"]`).AttrOr("content", ""), ""))
	}
	if g.title == "" {
		return "", errors.New("gallery title not found")
	}

	return g.title, nil
}

// galleryTitleSuffixRe is a site name after the title ("Title - HentaiFox",
// "Title | HentaiHand")
var galleryTitleSuffixRe = regexp.MustCompile(`\s+[-|»]\s+[^-|»]+$`)

// FetchChapters returns the series' chapters, or the gallery as one chapter
func (g *Gallery) FetchChapters() (Filterables, []error) {
	if err := g.load(); err != nil {
		return nil, []error{err}
	}

	if g.engine.chapters == nil {
		return Filterables{&GalleryChapter{
			Chapter: Chapter{
				Number: 1,
				ID:     chapterURLID(g.URL),
				Label:  Label{Style: LabelNumbered},
			},
			URL: g.URL,
		}}, nil
	}

	chapters, err := g.engine.chapters(g)
	if err != nil {
		return nil, []error{err}
	}
	if len(chapters) == 0 {
		return nil, []error{fmt.Errorf("no chapters found for %s", g.URL)}
	}
	list := make(Filterables, 0, len(chapters))
	for _, c := range chapters {
		if c.ID == "" {
			c.ID = chapterURLID(c.URL)
		}
		list = append(list, c)
	}

	return list, nil
}

// FetchChapter returns a chapter's pages
func (g *Gallery) FetchChapter(f Filterable) (*Chapter, error) {
	if err := g.load(); err != nil {
		return nil, err
	}
	gc, ok := f.(*GalleryChapter)
	if !ok {
		return nil, fmt.Errorf("not a gallery chapter: %T", f)
	}

	urls, err := g.engine.pages(g, gc)
	if err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("no pages found for %s", gc.URL)
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		ID:         gc.ID,
		PagesCount: int64(len(urls)),
		Language:   gc.Language,
	}
	if groups := gc.GetGroups(); len(groups) > 0 {
		chapter.SetGroups(groups...)
	}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}

	return chapter, nil
}

// docOf returns the parsed page at pageURL: the gallery page itself, or a
// fetch of another
func (g *Gallery) docOf(pageURL string) (*goquery.Document, error) {
	if pageURL == g.URL && g.doc != nil {
		return g.doc, nil
	}
	return galleryDoc(pageURL, g.URL)
}

// resolve makes href absolute against the gallery page
func (g *Gallery) resolve(href string) string {
	base, _ := url.Parse(g.URL)
	return resolveURLRelative(strings.TrimSpace(href), base)
}

// images returns the first non-empty attribute (of attrs, in order)
// of every element sel matches in doc, made absolute against the gallery
// page, without repeats
func (g *Gallery) images(doc *goquery.Document, sel string, attrs ...string) []string {
	var urls []string
	seen := map[string]bool{}
	doc.Find(sel).Each(func(_ int, s *goquery.Selection) {
		for _, attr := range attrs {
			if v := strings.TrimSpace(s.AttrOr(attr, "")); v != "" && !strings.HasPrefix(v, "data:") {
				u := g.resolve(v)
				if u != "" && !seen[u] {
					seen[u] = true
					urls = append(urls, u)
				}
				return
			}
		}
	})
	return urls
}

// galleryNumbered returns a chapter numbered from its text (else number)
func galleryNumbered(chapterURL, text string, number float64) *GalleryChapter {
	n, ok := parseChapterNumber(text)
	if !ok {
		n = number
	}
	return &GalleryChapter{
		Chapter: Chapter{Number: n, Label: Label{Style: LabelNumbered}},
		URL:     chapterURL,
	}
}

// galleryThumbRe is a thumbnail's page number and file type ("12t.jpg")
var galleryThumbRe = regexp.MustCompile(`/(\d+)t\.(\w+)$`)

// galleryAdultsTypes are g_th's file type letters
var galleryAdultsTypes = map[string]string{"j": "jpg", "p": "png", "b": "bmp", "g": "gif", "w": "webp"}

// galleryAdultsTypesRe is the g_th JSON, `g_th = $.parseJSON('{"1":"j,1280,1920",…}')`
var galleryAdultsTypesRe = regexp.MustCompile(`g_th\s*=\s*\$\.parseJSON\('(\{[^']*\})'\)`)

// galleryAdultsPages builds every page from the first thumbnail's folder, the
// page count and each page's file type (jpg when g_th doesn't say)
func galleryAdultsPages(g *Gallery, _ *GalleryChapter) ([]string, error) {
	count, err := strconv.Atoi(strings.TrimSpace(g.doc.Find("#load_pages").AttrOr("value", "")))
	if err != nil || count <= 0 {
		return nil, errors.New("gallery page count not found")
	}

	folder := ""
	g.doc.Find("img[data-src], img[src]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		src := s.AttrOr("data-src", s.AttrOr("src", ""))
		if galleryThumbRe.MatchString(src) {
			folder = galleryThumbRe.ReplaceAllString(src, "")
			return false
		}
		return true
	})
	if folder == "" {
		return nil, errors.New("gallery thumbnails not found")
	}

	types := map[string]string{}
	if m := galleryAdultsTypesRe.FindStringSubmatch(g.doc.Find("script").Text()); m != nil {
		_ = json.Unmarshal([]byte(m[1]), &types)
	}

	urls := make([]string, count)
	for i := range urls {
		ext := "jpg"
		letter, _, _ := strings.Cut(types[strconv.Itoa(i+1)], ",")
		if t, ok := galleryAdultsTypes[letter]; ok {
			ext = t
		}
		urls[i] = fmt.Sprintf("%s/%d.%s", folder, i+1, ext)
	}

	return urls, nil
}

// galleryEnvyPage is one page of HentaiEnvy's reader list
type galleryEnvyPage struct {
	Page int    `json:"page"`
	Ext  string `json:"ext"`
}

// galleryEnvyPages reads the first page's reader, which lists every page's
// file type in #readerPagesJson next to the image folder
func galleryEnvyPages(g *Gallery, _ *GalleryChapter) ([]string, error) {
	href, ok := g.doc.Find("#js-thumbs-grid a[href]").First().Attr("href")
	if !ok {
		return nil, errors.New("gallery reader link not found")
	}
	reader, err := galleryDoc(g.resolve(href), g.URL)
	if err != nil {
		return nil, err
	}

	folder := strings.TrimSuffix(reader.Find("[data-reader-image-base]").AttrOr("data-reader-image-base", ""), "/")
	var pages []galleryEnvyPage
	if err := json.Unmarshal([]byte(reader.Find("#readerPagesJson").Text()), &pages); err != nil || folder == "" {
		return nil, errors.New("gallery reader's page list not found")
	}

	urls := make([]string, 0, len(pages))
	for _, p := range pages {
		ext := p.Ext
		if ext == "" {
			ext = "webp" // what the site's reader assumes
		}
		urls = append(urls, fmt.Sprintf("%s/%d.%s", folder, p.Page, ext))
	}

	return urls, nil
}

// galleryNhentaiPages turns every thumbnail into its full-size page
func galleryNhentaiPages(g *Gallery, _ *GalleryChapter) ([]string, error) {
	var urls []string
	g.doc.Find("#thumbnail-container a.gallerythumb img").Each(func(_ int, s *goquery.Selection) {
		src := s.AttrOr("data-src", s.AttrOr("src", ""))
		if galleryThumbRe.MatchString(src) {
			urls = append(urls, galleryThumbRe.ReplaceAllString(src, "/$1.$2"))
		}
	})

	return urls, nil
}

// hentaiHandComicRe is the comic HentaiHand's page script holds
var hentaiHandComicRe = regexp.MustCompile(`\bcomic:\s*\{"id":\d+,"linkcode"`)

// hentaiHandSlugRe is a comic's slug in its URL (/en/comic/{slug})
var hentaiHandSlugRe = regexp.MustCompile(`/comic/([^/?#]+)`)

// galleryHentaiHandPages asks the comic's images API
func galleryHentaiHandPages(g *Gallery, _ *GalleryChapter) ([]string, error) {
	m := hentaiHandSlugRe.FindStringSubmatch(g.URL)
	if m == nil {
		return nil, fmt.Errorf("no comic slug in %s", g.URL)
	}
	text, err := http.GetText(http.RequestParams{
		URL:     g.BaseUrl() + "/api/comics/" + m[1] + "/images",
		Referer: g.URL,
		Accept:  "application/json",
	})
	if err != nil {
		return nil, err
	}
	var feed struct {
		Images []struct {
			Page      int    `json:"page"`
			SourceURL string `json:"source_url"`
		} `json:"images"`
	}
	if err := json.Unmarshal([]byte(text), &feed); err != nil {
		return nil, fmt.Errorf("reading the comic's images: %w", err)
	}

	sort.SliceStable(feed.Images, func(i, j int) bool { return feed.Images[i].Page < feed.Images[j].Page })
	urls := make([]string, 0, len(feed.Images))
	for _, img := range feed.Images {
		if img.SourceURL != "" {
			urls = append(urls, img.SourceURL)
		}
	}

	return urls, nil
}

// galleryDoc fetches and parses a page of the gallery
func galleryDoc(pageURL, referer string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: pageURL, Referer: referer})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}
