// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// MangAdventure is a grabber for sites running the MangAdventure reader
// (assortedscans.com, arc-relight.com): the page says so in its generator
// meta, and its JSON API (/api/v2) lists a series' chapters in one call and a
// chapter's pages in another
type MangAdventure struct {
	*Grabber
	title string
}

// MangAdventureChapter is a chapter listed by the series API
type MangAdventureChapter struct {
	Chapter
	APIID int64
}

// mangadventureReaderRe captures the series slug of a /reader/{slug}/ URL,
// series or chapter (/reader/{slug}/{volume}/{number}/)
var mangadventureReaderRe = regexp.MustCompile(`^/reader/([^/]+)`)

func NewMangAdventure(g *Grabber) *MangAdventure {
	return &MangAdventure{Grabber: g}
}

// Test fetches /reader/ URLs and accepts the page whose generator meta names
// MangAdventure, so every instance works without a domain list
func (m *MangAdventure) Test() (bool, error) {
	if m.slug() == "" {
		return false, nil
	}
	body, err := http.Get(http.RequestParams{URL: m.URL, DisableRetry: true})
	if err != nil {
		return false, nil
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return false, nil
	}
	generator := doc.Find(`meta[name="generator"]`).AttrOr("content", "")

	return strings.HasPrefix(generator, "MangAdventure"), nil
}

// slug returns the series slug of the URL, "" when it isn't a reader URL
func (m MangAdventure) slug() string {
	u, err := url.Parse(m.URL)
	if err != nil {
		return ""
	}
	matches := mangadventureReaderRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return ""
	}
	return matches[1]
}

// FetchTitle fetches and returns the series title
func (m *MangAdventure) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}
	series := struct {
		Title string `json:"title"`
	}{}
	if err := m.getJSON("/api/v2/series/"+url.PathEscape(m.slug()), &series); err != nil {
		return "", err
	}
	m.title = sanitizeTitle(series.Title)

	return m.title, nil
}

// FetchChapters returns the series' chapters, which the API lists unpaginated
func (m MangAdventure) FetchChapters() (Filterables, []error) {
	list := struct {
		Results []struct {
			ID     int64    `json:"id"`
			Title  string   `json:"title"`
			Number float64  `json:"number"`
			Volume *int     `json:"volume"`
			Groups []string `json:"groups"`
		} `json:"results"`
	}{}
	if err := m.getJSON("/api/v2/series/"+url.PathEscape(m.slug())+"/chapters", &list); err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(list.Results))
	for _, c := range list.Results {
		chapter := &MangAdventureChapter{
			Chapter: Chapter{
				Number: c.Number,
				Title:  c.Title,
				ID:     fmt.Sprint(c.ID),
				Label:  Label{Style: LabelNumbered},
			},
			APIID: c.ID,
		}
		if c.Volume != nil {
			chapter.Volume = *c.Volume
		}
		chapter.SetGroups(c.Groups...)
		chapters = append(chapters, chapter)
	}

	return chapters, nil
}

// FetchChapter fetches the chapter's pages, in the order the API numbers them
func (m MangAdventure) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangAdventureChapter)
	list := struct {
		Results []struct {
			Image  string `json:"image"`
			Number int64  `json:"number"`
		} `json:"results"`
	}{}
	if err := m.getJSON(fmt.Sprintf("/api/v2/chapters/%d/pages", mchap.APIID), &list); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Volume:     mchap.Volume,
		PagesCount: int64(len(list.Results)),
		Language:   "en",
		ID:         mchap.ID,
	}
	chapter.SetGroups(mchap.GetGroups()...)
	for i, p := range list.Results {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    p.Image,
		})
	}

	return chapter, nil
}

// getJSON fetches an API path of the site and decodes it into v
func (m MangAdventure) getJSON(path string, v any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     m.BaseUrl() + path,
		Referer: m.URL,
		Accept:  "application/json",
	})
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(body), v)
}
