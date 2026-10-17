// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// novelcoolUserAgent is sent on every novelcool.com document request. The
// site cloaks its chapter links for browsers: any User-Agent with "Mozilla"
// in it gets every chapter row pointed at an ad redirector (a rotating
// "…/go/rds/?data=…" domain that lands on an unrelated blog), while other
// clients (its own app, Tachiyomi's okhttp) get the real
// /chapter/{name}/{id}/ links. Images don't care, so pages keep the default.
const novelcoolUserAgent = "okhttp/4.12.0"

// novelcoolChapterIDRe pulls the numeric chapter id out of a reader URL like
// https://www.novelcool.com/chapter/Chapter-100/8476753/ (or a page of it,
// …/8476753-2.html)
var novelcoolChapterIDRe = regexp.MustCompile(`/chapter/[^/]+/(\d+)`)

// novelcoolNumberPrefixRe is the "Chapter 12" the site starts each row's
// title with; what follows it, if anything, is the chapter's own name
var novelcoolNumberPrefixRe = regexp.MustCompile(`(?i)^\s*(?:chapter|ch\.?)\s*\d+(?:\.\d+)?\s*[:\-–]?\s*`)

// Novelcool is a grabber for novelcool.com, which hosts manga and novels side
// by side under /novel/{slug}.html (only the manga are downloadable). The
// reader shows one image per HTML page ({id}-{n}.html, a page selector lists
// them all), and image URLs carry a signed token, so each page document has
// to be fetched. Language editions live on subdomains (es., de., it., br.,
// ru., fr.); www. is English.
type Novelcool struct {
	*Grabber
	doc      *goquery.Document
	resolved bool
}

// NovelcoolChapter is a listed novelcool.com chapter
type NovelcoolChapter struct {
	Chapter
	// URL is the chapter's reader URL, its first page
	URL string
}

// NewNovelcool returns a novelcool.com grabber
func NewNovelcool(g *Grabber) *Novelcool {
	return &Novelcool{Grabber: g}
}

// Test matches novelcool.com series and chapter URLs by domain, since every
// request needs the non-browser User-Agent anyway
func (n *Novelcool) Test() (bool, error) {
	if !hostMatchesDomain(n.URL, "novelcool.com") {
		return false, nil
	}
	u, err := url.Parse(n.URL)
	if err != nil {
		return false, nil
	}

	return strings.HasPrefix(u.Path, "/novel/") || strings.HasPrefix(u.Path, "/chapter/"), nil
}

// novelcoolLanguage returns the language of the edition a URL belongs to: its
// subdomain, or "en" for www.
func novelcoolLanguage(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "en"
	}
	sub, _, _ := strings.Cut(strings.ToLower(u.Hostname()), ".")
	switch sub {
	case "www", "novelcool", "m":
		return "en"
	case "br":
		return "pt-br"
	}

	return sub
}

// get fetches a novelcool.com document with the User-Agent that keeps its
// chapter links real (see novelcoolUserAgent)
func (n *Novelcool) get(uri, referer string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: referer,
		Headers: map[string]string{"User-Agent": novelcoolUserAgent},
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// resolveSeriesURL turns a chapter URL into its series URL, read off the
// reader's back link (chapter URLs carry no series slug). It runs once.
func (n *Novelcool) resolveSeriesURL() error {
	if n.resolved {
		return nil
	}
	n.resolved = true
	if !novelcoolChapterIDRe.MatchString(n.URL) {
		return nil
	}

	doc, err := n.get(n.URL, n.BaseUrl())
	if err != nil {
		return err
	}
	series := novelcoolSeriesLink(doc, n.URL)
	if series == "" {
		return errSeriesNotFound(n.URL)
	}
	n.URL = series

	return nil
}

// novelcoolSeriesLink returns the series a reader page links back to, "" when
// it links to none
func novelcoolSeriesLink(doc *goquery.Document, pageURL string) string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	href, ok := doc.Find(`a[href*="/novel/"]`).First().Attr("href")
	if !ok {
		return ""
	}

	return resolveURLRelative(href, base)
}

// series returns the series page, fetched once
func (n *Novelcool) series() (*goquery.Document, error) {
	if n.doc != nil {
		return n.doc, nil
	}
	if err := n.resolveSeriesURL(); err != nil {
		return nil, err
	}
	doc, err := n.get(n.URL, n.BaseUrl())
	if err != nil {
		return nil, err
	}
	n.doc = doc

	return doc, nil
}

// FetchTitle fetches and returns the manga title
func (n *Novelcool) FetchTitle() (string, error) {
	doc, err := n.series()
	if err != nil {
		return "", err
	}
	title := sanitizeTitle(doc.Find("h1.bookinfo-title").First().Text())
	if title == "" {
		return "", fmt.Errorf("novelcool: no title found in %s", n.URL)
	}

	return title, nil
}

// FetchChapters returns the chapters of the manga
func (n *Novelcool) FetchChapters() (Filterables, []error) {
	doc, err := n.series()
	if err != nil {
		return nil, []error{err}
	}
	// the book type badge tells the novels apart, whose readers hold text
	if doc.Find(".book-type").Length() > 0 && doc.Find(".book-type-manga").Length() == 0 {
		return nil, []error{fmt.Errorf("novelcool: %s is a novel, not a manga", n.URL)}
	}

	return novelcoolChapters(doc, n.URL)
}

// novelcoolChapters reads the chapter rows of a series page. A row whose link
// leaves the site is the ad redirect browsers are served (see
// novelcoolUserAgent); with every row like that, the cloaking won and the
// chapters can't be read.
func novelcoolChapters(doc *goquery.Document, seriesURL string) (chapters Filterables, errs []error) {
	base, _ := url.Parse(seriesURL)
	lang := novelcoolLanguage(seriesURL)
	rows := doc.Find(".chapter-item-list a")
	rows.Each(func(_ int, s *goquery.Selection) {
		href := resolveURLRelative(s.AttrOr("href", ""), base)
		if !hostMatchesDomain(href, "novelcool.com") {
			return
		}
		id := novelcoolChapterIDRe.FindStringSubmatch(href)
		if id == nil {
			return
		}
		text := sanitizeTitle(s.Find(".chapter-item-headtitle").Text())
		if text == "" {
			text = sanitizeTitle(s.AttrOr("title", ""))
		}
		number, ok := parseChapterNumber(text)
		if !ok {
			errs = append(errs, fmt.Errorf("novelcool: no chapter number in %q", text))
			return
		}
		chapters = append(chapters, &NovelcoolChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    novelcoolNumberPrefixRe.ReplaceAllString(text, ""),
				Label:    Label{Style: LabelNumbered},
				Language: lang,
				ID:       id[1],
			},
			URL: href,
		})
	})
	if len(chapters) == 0 && rows.Length() > 0 && len(errs) == 0 {
		errs = append(errs, fmt.Errorf("novelcool: the chapter links of %s point off the site", seriesURL))
	}

	return
}

// FetchChapter fetches a chapter and its pages: the chapter URL is the first
// reader page, whose page selector lists the rest
func (n *Novelcool) FetchChapter(f Filterable) (*Chapter, error) {
	nchap := f.(*NovelcoolChapter)
	doc, err := n.get(nchap.URL, n.URL)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: nchap.Language,
		ID:       nchap.ID,
	}
	first := novelcoolPageImage(doc)
	if first == "" {
		return nil, fmt.Errorf("novelcool: no page image in %s", nchap.URL)
	}
	chapter.Pages = append(chapter.Pages, Page{Number: 1, URL: first})

	for i, page := range novelcoolPageURLs(doc, nchap.URL) {
		if i == 0 {
			continue // the chapter URL itself
		}
		pdoc, err := n.get(page, nchap.URL)
		if err != nil {
			return nil, err
		}
		img := novelcoolPageImage(pdoc)
		if img == "" {
			return nil, fmt.Errorf("novelcool: no page image in %s", page)
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// novelcoolPageURLs returns the reader pages of a chapter in order, read off
// its first page selector (the reader repeats it below the image)
func novelcoolPageURLs(doc *goquery.Document, chapterURL string) []string {
	base, _ := url.Parse(chapterURL)
	var pages []string
	doc.Find("select.sl-page").First().Find("option").Each(func(_ int, s *goquery.Selection) {
		if page := resolveURLRelative(s.AttrOr("value", ""), base); page != "" {
			pages = append(pages, page)
		}
	})

	return pages
}

// novelcoolPageImage returns the image of a reader page, "" when it has none
// (a novel's text chapter)
func novelcoolPageImage(doc *goquery.Document) string {
	img := doc.Find("img.mangaread-manga-pic").First()
	src := strings.TrimSpace(img.AttrOr("src", ""))
	if src == "" || strings.HasPrefix(src, "data:") {
		src = strings.TrimSpace(img.AttrOr("lazy_url", ""))
	}
	if strings.HasPrefix(src, "//") {
		src = "https:" + src
	}

	return src
}
