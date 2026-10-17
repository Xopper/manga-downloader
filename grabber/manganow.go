// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// manganowHosts are the sites running this MangaReader clone. Matched by
// domain because PlainHTML would otherwise claim the series page (its
// chapter rows are plain links) and find no pictures in the reader.
var manganowHosts = []string{"manganow.to"}

// Manganow is a grabber for the MangaReader clone at manganow.to: the series
// page lists every chapter per language (ul#en-chapters), and a reader's
// pages come from an AJAX call on its reading id. The pictures are
// hotlinked from other sites that refuse manganow's Referer.
type Manganow struct {
	*Grabber
	doc *goquery.Document
}

// NewManganow returns a new Manganow grabber
func NewManganow(g *Grabber) *Manganow {
	return &Manganow{Grabber: g}
}

// ManganowChapter is a Manganow chapter
type ManganowChapter struct {
	Chapter
	URL string
}

// Test reports whether the URL is one of manganowHosts'
func (m *Manganow) Test() (bool, error) {
	return slices.ContainsFunc(manganowHosts, func(host string) bool {
		return hostMatchesDomain(m.URL, host)
	}), nil
}

// load fetches the series page once, from a chapter URL's series too
// (/manga/{slug}/chapter-N)
func (m *Manganow) load() error {
	if m.doc != nil {
		return nil
	}
	if i := strings.Index(m.URL, "/chapter-"); i > 0 {
		m.URL = m.URL[:i]
	}
	body, err := http.Get(http.RequestParams{URL: m.URL})
	if err != nil {
		return err
	}
	defer body.Close()
	m.doc, err = goquery.NewDocumentFromReader(body)
	return err
}

// FetchTitle returns the series title
func (m *Manganow) FetchTitle() (string, error) {
	if err := m.load(); err != nil {
		return "", err
	}
	title := sanitizeTitle(m.doc.Find("h2.manga-name").First().Text())
	if title == "" {
		return "", errors.New("series title not found")
	}
	return title, nil
}

// FetchChapters returns the chapters of the preferred language's list
// (English by default, else the first list)
func (m *Manganow) FetchChapters() (Filterables, []error) {
	if err := m.load(); err != nil {
		return nil, []error{err}
	}
	lang := m.Settings.Language
	if lang == "" || lang == "all" {
		lang = "en"
	}
	list := m.doc.Find("ul#" + lang + "-chapters")
	if list.Length() == 0 {
		list = m.doc.Find("ul.lang-chapters").First()
		lang = strings.TrimSuffix(list.AttrOr("id", ""), "-chapters")
	}

	var chapters Filterables
	list.Find("li.chapter-item").Each(func(_ int, s *goquery.Selection) {
		link := s.Find("a.item-link")
		n, err := strconv.ParseFloat(s.AttrOr("data-number", ""), 64)
		if err != nil {
			var ok bool
			if n, ok = parseChapterNumber(link.AttrOr("title", "")); !ok {
				return
			}
		}
		u := link.AttrOr("href", "")
		chapters = append(chapters, &ManganowChapter{
			Chapter: Chapter{Number: n, Language: lang, ID: chapterURLID(u), Label: Label{Style: LabelNumbered}},
			URL:     u,
		})
	})
	if len(chapters) == 0 {
		return nil, []error{fmt.Errorf("no chapters found for %s", m.URL)}
	}
	return chapters, nil
}

// FetchChapter reads the reader's id, then asks for its pages
func (m *Manganow) FetchChapter(f Filterable) (*Chapter, error) {
	mc := f.(*ManganowChapter)
	body, err := http.Get(http.RequestParams{URL: mc.URL, Referer: m.URL})
	if err != nil {
		return nil, err
	}
	reader, err := goquery.NewDocumentFromReader(body)
	body.Close()
	if err != nil {
		return nil, err
	}
	id := reader.Find("#wrapper[data-reading-id]").AttrOr("data-reading-id", "")
	if id == "" {
		return nil, fmt.Errorf("no reading id in %s", mc.URL)
	}

	text, err := http.GetText(http.RequestParams{
		URL:     m.BaseUrl() + "/ajax/image/list/" + id + "?mode=vertical&hozPageSize=1",
		Referer: mc.URL,
		Accept:  "application/json",
		Headers: map[string]string{"X-Requested-With": "XMLHttpRequest"},
	})
	if err != nil {
		return nil, err
	}
	urls, err := manganowPages(text, m.BaseUrl())
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   mc.Language,
		ID:         mc.ID,
		PagesCount: int64(len(urls)),
	}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u, NoReferer: true})
	}
	return chapter, nil
}

// manganowPages reads the image list's cards, leaving out the site's own
// banner card
func manganowPages(text, base string) ([]string, error) {
	var resp struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil, fmt.Errorf("reading the chapter's image list: %w", err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(resp.HTML))
	if err != nil {
		return nil, err
	}
	var urls []string
	doc.Find(".iv-card[data-url]").Each(func(_ int, s *goquery.Selection) {
		u := strings.TrimSpace(s.AttrOr("data-url", ""))
		if u != "" && !strings.HasPrefix(u, base+"/") {
			urls = append(urls, u)
		}
	})
	return urls, nil
}
