// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// ThreeHentai is a grabber for 3hentai.net, a doujinshi gallery site: each
// gallery (/d/{id}) is one chapter, and its page lists a thumbnail of every
// page. The full picture sits next to its thumbnail (/d{id}/2t.jpg is
// /d{id}/2.jpg), so the reader pages (/d/{id}/{n}, one picture each) are never
// fetched.
type ThreeHentai struct {
	*Grabber
	doc *goquery.Document
}

// ThreeHentaiChapter is a 3hentai gallery
type ThreeHentaiChapter struct {
	Chapter
}

func NewThreeHentai(g *Grabber) *ThreeHentai {
	return &ThreeHentai{Grabber: g}
}

var (
	threeHentaiGalleryRe = regexp.MustCompile(`^(https?://[^/]+/d/\d+)(?:/|$|[?#])`)
	threeHentaiThumbRe   = regexp.MustCompile(`/(\d+)t(\.\w+)$`)
)

// Test matches 3hentai.net gallery and reader URLs, reducing a reader URL
// (/d/{id}/{page}) to its gallery
func (h *ThreeHentai) Test() (bool, error) {
	if !hostMatchesDomain(h.URL, "3hentai.net") {
		return false, nil
	}
	m := threeHentaiGalleryRe.FindStringSubmatch(h.URL)
	if m == nil {
		return false, nil
	}
	h.URL = m[1]
	return true, nil
}

func (h *ThreeHentai) fetch() error {
	if h.doc != nil {
		return nil
	}
	body, err := http.Get(http.RequestParams{URL: h.URL})
	if err != nil {
		return err
	}
	defer body.Close()
	h.doc, err = goquery.NewDocumentFromReader(body)
	return err
}

// FetchTitle returns the gallery's title
func (h *ThreeHentai) FetchTitle() (string, error) {
	if err := h.fetch(); err != nil {
		return "", err
	}
	title := sanitizeTitle(h.doc.Find("h1").First().Text())
	if title == "" {
		return "", errors.New("could not find the gallery title")
	}
	return title, nil
}

// FetchChapters returns the gallery as its only chapter
func (h *ThreeHentai) FetchChapters() (Filterables, []error) {
	if err := h.fetch(); err != nil {
		return nil, []error{err}
	}
	return Filterables{&ThreeHentaiChapter{Chapter{
		Number: 1,
		ID:     chapterURLID(h.URL),
		Label:  Label{Style: LabelNumbered},
	}}}, nil
}

// FetchChapter turns the gallery's thumbnails into its pages
func (h *ThreeHentai) FetchChapter(f Filterable) (*Chapter, error) {
	if err := h.fetch(); err != nil {
		return nil, err
	}
	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		ID:       chapterURLID(h.URL),
		Language: "en",
	}
	for _, u := range threeHentaiPages(h.doc) {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    u,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	if chapter.PagesCount == 0 {
		return nil, errors.New("no pages found in the gallery")
	}
	return chapter, nil
}

// threeHentaiPages returns the full pictures of a gallery page's thumbnails
func threeHentaiPages(doc *goquery.Document) []string {
	var urls []string
	doc.Find("#thumbnail-gallery .single-thumb img").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", "")))
		if !threeHentaiThumbRe.MatchString(src) {
			return
		}
		urls = append(urls, threeHentaiThumbRe.ReplaceAllString(src, "/${1}${2}"))
	})
	return urls
}
