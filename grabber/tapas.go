// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Tapas is a grabber for tapas.io comics. The series page names the series'
// numeric ID (its app link, tapastic://series/{id}), and
// /series/{id}/episodes?sort=OLDEST answers JSON whose body is the episode
// list's HTML, 20 rows a page. Rows marked locked are paid or wait-for-free
// episodes a logged-out reader can't open. An episode's page holds its
// pictures, lazy-loaded.
type Tapas struct {
	*Grabber
	id    string
	title string
}

// TapasChapter is an episode of a Tapas series
type TapasChapter struct {
	Chapter
	URL string
}

// NewTapas returns a new Tapas grabber
func NewTapas(g *Grabber) *Tapas {
	return &Tapas{Grabber: g}
}

// Test tells whether the URL is a tapas.io series
func (t *Tapas) Test() (bool, error) {
	return hostMatchesDomain(t.URL, "tapas.io") && strings.Contains(t.URL, "/series/"), nil
}

// tapasBase is the site's root
var tapasBase = "https://tapas.io"

// tapasAppLinkRe reads the series' ID from its app link
var tapasAppLinkRe = regexp.MustCompile(`tapastic://series/(\d+)`)

// series reads the series page once, for its ID and title
func (t *Tapas) series() error {
	if t.id != "" {
		return nil
	}
	doc, err := wcGetDoc(t.URL, tapasBase+"/", false)
	if err != nil {
		return err
	}
	html, _ := doc.Html()
	m := tapasAppLinkRe.FindStringSubmatch(html)
	if m == nil {
		return errSeriesNotFound(t.URL)
	}
	t.id = m[1]
	t.title = sanitizeTitle(doc.Find(".center-info__title").First().Text())
	if t.title == "" {
		t.title = strings.TrimPrefix(smPageName(doc.Find("title").First().Text()), "Read ")
	}
	return nil
}

// FetchTitle returns the series' title
func (t *Tapas) FetchTitle() (string, error) {
	if err := t.series(); err != nil {
		return "", err
	}
	return t.title, nil
}

// tapasMaxPages bounds the episode list's pages
const tapasMaxPages = 200

// FetchChapters returns the series' episodes, oldest first
func (t *Tapas) FetchChapters() (Filterables, []error) {
	if err := t.series(); err != nil {
		return nil, []error{err}
	}
	var chapters Filterables
	for page := 1; page <= tapasMaxPages; page++ {
		body, err := http.GetText(http.RequestParams{
			URL:     fmt.Sprintf("%s/series/%s/episodes?page=%d&sort=OLDEST&max_limit=20", tapasBase, t.id, page),
			Referer: t.URL,
			Accept:  "application/json",
		})
		if err != nil {
			return chapters, []error{err}
		}
		var res struct {
			Data struct {
				Pagination struct {
					HasNext bool `json:"has_next"`
				} `json:"pagination"`
				Body string `json:"body"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			return chapters, []error{err}
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.Data.Body))
		if err != nil {
			return chapters, []error{err}
		}
		chapters = append(chapters, tapasChapters(doc, len(chapters))...)
		if !res.Data.Pagination.HasNext {
			break
		}
	}
	if len(chapters) == 0 {
		return nil, []error{errors.New("the series lists no episodes")}
	}
	return chapters, nil
}

// tapasChapters reads a page of the episode list. An episode is numbered by
// the number its title opens with ("1. Endurance"), else by its position
// after the before episodes listed already.
func tapasChapters(doc *goquery.Document, before int) Filterables {
	var chapters Filterables
	doc.Find("li[data-id][data-permalink]").Each(func(i int, row *goquery.Selection) {
		name := sanitizeTitle(row.Find(".info__title").First().Text())
		number, ok := wcNameNumber(name)
		if !ok {
			number = float64(before + i + 1)
		}
		ch := wcChapter(name, number)
		ch.ID = row.AttrOr("data-id", "")
		ch.Language = "en"
		ch.Locked = row.Find(".thumb__overlay--locked, .ico--lock").Length() > 0
		chapters = append(chapters, &TapasChapter{
			Chapter: ch,
			URL:     tapasBase + row.AttrOr("data-permalink", ""),
		})
	})
	return chapters
}

// FetchChapter fetches an episode's pictures
func (t *Tapas) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*TapasChapter)
	doc, err := wcGetDoc(ch.URL, t.URL, false)
	if err != nil {
		return nil, err
	}
	var pages []Page
	for _, src := range wcComicImages(doc, "img.content__img") {
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      ch.Title,
		Number:     ch.Number,
		Label:      ch.Label,
		ID:         ch.ID,
		PagesCount: int64(len(pages)),
		Language:   "en",
		Pages:      pages,
	}, nil
}
