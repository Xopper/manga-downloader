// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Manta is a grabber for manta.net, read through the API its site uses:
// /front/v1/series/{id} lists a series' episodes, and
// /front/v1/episodes/{id} gives an episode's pictures (signed links, cut
// into strips). An episode whose lock state isn't the free one (110) is paid
// or waits to be free, and is marked Locked.
type Manta struct {
	*Grabber
	id     string
	series *mantaSeries
}

// MantaChapter is an episode of a Manta series
type MantaChapter struct {
	Chapter
}

// NewManta returns a new Manta grabber
func NewManta(g *Grabber) *Manta {
	return &Manta{Grabber: g}
}

// mantaAPI is the site's API root
const mantaAPI = "https://manta.net/front/v1"

// mantaFree is the lock state of an episode anyone can read
const mantaFree = 110

// mantaSeriesRe reads a series' ID from its URL: /en/series/3429, or the
// seriesId query of /en/series/{slug}?seriesId=3429
var mantaSeriesRe = regexp.MustCompile(`(?:seriesId=|/series/)(\d+)\b`)

// mantaSeries is the part of a series the grabber reads
type mantaSeries struct {
	Data struct {
		Title map[string]string `json:"title"`
	} `json:"data"`
	Episodes []struct {
		ID   int     `json:"id"`
		Ord  float64 `json:"ord"`
		Data struct {
			Title string `json:"title"`
		} `json:"data"`
		LockData struct {
			State int `json:"state"`
		} `json:"lockData"`
	} `json:"episodes"`
}

// Test tells whether the URL is a manta.net series
func (m *Manta) Test() (bool, error) {
	if !hostMatchesDomain(m.URL, "manta.net") {
		return false, nil
	}
	match := mantaSeriesRe.FindStringSubmatch(m.URL)
	if match == nil {
		return false, nil
	}
	m.id = match[1]
	return true, nil
}

// get asks the API for path and decodes its data into v
func (m *Manta) get(path string, v any) error {
	body, err := http.GetText(http.RequestParams{URL: mantaAPI + path, Referer: "https://manta.net/", Accept: "application/json"})
	if err != nil {
		return err
	}
	var res struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return err
	}
	return json.Unmarshal(res.Data, v)
}

// load reads the series once
func (m *Manta) load() error {
	if m.series != nil {
		return nil
	}
	var s mantaSeries
	if err := m.get("/series/"+m.id+"?lang=en", &s); err != nil {
		return err
	}
	m.series = &s
	return nil
}

// FetchTitle returns the series' title, in English when it has one
func (m *Manta) FetchTitle() (string, error) {
	if err := m.load(); err != nil {
		return "", err
	}
	if t := m.series.Data.Title["en"]; t != "" {
		return t, nil
	}
	for _, t := range m.series.Data.Title {
		return t, nil
	}
	return "", fmt.Errorf("series %s has no title", m.id)
}

// FetchChapters returns the series' episodes, numbered by their order
func (m *Manta) FetchChapters() (Filterables, []error) {
	if err := m.load(); err != nil {
		return nil, []error{err}
	}
	var chapters Filterables
	for _, e := range m.series.Episodes {
		ch := wcChapter(sanitizeTitle(e.Data.Title), e.Ord)
		ch.ID = strconv.Itoa(e.ID)
		ch.Language = "en"
		ch.Locked = e.LockData.State != mantaFree
		chapters = append(chapters, &MantaChapter{Chapter: ch})
	}
	return chapters, nil
}

// FetchChapter fetches an episode's pictures
func (m *Manta) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*MantaChapter)
	var episode struct {
		CutImages []struct {
			DownloadURL string `json:"downloadUrl"`
		} `json:"cutImages"`
	}
	if err := m.get("/episodes/"+ch.ID, &episode); err != nil {
		return nil, err
	}
	var pages []Page
	for _, c := range episode.CutImages {
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: c.DownloadURL})
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
