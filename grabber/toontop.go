// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// toontopHosts are the domains of the Next.js (pages router) webtoon site
// toontop.io; toonily.me redirects there
var toontopHosts = []string{"toontop.io", "toonily.me"}

// Toontop is a grabber for toontop.io. Series live at /{slug} and readers at
// /{slug}/{chapter-slug}, both server rendered with their data in
// __NEXT_DATA__. The series page only embeds the newest 50 chapters, so the
// list comes from the site's API (siteConfig.apiUrl, /titles/{id}/chapters),
// whose "number" is a position, not the chapter number: that is read from the
// chapter's name. The API's own images endpoint answers a logged-out reader
// only the first few pages, while the reader page's __NEXT_DATA__ has all of
// them, so pages come from the reader page. The image host 403s without the
// site's Referer, which the downloader sends.
type Toontop struct {
	*Grabber
	data *toontopSeriesData
}

// ToontopChapter represents a Toontop chapter
type ToontopChapter struct {
	Chapter
	// URL is the chapter's reader page
	URL string
}

// NewToontop returns a Toontop grabber
func NewToontop(g *Grabber) *Toontop {
	return &Toontop{Grabber: g}
}

// toontopNameRe splits a chapter name like "Chapter 0 - Prologue" into its
// number and title
var toontopNameRe = regexp.MustCompile(`(?i)^chapter\s+(\d+(?:\.\d+)?)\s*(?:[-:–]\s*)?(.*)$`)

// Test returns true for toontop.io URLs
func (t *Toontop) Test() (bool, error) {
	host, _, ok := chapterPath(t.URL)
	if !ok {
		return false, nil
	}
	for _, h := range toontopHosts {
		if host == h {
			return true, nil
		}
	}
	return false, nil
}

// resolveSeriesURL rewrites a reader URL (/{slug}/{chapter-slug}) to its
// series URL (/{slug}). No-op for series URLs.
func (t *Toontop) resolveSeriesURL() {
	u, err := url.Parse(t.URL)
	if err != nil {
		return
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		return
	}
	u.Path = "/" + parts[0]
	u.RawQuery = ""
	u.Fragment = ""
	t.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (t *Toontop) FetchTitle() (string, error) {
	data, err := t.fetchSeries()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(data.Props.PageProps.InitialManga.Name), nil
}

// FetchChapters returns the chapters of the manga
func (t *Toontop) FetchChapters() (chapters Filterables, errs []error) {
	data, err := t.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}
	props := data.Props.PageProps

	api := strings.TrimSuffix(props.SiteConfig.APIURL, "/")
	if api == "" {
		return nil, []error{fmt.Errorf("could not find the site's API in %s", t.URL)}
	}
	body, err := http.Get(http.RequestParams{
		URL:     api + "/titles/" + url.PathEscape(props.InitialManga.ID) + "/chapters",
		Referer: t.BaseUrl(),
		Origin:  t.BaseUrl(),
		Accept:  "application/json",
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	list := toontopChapterList{}
	if err := json.NewDecoder(body).Decode(&list); err != nil {
		return nil, []error{err}
	}
	if !list.Success {
		return nil, []error{fmt.Errorf("the chapter list of %s failed to load", t.URL)}
	}

	return toontopChapters(list.Data.Chapters, t.BaseUrl()), nil
}

// toontopChapters builds the listing. A name with a chapter number gives
// the number and the rest of the name as title; any other name (side stories)
// is kept whole as title and numbered by its position in the list.
func toontopChapters(rows []toontopChapterRow, base string) (chapters Filterables) {
	for _, c := range rows {
		chapter := Chapter{ID: c.ID, Number: float64(c.Number), Title: sanitizeTitle(c.Name)}
		if m := toontopNameRe.FindStringSubmatch(strings.TrimSpace(c.Name)); m != nil {
			chapter.Number, _ = strconv.ParseFloat(m[1], 64)
			chapter.Title = sanitizeTitle(m[2])
			chapter.Label = Label{Style: LabelPrefixed}
		}
		chapters = append(chapters, &ToontopChapter{Chapter: chapter, URL: base + c.URL})
	}
	return chapters
}

// FetchChapter fetches a chapter and its pages
func (t *Toontop) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*ToontopChapter)

	data, err := toontopNextData(tchap.URL, t.URL)
	if err != nil {
		return nil, err
	}
	pages := data.Props.PageProps.InitialChapter.Pages
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found in %s", tchap.URL)
	}

	chapter := &Chapter{
		Title:  f.GetTitle(),
		Number: f.GetNumber(),
		Label:  labelOf(f),
		ID:     tchap.ID,
	}
	for i, p := range pages {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: p.URL})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// fetchSeries reads the series page's __NEXT_DATA__, once
func (t *Toontop) fetchSeries() (*toontopSeriesData, error) {
	t.resolveSeriesURL()
	if t.data != nil {
		return t.data, nil
	}
	data, err := toontopNextData(t.URL, "")
	if err != nil {
		return nil, err
	}
	if data.Props.PageProps.InitialManga.ID == "" {
		return nil, errSeriesNotFound(t.URL)
	}
	t.data = data
	return data, nil
}

// toontopNextData fetches a page and decodes its __NEXT_DATA__ script
func toontopNextData(pageURL, referer string) (*toontopSeriesData, error) {
	body, err := http.Get(http.RequestParams{URL: pageURL, Referer: referer})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	raw := doc.Find("script#__NEXT_DATA__").Text()
	if raw == "" {
		return nil, fmt.Errorf("no page data found in %s", pageURL)
	}
	data := &toontopSeriesData{}
	if err := json.Unmarshal([]byte(raw), data); err != nil {
		return nil, err
	}
	return data, nil
}

// toontopSeriesData is the part of __NEXT_DATA__ the grabber reads, for
// series and reader pages alike
type toontopSeriesData struct {
	Props struct {
		PageProps struct {
			SiteConfig struct {
				APIURL string `json:"apiUrl"`
			} `json:"siteConfig"`
			InitialManga struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"initialManga"`
			InitialChapter struct {
				Pages []struct {
					URL string `json:"url"`
				} `json:"pages"`
			} `json:"initialChapter"`
		} `json:"pageProps"`
	} `json:"props"`
}

// toontopChapterRow is a chapter of the /titles/{id}/chapters response
type toontopChapterRow struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
	// Number is the chapter's position in the list, oldest first
	Number int `json:"number"`
}

// toontopChapterList is the /titles/{id}/chapters response
type toontopChapterList struct {
	Success bool `json:"success"`
	Data    struct {
		Chapters []toontopChapterRow `json:"chapters"`
	} `json:"data"`
}
