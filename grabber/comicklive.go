// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// comickliveDomains are the mirrors of the comick.live clone
var comickliveDomains = []string{"comick.live", "comick.art"}

// comickliveSlugRe takes the series slug out of a series URL (/comic/{slug})
// or a chapter one (/comic/{slug}/{hid}-chapter-{n}-{lang})
var comickliveSlugRe = regexp.MustCompile(`^/comic/([^/?#]+)`)

// ComickLive is a grabber for comick.live (and its comick.art mirror), a
// clone of the defunct comick.io that keeps its URL scheme and JSON shapes. The
// series page embeds its details in a <script id="comic-data"> JSON (title and
// the languages it has chapters in), the chapter list is a 60-per-page
// /api/comics/{slug}/chapter-list, one row per release (several groups and
// languages per number), and each reader page embeds its images in a
// <script id="sv-data"> JSON.
type ComickLive struct {
	*Grabber
	data *comickliveComic
}

// ComickLiveChapter is a listed comick.live release
type ComickLiveChapter struct {
	Chapter
	// Path is the release's reader path, /comic/{slug}/{hid}-chapter-{n}-{lang}
	Path string
}

// NewComickLive returns a comick.live grabber
func NewComickLive(g *Grabber) *ComickLive {
	return &ComickLive{Grabber: g}
}

// Test matches the comick.live mirrors by domain
func (c *ComickLive) Test() (bool, error) {
	return slices.ContainsFunc(comickliveDomains, func(d string) bool {
		return hostMatchesDomain(c.URL, d)
	}), nil
}

// LanguageOptions lists the languages the series page says it has chapters
// in, valid once the title or chapters were fetched
func (c *ComickLive) LanguageOptions() FilterOptions {
	var langs []string
	if c.data != nil {
		for _, l := range c.data.LangList {
			if l.Value != "settings" && l.Value != FilterAll {
				langs = append(langs, l.Value)
			}
		}
	}

	return FilterOptions{Values: uniqueValues(langs), All: true}
}

// slug returns the series slug of the URL, series or chapter alike
func (c *ComickLive) slug() (string, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return "", err
	}
	m := comickliveSlugRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errSeriesNotFound(c.URL)
	}

	return m[1], nil
}

// resolveSeriesURL cuts a chapter URL back to its series, which its path
// starts with
func (c *ComickLive) resolveSeriesURL() error {
	slug, err := c.slug()
	if err != nil {
		return err
	}
	c.URL = c.BaseUrl() + "/comic/" + slug

	return nil
}

// comic returns the series' comic-data, fetched once
func (c *ComickLive) comic() (*comickliveComic, error) {
	if c.data != nil {
		return c.data, nil
	}
	if err := c.resolveSeriesURL(); err != nil {
		return nil, err
	}
	data := &comickliveComic{}
	if err := comickliveScriptJSON(c.URL, c.BaseUrl(), "comic-data", data); err != nil {
		return nil, err
	}
	c.data = data

	return data, nil
}

// comickliveScriptJSON decodes the JSON a page embeds in <script id={id}>
func comickliveScriptJSON(uri, referer, id string, v any) error {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: referer})
	if err != nil {
		return err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}
	raw := doc.Find(`script#` + id).First().Text()
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("comick: no %s in %s", id, uri)
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return fmt.Errorf("comick: %s in %s: %w", id, uri, err)
	}

	return nil
}

// FetchTitle fetches and returns the series title
func (c *ComickLive) FetchTitle() (string, error) {
	data, err := c.comic()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(data.Title), nil
}

// FetchChapters returns the series' releases in the chosen language (English
// by default, every one with "all"), walking the list's pages
func (c *ComickLive) FetchChapters() (chapters Filterables, errs []error) {
	data, err := c.comic()
	if err != nil {
		return nil, []error{err}
	}
	language := c.Settings.Language
	if language == "" {
		language = "en"
	}
	query := ""
	if language != FilterAll {
		query = "lang=" + url.QueryEscape(language) + "&"
	}

	for page, last := 1, 1; page <= last; page++ {
		uri := fmt.Sprintf("%s/api/comics/%s/chapter-list?%spage=%d", c.BaseUrl(), url.PathEscape(data.Slug), query, page)
		body, err := http.GetText(http.RequestParams{URL: uri, Referer: c.URL, Accept: "application/json"})
		if err != nil {
			return chapters, append(errs, err)
		}
		list := comickliveChapterList{}
		if err := json.Unmarshal([]byte(body), &list); err != nil {
			return chapters, append(errs, fmt.Errorf("comick: chapter list: %w", err))
		}
		last = list.Pagination.LastPage
		for _, ch := range list.Data {
			chapters = append(chapters, comickliveChapter(data.Slug, ch))
		}
	}

	return
}

// comickliveChapter turns a chapter-list row into a listed release. A row with
// no number (a one-shot) is chapter 0.
func comickliveChapter(slug string, ch comickliveChapterRow) *ComickLiveChapter {
	number, _ := strconv.ParseFloat(strings.TrimSpace(ch.Chap), 64)
	volume := 0
	if ch.Vol != nil {
		volume, _ = strconv.Atoi(strings.TrimSpace(*ch.Vol))
	}
	listed := &ComickLiveChapter{
		Chapter: Chapter{
			Number:   number,
			Title:    strings.TrimSpace(ch.Title),
			Label:    Label{Style: LabelNumbered, Number: strings.TrimSpace(ch.Chap)},
			Language: ch.Lang,
			ID:       ch.HID,
			Volume:   volume,
		},
		Path: fmt.Sprintf("/comic/%s/%s-chapter-%s-%s", slug, ch.HID, ch.Chap, ch.Lang),
	}
	listed.SetGroups(ch.Groups...)

	return listed
}

// FetchChapter fetches a release's pages from its reader page
func (c *ComickLive) FetchChapter(f Filterable) (*Chapter, error) {
	cchap := f.(*ComickLiveChapter)
	reader := comickliveReader{}
	if err := comickliveScriptJSON(c.BaseUrl()+cchap.Path, c.URL, "sv-data", &reader); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   cchap.Language,
		ID:         cchap.ID,
		Volume:     cchap.Volume,
		PagesCount: int64(len(reader.Chapter.Images)),
	}
	chapter.SetGroups(cchap.GetGroups()...)
	for i, img := range reader.Chapter.Images {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img.URL})
	}

	return chapter, nil
}

// comickliveComic is the part of the series page's comic-data used
type comickliveComic struct {
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	LangList []struct {
		Value string `json:"value"`
	} `json:"langList"`
}

// comickliveChapterList is a page of /api/comics/{slug}/chapter-list
type comickliveChapterList struct {
	Data       []comickliveChapterRow `json:"data"`
	Pagination struct {
		LastPage int `json:"last_page"`
	} `json:"pagination"`
}

// comickliveChapterRow is one release in the chapter list
type comickliveChapterRow struct {
	HID    string   `json:"hid"`
	Chap   string   `json:"chap"`
	Vol    *string  `json:"vol"`
	Lang   string   `json:"lang"`
	Title  string   `json:"title"`
	Groups []string `json:"group_name"`
}

// comickliveReader is the part of a reader page's sv-data used
type comickliveReader struct {
	Chapter struct {
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	} `json:"chapter"`
}
