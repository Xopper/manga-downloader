// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// HeanCMS is a grabber for the HeanCMS platform (omegascans.org): a Next.js
// front end whose series pages (/series/{slug}) carry the series id in their
// RSC payload, over a JSON API at api.{host}. The series page renders only the
// newest chapters, so the list comes from the API's paginated chapter/query
// (the same feed luacomic.org serves, read by walkLuascansChapters), and each
// chapter's pages from chapter/{series}/{chapter}.
type HeanCMS struct {
	*Grabber
	api   string // https://api.{host}
	slug  string
	id    int
	title string
	// apiFor names the API host of a site's URL (heanAPIBase; tests point it
	// at their own server)
	apiFor func(rawURL string) string
}

func NewHeanCMS(g *Grabber) *HeanCMS {
	return &HeanCMS{Grabber: g, apiFor: heanAPIBase}
}

var heanSeriesRe = regexp.MustCompile(`^/series/([^/?#]+)`)

// heanAPIBase returns the API host the platform pairs with a site
func heanAPIBase(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://api." + strings.TrimPrefix(u.Hostname(), "www.")
}

// Test fetches series and reader pages (/series/...) and accepts them when the
// RSC payload names a series id and the API knows the series
func (h *HeanCMS) Test() (bool, error) {
	u, err := url.Parse(h.URL)
	if err != nil {
		return false, nil
	}
	m := heanSeriesRe.FindStringSubmatch(u.EscapedPath())
	if m == nil {
		return false, nil
	}
	body, err := http.GetText(http.RequestParams{
		URL:          h.URL,
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		return false, err
	}
	if !seriesIDRe.MatchString(body) {
		return false, nil
	}

	api := h.apiFor(h.URL)
	res, err := http.GetText(http.RequestParams{
		URL:          api + "/series/" + m[1],
		Referer:      h.BaseUrl() + "/",
		DisableRetry: true,
	})
	if err != nil {
		return false, nil
	}
	var series struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Slug  string `json:"series_slug"`
	}
	if json.Unmarshal([]byte(res), &series) != nil || series.ID == 0 || series.Slug == "" {
		return false, nil
	}
	h.api, h.id, h.slug, h.title = api, series.ID, series.Slug, series.Title
	return true, nil
}

// FetchTitle returns the series title the API gives
func (h *HeanCMS) FetchTitle() (string, error) {
	return sanitizeTitle(h.title), nil
}

// FetchChapters walks the API's chapter feed, newest first
func (h *HeanCMS) FetchChapters() (Filterables, []error) {
	return walkLuascansChapters(func(page int) (string, error) {
		return http.GetText(http.RequestParams{
			URL: fmt.Sprintf("%s/chapter/query?page=%d&perPage=200&query=&order=desc&series_id=%d",
				h.api, page, h.id),
			Referer: h.BaseUrl() + "/",
		})
	})
}

// heanChapter is the API's answer for one chapter
type heanChapter struct {
	Chapter struct {
		Data *struct {
			Images []string `json:"images"`
		} `json:"chapter_data"`
	} `json:"chapter"`
}

// FetchChapter asks the API for a chapter's pages
func (h *HeanCMS) FetchChapter(f Filterable) (*Chapter, error) {
	lchap := f.(*LuascansChapter)
	body, err := http.GetText(http.RequestParams{
		URL:     h.api + "/chapter/" + h.slug + "/" + url.PathEscape(lchap.Slug),
		Referer: h.BaseUrl() + "/",
	})
	if err != nil {
		return nil, err
	}
	var res heanChapter
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return nil, fmt.Errorf("could not read %s: %w", ChapterLabel(f), err)
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "en",
		ID:       lchap.Slug,
		Locked:   lchap.Locked,
	}
	if res.Chapter.Data != nil {
		for _, img := range res.Chapter.Data.Images {
			if img = strings.TrimSpace(img); img == "" {
				continue
			}
			chapter.Pages = append(chapter.Pages, Page{
				Number: int64(len(chapter.Pages) + 1),
				URL:    img,
			})
		}
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	if chapter.PagesCount == 0 {
		return nil, fmt.Errorf("no pages found for %s (it may be a paid chapter)", ChapterLabel(f))
	}
	return chapter, nil
}
