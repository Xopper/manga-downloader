// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// MangaPlusCreators is a grabber for mangaplus-creators.jp (Shueisha's
// free platform for amateur manga): a title's page lists its episodes, a few
// per ?page=N, and an episode's viewer carries its pages as JSON in a
// data-pages attribute
type MangaPlusCreators struct {
	*Grabber
	doc *goquery.Document
}

// MangaPlusCreatorsChapter is an episode
type MangaPlusCreatorsChapter struct {
	Chapter
	URL string
}

var (
	// mangapluscreatorsTitleRe captures a title's id
	mangapluscreatorsTitleRe = regexp.MustCompile(`^/titles/([0-9a-z]+)`)
	// mangapluscreatorsNumberRe reads an episode number ("#001")
	mangapluscreatorsNumberRe = regexp.MustCompile(`#\s*(\d+(?:\.\d+)?)`)
)

// mangapluscreatorsMaxPages bounds the episode list walk
const mangapluscreatorsMaxPages = 100

func NewMangaPlusCreators(g *Grabber) *MangaPlusCreators {
	return &MangaPlusCreators{Grabber: g}
}

// Test returns true for a mangaplus-creators.jp title URL
func (m *MangaPlusCreators) Test() (bool, error) {
	return hostMatchesDomain(m.URL, "mangaplus-creators.jp") && m.titleID() != "", nil
}

// titleID returns the URL's title id
func (m MangaPlusCreators) titleID() string {
	u, err := url.Parse(m.URL)
	if err != nil {
		return ""
	}
	matches := mangapluscreatorsTitleRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return ""
	}
	return matches[1]
}

// fetchPage fetches one page of the title's episode list
func (m MangaPlusCreators) fetchPage(n int) (*goquery.Document, error) {
	uri := m.BaseUrl() + "/titles/" + m.titleID()
	if n > 1 {
		uri += "?page=" + strconv.Itoa(n)
	}
	body, err := http.Get(http.RequestParams{URL: uri})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

// firstPage fetches the title's page once
func (m *MangaPlusCreators) firstPage() (*goquery.Document, error) {
	if m.doc == nil {
		doc, err := m.fetchPage(1)
		if err != nil {
			return nil, err
		}
		m.doc = doc
	}
	return m.doc, nil
}

// FetchTitle returns the title's name
func (m *MangaPlusCreators) FetchTitle() (string, error) {
	doc, err := m.firstPage()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find(".book-box div.title").First().Text()), nil
}

// FetchChapters walks the episode list pages; a one-shot (no number) is
// numbered by its position
func (m *MangaPlusCreators) FetchChapters() (Filterables, []error) {
	doc, err := m.firstPage()
	if err != nil {
		return nil, []error{err}
	}
	chapters := Filterables{}
	for n := 1; n <= mangapluscreatorsMaxPages; n++ {
		if n > 1 {
			if doc, err = m.fetchPage(n); err != nil {
				return chapters, []error{err}
			}
		}
		doc.Find(`a.mod-item-series[href*="/episodes/"]`).Each(func(_ int, a *goquery.Selection) {
			text := strings.TrimSpace(a.Find(".number").First().Text())
			number := float64(len(chapters) + 1)
			title := text
			if match := mangapluscreatorsNumberRe.FindStringSubmatch(text); match != nil {
				number, _ = strconv.ParseFloat(match[1], 64)
				title = ""
			}
			href, _ := url.Parse(a.AttrOr("href", ""))
			base, _ := url.Parse(m.BaseUrl())
			uri := base.ResolveReference(href).String()
			chapters = append(chapters, &MangaPlusCreatorsChapter{
				Chapter: Chapter{
					Number: number,
					Title:  title,
					ID:     strings.TrimPrefix(href.Path, "/episodes/"),
					Label:  Label{Style: LabelNumbered},
				},
				URL: uri,
			})
		})
		if doc.Find(".mod-pagination .next").Length() == 0 {
			break
		}
	}
	return chapters, nil
}

// FetchChapter reads the episode viewer's data-pages JSON
func (m *MangaPlusCreators) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaPlusCreatorsChapter)
	body, err := http.Get(http.RequestParams{URL: mchap.URL, Referer: m.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	raw, ok := doc.Find(`[react="viewer"][data-pages]`).First().Attr("data-pages")
	if !ok {
		return nil, fmt.Errorf("no viewer pages in %s", mchap.URL)
	}
	var pages struct {
		PC []struct {
			PageNo   int    `json:"page_no"`
			ImageURL string `json:"image_url"`
		} `json:"pc"`
	}
	if err := json.Unmarshal([]byte(raw), &pages); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		ID:         mchap.ID,
		PagesCount: int64(len(pages.PC)),
	}
	for i, p := range pages.PC {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: p.ImageURL})
	}
	return chapter, nil
}
