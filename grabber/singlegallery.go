// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// SingleGallery is a grabber for sites where one URL is one gallery (an
// adult comic or a photo set) with every picture linked or shown on the
// page, sometimes split over a few numbered pages. Each site is an entry in
// singleGallerySites, matched by domain; the gallery is listed as a single
// chapter.
type SingleGallery struct {
	*Grabber
	site singleGallerySite
	doc  *goquery.Document
}

// SingleGalleryChapter is the gallery, as the only chapter
type SingleGalleryChapter struct {
	Chapter
	URL string
}

// singleGallerySite describes one site's gallery page
type singleGallerySite struct {
	Domains []string
	// Title selects the gallery's name; TitleNoise elements inside it are
	// left out (a "(55P)" count), and TitleStrip text matching it
	Title, TitleNoise string
	TitleStrip        *regexp.Regexp
	// Pages selects one element per picture, and Attrs the attributes to
	// read its URL from, the first one set winning
	Pages string
	Attrs []string
	// Rewrite, when set, turns a thumbnail's URL into the picture's,
	// replacing its matches with RewriteTo (WordPress' -739x1024 size
	// suffix dropped, a /thumb/ folder swapped)
	Rewrite   *regexp.Regexp
	RewriteTo string
	// Pagination selects the links to the gallery's other pages, followed
	// until none is new
	Pagination string
	// Build, when set, lists the pictures from the page's source instead of
	// Pages
	Build func(html string, base *url.URL) []string
}

var (
	koharuHashRe  = regexp.MustCompile(`hash:"([0-9a-f]+)"`)
	koharuPagesRe = regexp.MustCompile(`pages:(\d+)`)
)

// koharuPictures lists a Koharu gallery's pictures, /image/{hash}/{n}
func koharuPictures(html string, base *url.URL) (urls []string) {
	hash := koharuHashRe.FindStringSubmatch(html)
	pages := koharuPagesRe.FindStringSubmatch(html)
	if hash == nil || pages == nil {
		return nil
	}
	n, _ := strconv.Atoi(pages[1])
	for i := 1; i <= n; i++ {
		urls = append(urls, resolveURLRelative("/image/"+hash[1]+"/"+strconv.Itoa(i), base))
	}
	return urls
}

// singleGalleryMaxPages bounds the pages of one gallery that are walked
const singleGalleryMaxPages = 60

var singleGallerySites = []singleGallerySite{
	// two WordPress sites of one theme: a PhotoSwipe link per page
	{
		Domains: []string{"bestporncomix.com", "ilikecomix.com"},
		Title:   "h1.entry-title",
		Pages:   `a[itemprop="contentUrl"]`,
		Attrs:   []string{"href"},
	},
	// the photo set network of elitebabes.com: a link per photo, sized
	{
		Domains: []string{"elitebabes.com", "joymiihub.com", "metarthunter.com", "femjoyhunter.com",
			"ftvhunter.com", "xarthunter.com", "pmatehunter.com"},
		Title: "h1",
		Pages: `ul.list-gallery.css a[data-width]`,
		Attrs: []string{"href"},
	},
	{
		Domains: []string{"photos18.com"},
		Title:   "h1.title",
		Pages:   `a[data-fancybox="gallery"]`,
		Attrs:   []string{"href"},
	},
	{
		Domains: []string{"mitaku.net"},
		Title:   "h1.entry-title",
		Pages:   "img.msacwl-img",
		Attrs:   []string{"data-lazy", "src"},
	},
	// pages of 14 photos; the theme swaps its prev/next classes, and its
	// "next" one leads to another set
	{
		Domains:    []string{"foamgirl.net"},
		Title:      "h1",
		TitleNoise: "span",
		Pages:      "a.imageclick-imgbox",
		Attrs:      []string{"href"},
		Pagination: "a.page-numbers:not(.next)",
	},
	// a WordPress post split in numbered pages of 20 photos
	{
		Domains:    []string{"4khd.com"},
		Title:      "h3.wp-block-post-title",
		Pages:      ".entry-content img",
		Attrs:      []string{"src"},
		Pagination: ".page-links a.page-numbers",
	},
	// WordPress gallery blocks, of resized copies on onlythebesthentai
	{
		Domains:   []string{"fsicomics.com"},
		Title:     "h1",
		Pages:     ".entry-content .wp-block-gallery img",
		Attrs:     []string{"src"},
		Rewrite:   regexp.MustCompile(`-\d+x\d+(\.\w+)$`),
		RewriteTo: "$1",
	},
	{
		Domains:   []string{"onlythebesthentai.com"},
		Title:     "h1.manga-title",
		Pages:     ".wp-block-gallery img",
		Attrs:     []string{"src"},
		Rewrite:   regexp.MustCompile(`-\d+x\d+(\.\w+)(?:\.webp)?$`),
		RewriteTo: "$1",
	},
	// two sites of one engine, ?page=N pages of 20 photos
	{
		Domains:    []string{"buondua.com", "xiutaku.com"},
		Title:      "h1",
		TitleStrip: regexp.MustCompile(`\s*-\s*\(\s*Page \d+ / \d+\s*\)\s*$`),
		Pages:      ".article-fulltext img",
		Attrs:      []string{"src"},
		Pagination: "a.pagination-link",
	},
	// a thumbnail per page, linking to a one-page reader
	{
		Domains:    []string{"eggporncomics.com"},
		Title:      "h1",
		TitleStrip: regexp.MustCompile(`\s+-\s+[^-]*porn comics$`),
		Pages:      `a[href*="per-page="] img`,
		Attrs:      []string{"src"},
		Rewrite:    regexp.MustCompile(`/thumb\d+_`),
		RewriteTo:  "/",
	},
	{
		Domains:    []string{"erofus.com"},
		Title:      "title",
		TitleStrip: regexp.MustCompile(`\s*\|\s*Erofus.*$`),
		Pages:      `a[href*="/pic/"] img`,
		Attrs:      []string{"src"},
		Rewrite:    regexp.MustCompile(`^/thumb/`),
		RewriteTo:  "/medium/",
	},
	// a Koharu (faccina) instance: the page data names the gallery's hash
	// and page count, and only the first thumbnails are rendered
	{
		Domains:    []string{"hentalk.pw"},
		Title:      "title",
		TitleStrip: regexp.MustCompile(`\s+-\s+faccina$`),
		Build:      koharuPictures,
	},
	// the gallery page shows thumbnails; its first reader page lists every
	// page's picture
	{
		Domains:    []string{"rokuhentai.com"},
		Title:      "title",
		TitleStrip: regexp.MustCompile(`\s+-\s+Roku Hentai$`),
		Pages:      `img[data-src*="/_images/pages/"]`,
		Attrs:      []string{"data-src"},
		Pagination: `a[href$="/0"]`,
	},
	{
		Domains: []string{"ososedki.com"},
		Title:   "h1",
		Pages:   `#photos a[data-gallery="album"]`,
		Attrs:   []string{"href"},
	},
	{
		Domains: []string{"cutiecomics.com"},
		Title:   "h1#page-title",
		Pages:   ".xfieldimagegallery img",
		Attrs:   []string{"src"},
	},
}

func NewSingleGallery(g *Grabber) *SingleGallery {
	return &SingleGallery{Grabber: g}
}

// Test returns true for a URL on one of the sites' domains
func (s *SingleGallery) Test() (bool, error) {
	u, err := url.Parse(s.URL)
	if err != nil {
		return false, nil
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	for _, site := range singleGallerySites {
		for _, domain := range site.Domains {
			if host == domain {
				s.site = site
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *SingleGallery) fetch(u string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: u, Referer: s.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

func (s *SingleGallery) page() (*goquery.Document, error) {
	if s.doc != nil {
		return s.doc, nil
	}
	doc, err := s.fetch(s.URL)
	s.doc = doc
	return doc, err
}

// FetchTitle returns the gallery's name
func (s *SingleGallery) FetchTitle() (string, error) {
	doc, err := s.page()
	if err != nil {
		return "", err
	}
	title := doc.Find(s.site.Title).First().Clone()
	if s.site.TitleNoise != "" {
		title.Find(s.site.TitleNoise).Remove()
	}
	text := title.Text()
	if s.site.TitleStrip != nil {
		text = s.site.TitleStrip.ReplaceAllString(text, "")
	}
	return sanitizeTitle(text), nil
}

// FetchChapters returns the gallery as its only chapter
func (s *SingleGallery) FetchChapters() (Filterables, []error) {
	if _, err := s.page(); err != nil {
		return nil, []error{err}
	}
	return Filterables{&SingleGalleryChapter{
		Chapter: Chapter{Number: 1, Label: Label{Style: LabelNumbered}, ID: chapterURLID(s.URL)},
		URL:     s.URL,
	}}, nil
}

// FetchChapter collects the pictures of every page of the gallery
func (s *SingleGallery) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := s.page()
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(s.URL)
	// the gallery's other pages extend its path (/1838407_2.html,
	// /post.html/2): a link elsewhere is another gallery
	stem := strings.TrimSuffix(base.Path, path.Ext(base.Path))
	queue := []string{s.URL}
	seenPage := map[string]bool{s.URL: true}
	var walked []singleGalleryPage
	for i := 0; i < len(queue) && i < singleGalleryMaxPages; i++ {
		if i > 0 {
			if doc, err = s.fetch(queue[i]); err != nil {
				return nil, err
			}
		}
		walked = append(walked, singleGalleryPage{
			index: singleGalleryPageIndex(queue[i], stem),
			pics:  s.pictures(doc, base),
		})
		if s.site.Pagination == "" {
			break
		}
		doc.Find(s.site.Pagination).Each(func(_ int, a *goquery.Selection) {
			u := resolveURLRelative(a.AttrOr("href", ""), base)
			next, err := url.Parse(u)
			if u != "" && err == nil && next.Host == base.Host && strings.HasPrefix(next.Path, stem) && !seenPage[u] {
				seenPage[u] = true
				queue = append(queue, u)
			}
		})
	}
	// pages are found out of order when the pager skips some (1 2 … 5)
	slices.SortStableFunc(walked, func(a, b singleGalleryPage) int { return a.index - b.index })
	seenPic := map[string]bool{}
	var pics []string
	for _, p := range walked {
		for _, u := range p.pics {
			if !seenPic[u] {
				seenPic[u] = true
				pics = append(pics, u)
			}
		}
	}
	if len(pics) == 0 {
		return nil, errors.New("no pictures found on the gallery page")
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Label:      labelOf(f),
		ID:         f.(*SingleGalleryChapter).ID,
		PagesCount: int64(len(pics)),
	}
	for i, u := range pics {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	return chapter, nil
}

// singleGalleryPage is one walked page of a gallery: its position and its
// pictures
type singleGalleryPage struct {
	index int
	pics  []string
}

var singleGalleryIndexRe = regexp.MustCompile(`(\d+)\D*$`)

// singleGalleryPageIndex returns the position of page u of the gallery whose
// path without extension is stem: the last number after the stem
// (_2.html, .html/3, ?page=4), 1 for the first page
func singleGalleryPageIndex(u, stem string) int {
	_, rest, ok := strings.Cut(u, stem)
	if !ok {
		return 1
	}
	if m := singleGalleryIndexRe.FindStringSubmatch(rest); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 1
}

// pictures returns the absolute picture URLs on one page of the gallery
func (s *SingleGallery) pictures(doc *goquery.Document, base *url.URL) (urls []string) {
	if s.site.Build != nil {
		html, _ := doc.Html()
		return s.site.Build(html, base)
	}
	doc.Find(s.site.Pages).Each(func(_ int, el *goquery.Selection) {
		for _, attr := range s.site.Attrs {
			v := strings.TrimSpace(el.AttrOr(attr, ""))
			if v == "" || strings.HasPrefix(v, "data:") {
				continue
			}
			if s.site.Rewrite != nil {
				v = s.site.Rewrite.ReplaceAllString(v, s.site.RewriteTo)
			}
			if u := resolveURLRelative(v, base); u != "" {
				urls = append(urls, u)
			}
			return
		}
	})
	return urls
}
