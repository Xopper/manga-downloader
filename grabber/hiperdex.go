// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// hiperdexAuth is the x-cfg-auth value the site's own front end sends with
// every API call
const hiperdexAuth = "yceqt7qgu004"

var (
	hiperdexHostRe   = regexp.MustCompile(`(?i)^https?://(?:www\.)?hiperdex\.tv(?:[/?#]|$)`)
	hiperdexSeriesRe = regexp.MustCompile(`^/manga/([^/?#]+)`)
)

// Hiperdex is a grabber for hiperdex.tv, a Next.js site over a tRPC API
// (/api/trpc/{procedures}?batch=1&input=…, answered with one result per
// procedure). The API answers 401 until the visitor holds the __st cookie
// that any HTML page sets, so the series page is fetched first; the http
// package keeps the cookie for the API calls that follow.
type Hiperdex struct {
	*Grabber
	series *hiperdexSeries
}

func NewHiperdex(g *Grabber) *Hiperdex {
	return &Hiperdex{Grabber: g}
}

// HiperdexChapter is a hiperdex.tv chapter
type HiperdexChapter struct {
	Chapter
	ChapterID int64
	Raw       json.Number
}

type hiperdexSeries struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type hiperdexChapterJSON struct {
	ID        int64       `json:"id"`
	Number    json.Number `json:"number"`
	Title     *string     `json:"title"`
	Language  string      `json:"language"`
	Status    string      `json:"status"`
	IsDeleted bool        `json:"isDeleted"`
}

type hiperdexPageJSON struct {
	PageOrder int     `json:"pageOrder"`
	WebpURL   string  `json:"webpUrl"`
	AvifURL   *string `json:"avifUrl"`
}

// Test returns true for hiperdex.tv URLs
func (h *Hiperdex) Test() (bool, error) {
	return hiperdexHostRe.MatchString(h.URL), nil
}

// hiperdexSlug reads the series slug off a series or chapter URL
// (/manga/{slug}, /manga/{slug}/{number})
func hiperdexSlug(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	m := hiperdexSeriesRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errSeriesNotFound(rawURL)
	}

	return m[1], nil
}

// call runs a batch of tRPC procedures and returns each one's result data
func (h *Hiperdex) call(procedures string, input map[string]any) ([]json.RawMessage, error) {
	in, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	q := url.Values{"batch": {"1"}, "input": {string(in)}}
	text, err := http.GetText(http.RequestParams{
		URL:     h.BaseUrl() + "/api/trpc/" + procedures + "?" + q.Encode(),
		Headers: map[string]string{"x-cfg-auth": hiperdexAuth},
		Accept:  "application/json",
	})
	if err != nil {
		return nil, err
	}

	return hiperdexResults(text)
}

func hiperdexResults(text string) ([]json.RawMessage, error) {
	batch := []struct {
		Result struct {
			Data struct {
				JSON json.RawMessage `json:"json"`
			} `json:"data"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}{}
	if err := json.Unmarshal([]byte(text), &batch); err != nil {
		return nil, fmt.Errorf("unexpected API answer: %w", err)
	}
	results := make([]json.RawMessage, len(batch))
	for i, b := range batch {
		if len(b.Error) > 0 {
			return nil, fmt.Errorf("the API answered an error: %s", b.Error)
		}
		results[i] = b.Result.Data.JSON
	}

	return results, nil
}

func (h *Hiperdex) fetchSeries() (*hiperdexSeries, error) {
	if h.series != nil {
		return h.series, nil
	}
	slug, err := hiperdexSlug(h.URL)
	if err != nil {
		return nil, err
	}
	// sets the __st cookie the API asks for
	if _, err = http.GetText(http.RequestParams{URL: h.BaseUrl() + "/manga/" + slug}); err != nil {
		return nil, err
	}
	results, err := h.call("auth.me,series.bySlugWithGenres", map[string]any{
		"0": map[string]any{"json": nil, "meta": map[string]any{"values": []string{"undefined"}}},
		"1": map[string]any{"json": map[string]any{"slug": slug}},
	})
	if err != nil {
		return nil, err
	}
	series := &hiperdexSeries{}
	if len(results) == 0 || json.Unmarshal(results[len(results)-1], series) != nil || series.ID == 0 {
		return nil, fmt.Errorf("series %s not found", slug)
	}
	h.series = series

	return series, nil
}

// FetchTitle returns the series title
func (h *Hiperdex) FetchTitle() (string, error) {
	series, err := h.fetchSeries()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(series.Title), nil
}

// FetchChapters returns the series' published chapters
func (h *Hiperdex) FetchChapters() (Filterables, []error) {
	series, err := h.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}
	results, err := h.call("auth.me,series.chapters", map[string]any{
		"0": map[string]any{"json": map[string]any{"values": []string{"undefined"}}},
		"1": map[string]any{
			"json": map[string]any{"seriesId": series.ID, "chapterId": nil, "sort": "best", "page": 1, "limit": 20},
			"meta": map[string]any{"values": map[string]any{"chapterId": []string{"undefined"}}},
		},
		"2": map[string]any{"json": map[string]any{"seriesId": series.ID}},
	})
	if err != nil {
		return nil, []error{err}
	}
	list := []hiperdexChapterJSON{}
	if len(results) == 0 || json.Unmarshal(results[len(results)-1], &list) != nil {
		return nil, []error{errors.New("could not read the chapter list")}
	}

	return hiperdexChapters(list), nil
}

func hiperdexChapters(list []hiperdexChapterJSON) (chapters Filterables) {
	for _, c := range list {
		if c.IsDeleted || (c.Status != "" && c.Status != "published") {
			continue
		}
		number, _ := c.Number.Float64()
		title := ""
		if c.Title != nil {
			title = *c.Title
		}
		chapters = append(chapters, &HiperdexChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    sanitizeTitle(title),
				ID:       strconv.FormatInt(c.ID, 10),
				Language: c.Language,
				Label:    Label{Style: LabelPrefixed},
			},
			ChapterID: c.ID,
			Raw:       c.Number,
		})
	}

	return chapters
}

// FetchChapter asks the reader API for the chapter's pages
func (h *Hiperdex) FetchChapter(f Filterable) (*Chapter, error) {
	hchap := f.(*HiperdexChapter)
	series, err := h.fetchSeries()
	if err != nil {
		return nil, err
	}
	results, err := h.call("auth.me,series.bySlug,reader.chapterPages", map[string]any{
		"0": map[string]any{"json": nil, "meta": map[string]any{"values": []string{"undefined"}}},
		"1": map[string]any{"json": map[string]any{"slug": series.Slug}},
		"2": map[string]any{"json": map[string]any{"seriesSlug": series.Slug, "chapterNumber": hchap.Raw, "chapterId": hchap.ChapterID}},
		"3": map[string]any{"json": map[string]any{"position": "footer_bottom"}},
	})
	if err != nil {
		return nil, err
	}
	pages := hiperdexPages(results)
	if len(pages) == 0 {
		return nil, fmt.Errorf("chapter %s has no pages", hchap.Raw)
	}

	chapter := &Chapter{
		Title:      hchap.GetTitle(),
		Number:     hchap.GetNumber(),
		Language:   hchap.Language,
		ID:         hchap.ID,
		Label:      labelOf(f),
		Pages:      pages,
		PagesCount: int64(len(pages)),
	}

	return chapter, nil
}

// hiperdexPages finds the page list among the batch's results (the reader's
// is the only list of pages), preferring AVIF when the site has it
func hiperdexPages(results []json.RawMessage) []Page {
	for _, r := range results {
		list := []hiperdexPageJSON{}
		if json.Unmarshal(r, &list) != nil || len(list) == 0 || list[0].WebpURL == "" {
			continue
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].PageOrder < list[j].PageOrder })
		pages := make([]Page, 0, len(list))
		for i, p := range list {
			u := p.WebpURL
			if p.AvifURL != nil && *p.AvifURL != "" {
				u = *p.AvifURL
			}
			pages = append(pages, Page{Number: int64(i + 1), URL: u})
		}
		return pages
	}

	return nil
}
