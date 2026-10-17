// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// MyHentai is a grabber for myhentaigallery.com and myhentaicomics.com, two
// sites of one owner sharing a CDN. A gallery is one work, listed as a single
// chapter. Its page shows a thumbnail of every page, and each page's picture
// is the thumbnail's path with /original/ in place of /thumbnail/.
type MyHentai struct {
	*Grabber
	id  string
	doc *goquery.Document
}

// MyHentaiChapter is the gallery, as the only chapter
type MyHentaiChapter struct {
	Chapter
	URL string
}

func NewMyHentai(g *Grabber) *MyHentai {
	return &MyHentai{Grabber: g}
}

var (
	// gallery pages are /a/{id} and /gallery/thumbnails/{id}, readers
	// /a/{id}/{n} and /gallery/show/{id}/{n}
	myHentaiURLRe   = regexp.MustCompile(`^https?://(?:www\.)?(myhentaigallery\.com/a|myhentaicomics\.com/gallery/(?:thumbnails|show))/(\d+)(?:/|$)`)
	myHentaiPagesRe = regexp.MustCompile(`Pages:\s*(\d+)`)
)

// Test returns true for a gallery or reader URL of either site, which it
// rewrites to the gallery page
func (m *MyHentai) Test() (bool, error) {
	match := myHentaiURLRe.FindStringSubmatch(m.URL)
	if match == nil {
		return false, nil
	}
	m.id = match[2]
	path := strings.Replace(match[1], "/gallery/show", "/gallery/thumbnails", 1)
	m.URL = "https://" + path + "/" + m.id
	return true, nil
}

func (m *MyHentai) page() (*goquery.Document, error) {
	if m.doc != nil {
		return m.doc, nil
	}
	body, err := http.Get(http.RequestParams{URL: m.URL, Referer: m.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	m.doc, err = goquery.NewDocumentFromReader(body)
	return m.doc, err
}

// FetchTitle returns the gallery's title
func (m *MyHentai) FetchTitle() (string, error) {
	doc, err := m.page()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find("h1").First().Text()), nil
}

// FetchChapters returns the gallery as its only chapter
func (m *MyHentai) FetchChapters() (Filterables, []error) {
	if _, err := m.page(); err != nil {
		return nil, []error{err}
	}
	return Filterables{&MyHentaiChapter{
		Chapter: Chapter{Number: 1, Label: Label{Style: LabelNumbered}, ID: m.id},
		URL:     m.URL,
	}}, nil
}

// FetchChapter lists the gallery's pages, checking them against the page
// count the gallery states
func (m *MyHentai) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := m.page()
	if err != nil {
		return nil, err
	}
	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Label:    labelOf(f),
		ID:       m.id,
		Language: "en",
	}
	doc.Find("ul.comics-grid li.item .comic-thumb img").Each(func(i int, s *goquery.Selection) {
		src := strings.Replace(s.AttrOr("src", ""), "/thumbnail/", "/original/", 1)
		if src == "" {
			return
		}
		// the paths hold spaces
		if u, err := url.Parse(src); err == nil {
			src = u.String()
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: src})
	})
	if match := myHentaiPagesRe.FindStringSubmatch(doc.Text()); match != nil {
		if want, _ := strconv.Atoi(match[1]); want != len(chapter.Pages) {
			return nil, fmt.Errorf("the gallery lists %d of its %d pages", len(chapter.Pages), want)
		}
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}
