// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

var (
	xlecxHostRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?xlecx\.one(?:[/?#]|$)`)
	xlecxPostRe = regexp.MustCompile(`^/\d+-[^/]*\.html$`)
	// xlecxPictureRe tells a link to a picture from one to another page
	xlecxPictureRe = regexp.MustCompile(`(?i)\.(?:jpe?g|png|gif|webp|avif)(?:\?|$)`)
)

// Xlecx is a grabber for xlecx.one, a DataLife Engine comics gallery. A post
// (/{id}-{slug}.html) is one gallery, so it's a series with one chapter. The
// pictures sit in one of a few places depending on how the post was made: a
// "full size" tab, the post's text (inline pictures, and thumbnails linking to
// their full size), or a "thumb" tab, tried in that order.
type Xlecx struct {
	*Grabber
	doc *goquery.Document
}

func NewXlecx(g *Grabber) *Xlecx {
	return &Xlecx{Grabber: g}
}

// XlecxChapter is the gallery, the only chapter of its series
type XlecxChapter struct {
	Chapter
}

// Test returns true for xlecx.one gallery posts
func (x *Xlecx) Test() (bool, error) {
	if !xlecxHostRe.MatchString(x.URL) {
		return false, nil
	}
	u, err := url.Parse(x.URL)
	if err != nil {
		return false, nil
	}

	return xlecxPostRe.MatchString(u.Path), nil
}

func (x *Xlecx) fetchDoc() (*goquery.Document, error) {
	if x.doc != nil {
		return x.doc, nil
	}
	body, err := http.Get(http.RequestParams{URL: x.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	x.doc = doc

	return doc, nil
}

// FetchTitle returns the gallery title
func (x *Xlecx) FetchTitle() (string, error) {
	doc, err := x.fetchDoc()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(doc.Find("h1").First().Text()), nil
}

// FetchChapters returns the gallery as the only chapter
func (x *Xlecx) FetchChapters() (Filterables, []error) {
	if _, err := x.fetchDoc(); err != nil {
		return nil, []error{err}
	}
	u, _ := url.Parse(x.URL)

	return Filterables{&XlecxChapter{Chapter{Number: 1, ID: u.Path, Language: "en", Label: Label{Style: LabelUnnumbered}}}}, nil
}

// FetchChapter lists the gallery's pictures
func (x *Xlecx) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := x.fetchDoc()
	if err != nil {
		return nil, err
	}
	urls := xlecxImages(doc, x.BaseUrl())
	if len(urls) == 0 {
		return nil, errors.New("the gallery shows no pictures")
	}

	chapter := &Chapter{Title: f.GetTitle(), Number: f.GetNumber(), Language: "en", ID: f.(*XlecxChapter).ID, Label: labelOf(f)}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// xlecxImages returns the post's pictures from the first place that has any
func xlecxImages(doc *goquery.Document, base string) []string {
	imgSrc := func(s *goquery.Selection) string {
		if src := strings.TrimSpace(s.AttrOr("data-src", "")); src != "" {
			return src
		}
		return strings.TrimSpace(s.AttrOr("src", ""))
	}
	// a thumbnail links to its full-size picture; other pictures are inline
	postImage := func(s *goquery.Selection) string {
		if href := s.ParentFiltered("a").AttrOr("href", ""); xlecxPictureRe.MatchString(href) {
			return href
		}
		return imgSrc(s)
	}
	sources := []func() []string{
		func() []string { return xlecxCollect(doc.Find("#content-2 > .imagegall23 > img"), imgSrc) },
		func() []string { return xlecxCollect(doc.Find(".page__text img"), postImage) },
		func() []string { return xlecxCollect(doc.Find("#content-1 > .imagegall23 > img"), imgSrc) },
	}
	for _, source := range sources {
		if found := source(); len(found) > 0 {
			return xlecxAbsolute(base, found)
		}
	}

	return nil
}

func xlecxCollect(sel *goquery.Selection, attr func(*goquery.Selection) string) (urls []string) {
	sel.Each(func(_ int, s *goquery.Selection) {
		if u := attr(s); u != "" {
			urls = append(urls, u)
		}
	})

	return urls
}

// xlecxAbsolute resolves page-relative links against base
func xlecxAbsolute(base string, links []string) []string {
	b, err := url.Parse(base + "/")
	if err != nil {
		return links
	}
	out := make([]string, 0, len(links))
	for _, l := range links {
		if u, err := b.Parse(l); err == nil {
			out = append(out, u.String())
		}
	}

	return out
}
