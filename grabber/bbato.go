// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// bbatoDomains are the hosts of the "BATOTO"-branded bato1.com (no relation
// to bato.to's own code: different markup and URLs)
var bbatoDomains = []string{"bato1.com", "bbato.com"}

// bbatoPathRe matches series (/manga/{slug}) and reader
// (/read/{slug}/{chapter}) paths, capturing the series slug
var bbatoPathRe = regexp.MustCompile(`^/(?:manga|read)/([^/?#]+)`)

// bbatoNumberPrefixRe is the "Chapter 12" a chapter name starts with; what
// follows, if anything, is the chapter's own title
var bbatoNumberPrefixRe = regexp.MustCompile(`(?i)^\s*chapter\s*\d+(?:\.\d+)?\s*[:\-–]?\s*`)

// Bbato is a grabber for bato1.com. The series page lists no chapters: they
// come from /get-chapter-list?slug={slug}, a JSON answered only to
// XMLHttpRequest calls. A chapter's reader is plain HTML, with the site's own
// banner as a .notice-page in front of the pages.
type Bbato struct {
	*Grabber
	slug string
}

// BbatoChapter is a listed bato1.com chapter
type BbatoChapter struct {
	Chapter
	// Slug is the chapter's part of the reader URL ("chapter-23")
	Slug string
}

// NewBbato returns a bato1.com grabber
func NewBbato(g *Grabber) *Bbato {
	return &Bbato{Grabber: g}
}

// Test matches bato1.com series and chapter URLs by domain
func (b *Bbato) Test() (bool, error) {
	if !slices.ContainsFunc(bbatoDomains, func(d string) bool { return hostMatchesDomain(b.URL, d) }) {
		return false, nil
	}
	u, err := url.Parse(b.URL)
	if err != nil {
		return false, nil
	}

	return bbatoPathRe.MatchString(u.Path), nil
}

// resolveSeriesURL turns a reader URL into its series URL, whose slug the
// reader path starts with
func (b *Bbato) resolveSeriesURL() error {
	if b.slug != "" {
		return nil
	}
	u, err := url.Parse(b.URL)
	if err != nil {
		return err
	}
	m := bbatoPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return errSeriesNotFound(b.URL)
	}
	b.slug = m[1]
	b.URL = b.BaseUrl() + "/manga/" + b.slug

	return nil
}

// FetchTitle fetches and returns the manga title
func (b *Bbato) FetchTitle() (string, error) {
	if err := b.resolveSeriesURL(); err != nil {
		return "", err
	}
	doc, err := bbatoFetch(b.URL, b.BaseUrl())
	if err != nil {
		return "", err
	}
	title := sanitizeTitle(doc.Find(`h1[itemprop="name"]`).First().Text())
	if title == "" {
		return "", fmt.Errorf("bbato: no title found in %s", b.URL)
	}

	return title, nil
}

// bbatoFetch returns the document at uri
func bbatoFetch(uri, referer string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: referer})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// FetchChapters returns the chapters of the manga
func (b *Bbato) FetchChapters() (chapters Filterables, errs []error) {
	if err := b.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}
	body, err := http.GetText(http.RequestParams{
		URL:     b.BaseUrl() + "/get-chapter-list?slug=" + url.QueryEscape(b.slug),
		Referer: b.URL,
		Accept:  "application/json",
		Headers: map[string]string{"X-Requested-With": "XMLHttpRequest"},
	})
	if err != nil {
		return nil, []error{err}
	}
	list := bbatoChapterList{}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return nil, []error{fmt.Errorf("bbato: chapter list: %w", err)}
	}

	for _, c := range list.Data {
		number := c.Number
		if number == nil {
			n, ok := parseChapterNumber(c.Name)
			if !ok {
				errs = append(errs, fmt.Errorf("bbato: no chapter number in %q", c.Name))
				continue
			}
			number = &n
		}
		chapters = append(chapters, &BbatoChapter{
			Chapter: Chapter{
				Number:   *number,
				Title:    bbatoNumberPrefixRe.ReplaceAllString(sanitizeTitle(c.Name), ""),
				Label:    Label{Style: LabelNumbered},
				Language: "en",
				ID:       c.Slug,
			},
			Slug: c.Slug,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (b *Bbato) FetchChapter(f Filterable) (*Chapter, error) {
	bchap := f.(*BbatoChapter)
	uri := b.BaseUrl() + "/read/" + b.slug + "/" + url.PathEscape(bchap.Slug)
	doc, err := bbatoFetch(uri, b.URL)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: bchap.Language,
		ID:       bchap.ID,
	}
	for i, img := range bbatoPages(doc, uri) {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// bbatoPages returns a reader's image URLs, the banner page left out
func bbatoPages(doc *goquery.Document, chapterURL string) (imgs []string) {
	base, _ := url.Parse(chapterURL)
	doc.Find(".pages .page:not(.notice-page) img").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("data-src", ""))
		if src == "" {
			src = strings.TrimSpace(s.AttrOr("src", ""))
		}
		if img := resolveURLRelative(src, base); img != "" && !strings.HasPrefix(img, "data:") {
			imgs = append(imgs, img)
		}
	})

	return
}

// bbatoChapterList is the /get-chapter-list answer
type bbatoChapterList struct {
	Data []struct {
		Name   string   `json:"chapter_name"`
		Slug   string   `json:"chapter_slug"`
		Number *float64 `json:"chapter_num"`
	} `json:"data"`
}
