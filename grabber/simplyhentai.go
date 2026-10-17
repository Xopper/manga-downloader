// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// SimplyHentai is a grabber for simply-hentai.com, whose galleries are one
// work each, listed as a single chapter. The gallery page links each page's
// reader by its id; any reader page's Next.js payload lists every page with
// its full size picture.
type SimplyHentai struct {
	*Grabber
	doc *goquery.Document
}

// SimplyHentaiChapter is the gallery, as the only chapter
type SimplyHentaiChapter struct {
	Chapter
}

var (
	// /{series}/{slug} and its readers /{series}/{slug}/page/{id}
	simplyHentaiURLRe  = regexp.MustCompile(`^(https?://(?:www\.)?simply-hentai\.com/[^/]+/[^/?#]+)`)
	simplyHentaiPageRe = regexp.MustCompile(`\\"pageNum\\":(\d+),\\"sizes\\":\{\\"full\\":\\"([^"\\]+)`)
)

func NewSimplyHentai(g *Grabber) *SimplyHentai {
	return &SimplyHentai{Grabber: g}
}

// Test returns true for a gallery or reader URL of simply-hentai.com, which
// it rewrites to the gallery page
func (s *SimplyHentai) Test() (bool, error) {
	m := simplyHentaiURLRe.FindStringSubmatch(s.URL)
	if m == nil {
		return false, nil
	}
	s.URL = m[1]
	return true, nil
}

func (s *SimplyHentai) fetch(u string) (*goquery.Document, string, error) {
	text, err := http.GetText(http.RequestParams{URL: u, Referer: s.BaseUrl()})
	if err != nil {
		return nil, "", err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(text))
	return doc, text, err
}

func (s *SimplyHentai) page() (*goquery.Document, error) {
	if s.doc == nil {
		doc, _, err := s.fetch(s.URL)
		if err != nil {
			return nil, err
		}
		s.doc = doc
	}
	return s.doc, nil
}

// FetchTitle returns the gallery's title
func (s *SimplyHentai) FetchTitle() (string, error) {
	doc, err := s.page()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find("h1").First().Text()), nil
}

// FetchChapters returns the gallery as its only chapter
func (s *SimplyHentai) FetchChapters() (Filterables, []error) {
	if _, err := s.page(); err != nil {
		return nil, []error{err}
	}
	return Filterables{&SimplyHentaiChapter{Chapter{Number: 1, Label: Label{Style: LabelNumbered}, ID: chapterURLID(s.URL)}}}, nil
}

// FetchChapter lists the pages from the first reader page's payload
func (s *SimplyHentai) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := s.page()
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(s.URL)
	reader := resolveURLRelative(doc.Find(`a[href*="/page/"]`).First().AttrOr("href", ""), base)
	if reader == "" {
		return nil, errors.New("the gallery links no reader page")
	}
	_, text, err := s.fetch(reader)
	if err != nil {
		return nil, err
	}
	type picture struct {
		num int
		url string
	}
	var pics []picture
	seen := map[int]bool{}
	for _, m := range simplyHentaiPageRe.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[1])
		if !seen[n] {
			seen[n] = true
			pics = append(pics, picture{n, m[2]})
		}
	}
	if len(pics) == 0 {
		return nil, errors.New("no pictures found in the reader")
	}
	slices.SortFunc(pics, func(a, b picture) int { return a.num - b.num })
	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Label:      labelOf(f),
		ID:         chapterURLID(s.URL),
		PagesCount: int64(len(pics)),
	}
	for _, p := range pics {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(p.num), URL: p.url})
	}
	return chapter, nil
}
