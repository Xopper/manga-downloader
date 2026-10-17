// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// comiclandAPI is the JSON API behind comicland.org's React SPA
const comiclandAPI = "https://api.comicland.org/api"

// Comicland is a grabber for comicland.org, a client-rendered SPA (its HTML
// is an empty shell titled "Manga Web") backed by a JSON API on
// api.comicland.org. The API refuses any request whose Referer or Origin
// isn't the site ("invalid referer or origin"), so both are sent. Series
// details carry the whole chapter list; a chapter's pages are asked for by
// series slug and chapter index, the same pair the reader URL holds
// (/comic/{slug}/chapter/{index}).
type Comicland struct {
	*Grabber
	// api is the API base, overridden in tests
	api    string
	detail *comiclandDetail
}

// ComiclandChapter represents a Comicland chapter
type ComiclandChapter struct {
	Chapter
	// Slug and Index are what the pages endpoint takes
	Slug  string
	Index float64
}

// NewComicland returns a Comicland grabber
func NewComicland(g *Grabber) *Comicland {
	return &Comicland{Grabber: g, api: comiclandAPI}
}

// comiclandPathRe matches series and reader paths and captures the slug
var comiclandPathRe = regexp.MustCompile(`^/comic/([^/]+)(/chapter/[^/]+)?/?$`)

// Test returns true for comicland.org URLs
func (c *Comicland) Test() (bool, error) {
	host, _, ok := chapterPath(c.URL)
	return ok && host == "comicland.org", nil
}

// resolveSeriesURL rewrites a reader URL to its series URL. No-op for series
// URLs.
func (c *Comicland) resolveSeriesURL() {
	u, err := url.Parse(c.URL)
	if err != nil {
		return
	}
	m := comiclandPathRe.FindStringSubmatch(u.Path)
	if m == nil || m[2] == "" {
		return
	}
	u.Path = "/comic/" + m[1]
	u.RawQuery = ""
	u.Fragment = ""
	c.URL = u.String()
}

// slug returns the series slug of the (resolved) URL
func (c *Comicland) slug() (string, error) {
	c.resolveSeriesURL()
	u, err := url.Parse(c.URL)
	if err != nil {
		return "", err
	}
	m := comiclandPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errSeriesNotFound(c.URL)
	}
	return m[1], nil
}

// FetchTitle fetches and returns the manga title
func (c *Comicland) FetchTitle() (string, error) {
	detail, err := c.fetchDetail()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(detail.Title), nil
}

// FetchChapters returns the chapters of the manga
func (c *Comicland) FetchChapters() (chapters Filterables, errs []error) {
	detail, err := c.fetchDetail()
	if err != nil {
		return nil, []error{err}
	}
	for _, ch := range detail.Chapters {
		// the API titles every chapter "Chapter N": that's the label's job
		title := sanitizeTitle(ch.Title)
		if n, ok := parseChapterNumber(title); ok && n == ch.ChapterIndex && chapterNumberRe.ReplaceAllString(title, "") == "" {
			title = ""
		}
		chapters = append(chapters, &ComiclandChapter{
			Chapter: Chapter{
				Number:     ch.ChapterIndex,
				Title:      title,
				PagesCount: ch.PageCount,
				ID:         strconv.FormatInt(ch.ID, 10),
				Label:      Label{Style: LabelNumbered},
			},
			Slug:  detail.Slug,
			Index: ch.ChapterIndex,
		})
	}
	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (c *Comicland) FetchChapter(f Filterable) (*Chapter, error) {
	cchap := f.(*ComiclandChapter)

	resp := struct {
		Code int `json:"code"`
		Data struct {
			ChapterID int64    `json:"chapter_id"`
			Pages     []string `json:"pages"`
		} `json:"data"`
	}{}
	q := url.Values{"slug": {cchap.Slug}, "index": {strconv.FormatFloat(cchap.Index, 'f', -1, 64)}}
	if err := c.get("/chapter/pages_by_index?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	// the reader shows a chapter with no id or pages as unavailable
	if resp.Data.ChapterID == 0 || len(resp.Data.Pages) == 0 {
		return nil, fmt.Errorf("chapter %s is not available", strconv.FormatFloat(cchap.Index, 'f', -1, 64))
	}

	chapter := &Chapter{
		Title:  f.GetTitle(),
		Number: f.GetNumber(),
		Label:  labelOf(f),
		ID:     cchap.ID,
	}
	for i, p := range resp.Data.Pages {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: p})
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}

// fetchDetail loads the series details, once
func (c *Comicland) fetchDetail() (*comiclandDetail, error) {
	slug, err := c.slug()
	if err != nil {
		return nil, err
	}
	if c.detail != nil {
		return c.detail, nil
	}
	resp := struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    comiclandDetail `json:"data"`
	}{}
	if err := c.get("/comic/detail?"+url.Values{"slug": {slug}}.Encode(), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 || resp.Data.Slug == "" {
		return nil, errSeriesNotFound(c.URL)
	}
	c.detail = &resp.Data
	return c.detail, nil
}

// get calls the API with the site as Referer and Origin
func (c *Comicland) get(path string, out any) error {
	site := c.BaseUrl()
	body, err := http.Get(http.RequestParams{
		URL:     c.api + path,
		Referer: site,
		Origin:  site,
		Accept:  "application/json",
	})
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(out)
}

// comiclandDetail is the /comic/detail payload
type comiclandDetail struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Chapters []struct {
		ID           int64   `json:"id"`
		ChapterIndex float64 `json:"chapter_index"`
		Title        string  `json:"title"`
		PageCount    int64   `json:"page_count"`
	} `json:"chapters"`
}
