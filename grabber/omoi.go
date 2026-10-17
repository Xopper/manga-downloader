// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

// Omoi is a grabber for omoi.com, run on the Azuki platform: its API lists
// a series' chapters with their free window, and a free chapter's pages. The
// pictures come XORed with 0xAE, undone by each page's Transform.
type Omoi struct {
	*Grabber
	series *omoiSeries
}

// OmoiChapter is a chapter of the series
type OmoiChapter struct {
	Chapter
	UUID string
}

// omoiSeries is the part of the series answer used
type omoiSeries struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

var (
	// omoiAPI is Azuki's API, a var so tests can point it elsewhere
	omoiAPI = "https://production.api.azuki.co"
	// omoiOrganizationKey names omoi to the Azuki API, as its web app does
	omoiOrganizationKey = "199e5a19-a236-49f5-81f4-43d4a541748a"
	// omoiSlugRe captures the series slug of a /series/{slug} URL
	omoiSlugRe = regexp.MustCompile(`^/series/([^/]+)`)
	// omoiNumberRe reads the number a chapter label starts with ("247b")
	omoiNumberRe = regexp.MustCompile(`^\d+(?:\.\d+)?`)
)

// omoiXOR is the byte every picture byte is XORed with
const omoiXOR = 0xAE

func NewOmoi(g *Grabber) *Omoi {
	return &Omoi{Grabber: g}
}

// Test returns true for an omoi.com series or chapter URL
func (o *Omoi) Test() (bool, error) {
	return hostMatchesDomain(o.URL, "omoi.com") && o.slug() != "", nil
}

// slug returns the URL's series slug
func (o Omoi) slug() string {
	u, err := url.Parse(o.URL)
	if err != nil {
		return ""
	}
	matches := omoiSlugRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return ""
	}
	return matches[1]
}

// getJSON fetches an API path into v
func (o Omoi) getJSON(path string, v any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     omoiAPI + path,
		Referer: o.BaseUrl(),
		Origin:  o.BaseUrl(),
		Accept:  "application/json",
		Headers: map[string]string{"azuki-organization-key": omoiOrganizationKey},
	})
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), v)
}

// fetchSeries fetches the series once
func (o *Omoi) fetchSeries() (*omoiSeries, error) {
	if o.series == nil {
		series := &omoiSeries{}
		if err := o.getJSON("/manga/slug/"+url.PathEscape(o.slug())+"/v0", series); err != nil {
			return nil, err
		}
		o.series = series
	}
	return o.series, nil
}

// FetchTitle returns the series name
func (o *Omoi) FetchTitle() (string, error) {
	series, err := o.fetchSeries()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(series.Name), nil
}

// FetchChapters returns the chapters oldest first, locked unless their free
// window is open now
func (o *Omoi) FetchChapters() (Filterables, []error) {
	series, err := o.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}
	var feed struct {
		Chapters []struct {
			UUID        string     `json:"uuid"`
			OrderNumber float64    `json:"order_number"`
			Label       string     `json:"label"`
			Title       *string    `json:"title"`
			FreeFrom    *time.Time `json:"free_published_date"`
			FreeUntil   *time.Time `json:"free_unpublished_date"`
			IsUpcoming  bool       `json:"is_upcoming"`
		} `json:"chapters"`
	}
	if err := o.getJSON("/mangas/"+series.UUID+"/chapters/v4?order=ascending&count=1000", &feed); err != nil {
		return nil, []error{err}
	}

	now := time.Now()
	chapters := make(Filterables, 0, len(feed.Chapters))
	for _, c := range feed.Chapters {
		number := c.OrderNumber / 100
		numberText := ""
		if match := omoiNumberRe.FindString(c.Label); match != "" {
			number, _ = strconv.ParseFloat(match, 64)
			numberText = c.Label
		}
		title := ""
		if c.Title != nil {
			title = *c.Title
		}
		free := c.FreeFrom != nil && !c.FreeFrom.After(now) && (c.FreeUntil == nil || c.FreeUntil.After(now)) && !c.IsUpcoming
		chapters = append(chapters, &OmoiChapter{
			Chapter: Chapter{
				Number: number,
				Title:  title,
				ID:     c.UUID,
				Locked: !free,
				Label:  Label{Style: LabelNumbered, Number: numberText},
			},
			UUID: c.UUID,
		})
	}
	return chapters, nil
}

// FetchChapter returns the chapter's pages at their largest size
func (o *Omoi) FetchChapter(f Filterable) (*Chapter, error) {
	ochap := f.(*OmoiChapter)
	var feed struct {
		Data struct {
			Pages []struct {
				Image struct {
					WebP []struct {
						URL   string `json:"url"`
						Width int    `json:"width"`
					} `json:"webp"`
				} `json:"image"`
			} `json:"pages"`
		} `json:"data"`
	}
	if err := o.getJSON("/chapters/"+ochap.UUID+"/pages/v1", &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "en",
		ID:       ochap.ID,
	}
	for _, p := range feed.Data.Pages {
		best, width := "", -1
		for _, w := range p.Image.WebP {
			if w.Width > width {
				best, width = w.URL, w.Width
			}
		}
		if best == "" {
			continue
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number:    int64(len(chapter.Pages) + 1),
			URL:       best,
			Transform: omoiDecode,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	if len(chapter.Pages) == 0 {
		return nil, fmt.Errorf("no pages in chapter %s", ochap.UUID)
	}
	return chapter, nil
}

// omoiDecode undoes the pictures' XOR
func omoiDecode(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ omoiXOR
	}
	return out, nil
}
