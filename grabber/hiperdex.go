// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Hiperdex is a grabber for hiperdex.tv, a Vite/React SPA (not the older
// Madara hiperdex sites) whose data comes from a tRPC API at /api/trpc. Two
// things gate that API: a session cookie (__st) set by any page of the site,
// which the http session harvests from the series page fetch, and a header
// the SPA's bundle builds from obfuscated pieces (a char-code array for the
// name, atob() plus a suffix for the value). Without the header the listing
// still answers but the reader's page list is refused ("use the official
// app"), so it's read from the live bundle rather than hardcoded, to survive
// a rotation.
type Hiperdex struct {
	*Grabber
	series *hiperdexSeries
	// header is the client header the API wants, read from the bundle
	header map[string]string
}

// HiperdexChapter represents a Hiperdex chapter
type HiperdexChapter struct {
	Chapter
	// Slug is the series slug the reader endpoint takes
	Slug string
}

// NewHiperdex returns a Hiperdex grabber
func NewHiperdex(g *Grabber) *Hiperdex {
	return &Hiperdex{Grabber: g}
}

// hiperdexChapterRe matches reader paths (/manga/{slug}/{number})
var hiperdexChapterRe = regexp.MustCompile(`^/manga/([^/]+)/[^/]+/?$`)

// hiperdexSeriesRe matches series paths and captures the slug
var hiperdexSeriesRe = regexp.MustCompile(`^/manga/([^/]+)/?$`)

// hiperdexBundleRe finds the SPA's entry bundle in the page
var hiperdexBundleRe = regexp.MustCompile(`<script[^>]+src="(/assets/index-[^"]+\.js)"`)

// hiperdexHeaderRe matches how the bundle assembles the client header:
// [120,45,...].map(n=>String.fromCharCode(n)).join(""),x=atob("...")+"..."
var hiperdexHeaderRe = regexp.MustCompile(`\[([\d,\s]+)\]\.map\(\w+=>String\.fromCharCode\(\w+\)\)\.join\(""\),\w+=atob\("([A-Za-z0-9+/=]+)"\)(?:\+"([^"]*)")?`)

// Test returns true for hiperdex.tv URLs
func (h *Hiperdex) Test() (bool, error) {
	host, _, ok := chapterPath(h.URL)
	return ok && host == "hiperdex.tv", nil
}

// resolveSeriesURL rewrites a reader URL to its series URL. No-op for series
// URLs.
func (h *Hiperdex) resolveSeriesURL() {
	u, err := url.Parse(h.URL)
	if err != nil {
		return
	}
	m := hiperdexChapterRe.FindStringSubmatch(u.Path)
	if m == nil {
		return
	}
	u.Path = "/manga/" + m[1]
	u.RawQuery = ""
	u.Fragment = ""
	h.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (h *Hiperdex) FetchTitle() (string, error) {
	series, err := h.fetchSeries()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(series.Title), nil
}

// FetchChapters returns the chapters of the manga
func (h *Hiperdex) FetchChapters() (chapters Filterables, errs []error) {
	series, err := h.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	var rows []hiperdexChapterRow
	if err := h.query("series.chapters", map[string]any{"seriesId": series.ID}, &rows); err != nil {
		return nil, []error{err}
	}

	for _, c := range rows {
		if c.IsDeleted || (c.Status != "" && c.Status != "published") {
			continue
		}
		title := ""
		if c.Title != nil {
			title = sanitizeTitle(*c.Title)
		}
		volume := 0
		if c.Volume != nil {
			volume = *c.Volume
		}
		chapters = append(chapters, &HiperdexChapter{
			Chapter: Chapter{
				Number:     c.Number,
				Title:      title,
				PagesCount: c.PagesCount,
				Language:   c.Language,
				Volume:     volume,
				ID:         strconv.FormatInt(c.ID, 10),
				Label:      Label{Style: LabelPrefixed},
			},
			Slug: series.Slug,
		})
	}

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (h *Hiperdex) FetchChapter(f Filterable) (*Chapter, error) {
	hchap := f.(*HiperdexChapter)
	id, err := strconv.ParseInt(hchap.ID, 10, 64)
	if err != nil {
		return nil, err
	}
	// the cookie and header come with the series fetch
	if _, err := h.fetchSeries(); err != nil {
		return nil, err
	}

	var pages []hiperdexPage
	err = h.query("reader.chapterPages", map[string]any{
		"seriesSlug":    hchap.Slug,
		"chapterNumber": hchap.Number,
		"chapterId":     id,
	}, &pages)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("chapter %s has no pages", hchap.ID)
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].PageOrder < pages[j].PageOrder })

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Label:    labelOf(f),
		Language: hchap.Language,
		Volume:   hchap.Volume,
		ID:       hchap.ID,
	}
	for i, p := range pages {
		src := p.WebpURL
		if src == "" && p.AvifURL != nil {
			src = *p.AvifURL
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: src})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// fetchSeries loads the series page (for its session cookie and bundle) and
// then the series record, once
func (h *Hiperdex) fetchSeries() (*hiperdexSeries, error) {
	h.resolveSeriesURL()
	if h.series != nil {
		return h.series, nil
	}

	u, err := url.Parse(h.URL)
	if err != nil {
		return nil, err
	}
	m := hiperdexSeriesRe.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, errSeriesNotFound(h.URL)
	}

	html, err := http.GetText(http.RequestParams{URL: h.URL})
	if err != nil {
		return nil, err
	}
	bundle := hiperdexBundleRe.FindStringSubmatch(html)
	if bundle == nil {
		return nil, fmt.Errorf("could not find the site's script in %s", h.URL)
	}
	js, err := http.GetText(http.RequestParams{URL: h.BaseUrl() + bundle[1], Referer: h.URL})
	if err != nil {
		return nil, err
	}
	name, value, err := hiperdexClientHeader(js)
	if err != nil {
		return nil, err
	}
	h.header = map[string]string{name: value}

	series := &hiperdexSeries{}
	if err := h.query("series.bySlug", map[string]any{"slug": m[1]}, series); err != nil {
		return nil, err
	}
	if series.ID == 0 {
		return nil, errSeriesNotFound(h.URL)
	}
	h.series = series
	return series, nil
}

// hiperdexClientHeader decodes the client header's name and value from the
// SPA's bundle
func hiperdexClientHeader(js string) (name, value string, err error) {
	m := hiperdexHeaderRe.FindStringSubmatch(js)
	if m == nil {
		return "", "", fmt.Errorf("could not find the API client header in the site's script")
	}
	var b strings.Builder
	for _, code := range strings.Split(m[1], ",") {
		n, err := strconv.Atoi(strings.TrimSpace(code))
		if err != nil {
			return "", "", err
		}
		b.WriteRune(rune(n))
	}
	decoded, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil {
		return "", "", err
	}
	return b.String(), string(decoded) + m[3], nil
}

// query runs a tRPC query (GET, superjson-wrapped input) and decodes its
// result into out
func (h *Hiperdex) query(procedure string, input any, out any) error {
	raw, err := json.Marshal(map[string]any{"json": input})
	if err != nil {
		return err
	}
	body, err := http.Get(http.RequestParams{
		URL:     h.BaseUrl() + "/api/trpc/" + procedure + "?input=" + url.QueryEscape(string(raw)),
		Referer: h.URL,
		Accept:  "application/json",
		Headers: h.header,
	})
	if err != nil {
		return err
	}
	defer body.Close()

	resp := hiperdexResponse{}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s: %s", procedure, resp.Error.JSON.Message)
	}
	return json.Unmarshal(resp.Result.Data.JSON, out)
}

// hiperdexResponse is a tRPC response envelope
type hiperdexResponse struct {
	Result struct {
		Data struct {
			JSON json.RawMessage `json:"json"`
		} `json:"data"`
	} `json:"result"`
	Error *struct {
		JSON struct {
			Message string `json:"message"`
		} `json:"json"`
	} `json:"error"`
}

// hiperdexSeries is the series.bySlug result
type hiperdexSeries struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// hiperdexChapterRow is a series.chapters row
type hiperdexChapterRow struct {
	ID         int64   `json:"id"`
	Number     float64 `json:"number"`
	Volume     *int    `json:"volume"`
	Title      *string `json:"title"`
	Language   string  `json:"language"`
	Status     string  `json:"status"`
	PagesCount int64   `json:"pagesCount"`
	IsDeleted  bool    `json:"isDeleted"`
}

// hiperdexPage is a reader.chapterPages row
type hiperdexPage struct {
	PageOrder int     `json:"pageOrder"`
	WebpURL   string  `json:"webpUrl"`
	AvifURL   *string `json:"avifUrl"`
}
