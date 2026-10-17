// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// monochromeDomains are the hosts running MonochromeCMS (Tachiyomi's
// monochrome multisrc); its API lives on the same host with an "api."
// prefix. The CMS is self-hosted, but no other public instance is known.
var monochromeDomains = []string{"manga.d34d.one"}

// monochromeURLRe captures a series (/manga/{uuid}) or chapter
// (/chapters/{uuid}) link of the Vue app
var monochromeURLRe = regexp.MustCompile(`(?i)^/(manga|chapters)/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/?$`)

// Monochrome is a grabber for MonochromeCMS, the Vue reader Monochrome Scans
// runs. The site is an empty app shell, so everything comes from its JSON
// API: /manga/{id} and /manga/{id}/chapters list the series, and a page is
// /media/{manga}/{chapter}/{n}.jpg?version={v}, the version busting the
// cache when a chapter is re-uploaded.
type Monochrome struct {
	*Grabber
	mangaID string
}

// MonochromeChapter is a chapter of the API listing
type MonochromeChapter struct {
	Chapter
	MangaID string
	Version int
	Length  int
}

// monochromeChapterDTO is a chapter as the API returns it
type monochromeChapterDTO struct {
	Name      string  `json:"name"`
	Volume    *int    `json:"volume"`
	Number    float64 `json:"number"`
	ScanGroup string  `json:"scanGroup"`
	ID        string  `json:"id"`
	Version   int     `json:"version"`
	MangaID   string  `json:"mangaId"`
	Length    int     `json:"length"`
}

// NewMonochrome returns a new Monochrome grabber
func NewMonochrome(g *Grabber) *Monochrome {
	return &Monochrome{Grabber: g}
}

// Test matches the known MonochromeCMS hosts
func (m *Monochrome) Test() (bool, error) {
	for _, d := range monochromeDomains {
		if hostMatchesDomain(m.URL, d) {
			return true, nil
		}
	}
	return false, nil
}

// apiURL is the API base: the site's host with an "api." prefix
func (m *Monochrome) apiURL() string {
	u, _ := url.Parse(m.URL)
	return "https://api." + strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// getJSON reads one API answer
func (m *Monochrome) getJSON(path string, v any) error {
	body, err := http.Get(http.RequestParams{URL: m.apiURL() + path, Referer: m.BaseUrl(), Accept: "application/json"})
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(v)
}

// resolveSeriesURL finds the series id: in a series URL directly, for a
// chapter URL from the API's /chapter/{id}, which names its manga
func (m *Monochrome) resolveSeriesURL() (string, error) {
	if m.mangaID != "" {
		return m.mangaID, nil
	}
	kind, id, ok := monochromeParseURL(m.URL)
	if !ok {
		return "", errSeriesNotFound(m.URL)
	}
	if kind == "chapters" {
		ch := &monochromeChapterDTO{}
		if err := m.getJSON("/chapter/"+id, ch); err != nil {
			return "", err
		}
		if ch.MangaID == "" {
			return "", errSeriesNotFound(m.URL)
		}
		id = ch.MangaID
	}
	m.mangaID = id
	return id, nil
}

// monochromeParseURL returns whether a URL is a "manga" or a "chapters" link,
// and its id
func monochromeParseURL(rawURL string) (kind, id string, ok bool) {
	_, path, ok := chapterPath(rawURL)
	if !ok {
		return "", "", false
	}
	mm := monochromeURLRe.FindStringSubmatch(path)
	if mm == nil {
		return "", "", false
	}
	return strings.ToLower(mm[1]), strings.ToLower(mm[2]), true
}

// FetchTitle returns the series title
func (m *Monochrome) FetchTitle() (string, error) {
	id, err := m.resolveSeriesURL()
	if err != nil {
		return "", err
	}
	var manga struct {
		Title string `json:"title"`
	}
	if err := m.getJSON("/manga/"+id, &manga); err != nil {
		return "", err
	}
	return sanitizeTitle(manga.Title), nil
}

// FetchChapters returns the series' chapters
func (m *Monochrome) FetchChapters() (Filterables, []error) {
	id, err := m.resolveSeriesURL()
	if err != nil {
		return nil, []error{err}
	}
	var rows []monochromeChapterDTO
	if err := m.getJSON("/manga/"+id+"/chapters", &rows); err != nil {
		return nil, []error{err}
	}
	return monochromeChapters(rows, id), nil
}

// monochromeChapters turns the API rows into chapters, one per upload, with
// the uploading group and the volume when the site files one
func monochromeChapters(rows []monochromeChapterDTO, mangaID string) Filterables {
	chapters := make(Filterables, 0, len(rows))
	for _, r := range rows {
		c := &MonochromeChapter{
			Chapter: Chapter{
				Number:   r.Number,
				Title:    sanitizeTitle(r.Name),
				Language: "en",
				ID:       r.ID,
				Label:    Label{Style: LabelPrefixed, Number: strconv.FormatFloat(r.Number, 'f', -1, 64)},
			},
			MangaID: mangaID,
			Version: r.Version,
			Length:  r.Length,
		}
		if r.Volume != nil {
			c.Volume = *r.Volume
		}
		c.SetGroups(r.ScanGroup)
		chapters = append(chapters, c)
	}
	return chapters
}

// FetchChapter builds the page URLs from the chapter's length and version,
// with no request
func (m *Monochrome) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MonochromeChapter)
	pages := monochromePages(m.apiURL(), mchap)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		ID:         mchap.ID,
		Group:      mchap.Group,
		Groups:     mchap.Groups,
		Volume:     mchap.Volume,
		Pages:      pages,
	}, nil
}

// monochromePages lists a chapter's /media pages, numbered from 1
func monochromePages(api string, c *MonochromeChapter) []Page {
	pages := make([]Page, 0, c.Length)
	for i := 1; i <= c.Length; i++ {
		pages = append(pages, Page{
			Number: int64(i),
			URL:    fmt.Sprintf("%s/media/%s/%s/%d.jpg?version=%d", api, c.MangaID, c.ID, i, c.Version),
		})
	}
	return pages
}
