// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// PizzaReader is a grabber for sites running the PizzaReader reader
// (hni-scantrad.net): /api/comics/{slug} lists a series with all its
// chapters, and /api/read/... gives one chapter's pages
type PizzaReader struct {
	*Grabber
	comic *pizzareaderComic
}

// PizzaReaderChapter is a chapter listed by the series API
type PizzaReaderChapter struct {
	Chapter
	// Path is the chapter's reader path (/read/{slug}/{lang}/vol/1/ch/2)
	Path string
}

// pizzareaderPathRe captures the series slug of a series (/comics/{slug}) or
// reader (/read/{slug}/...) path
var pizzareaderPathRe = regexp.MustCompile(`^/(?:comics|read)/([^/]+)`)

func NewPizzaReader(g *Grabber) *PizzaReader {
	return &PizzaReader{Grabber: g}
}

// Test accepts a /comics/ or /read/ URL whose site answers the PizzaReader
// series API, so every instance works without a domain list
func (p *PizzaReader) Test() (bool, error) {
	if p.slug() == "" {
		return false, nil
	}
	comic, err := p.fetchComic(true)

	return err == nil && comic.Slug != "" && comic.Chapters != nil, nil
}

// slug returns the series slug of the URL, "" when it names none
func (p PizzaReader) slug() string {
	u, err := url.Parse(p.URL)
	if err != nil {
		return ""
	}
	matches := pizzareaderPathRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return ""
	}
	return matches[1]
}

// pizzareaderComic is the series API's comic
type pizzareaderComic struct {
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Chapters []struct {
		Title      string `json:"title"`
		FullTitle  string `json:"full_title"`
		Volume     *int   `json:"volume"`
		Chapter    *int   `json:"chapter"`
		Subchapter *int   `json:"subchapter"`
		Language   string `json:"language"`
		URL        string `json:"url"`
		Key        string `json:"slug_lang_vol_ch_sub"`
		Teams      []*struct {
			Name string `json:"name"`
		} `json:"teams"`
	} `json:"chapters"`
}

// fetchComic fetches the series API once; probe skips the retries, for
// identifying a site
func (p *PizzaReader) fetchComic(probe bool) (*pizzareaderComic, error) {
	if p.comic != nil {
		return p.comic, nil
	}
	body, err := http.GetText(http.RequestParams{
		URL:          p.BaseUrl() + "/api/comics/" + url.PathEscape(p.slug()),
		Referer:      p.URL,
		Accept:       "application/json",
		DisableRetry: probe,
	})
	if err != nil {
		return nil, err
	}
	feed := struct {
		Comic pizzareaderComic `json:"comic"`
	}{}
	if err := json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	p.comic = &feed.Comic

	return p.comic, nil
}

// FetchTitle returns the series title
func (p *PizzaReader) FetchTitle() (string, error) {
	comic, err := p.fetchComic(false)
	if err != nil {
		return "", err
	}
	return sanitizeTitle(comic.Title), nil
}

// FetchChapters returns the series' chapters, in the preferred language when
// the series has it (a series can mix languages). A one-shot (no chapter
// number) is chapter 0; a subchapter is the decimal part (12.5).
func (p *PizzaReader) FetchChapters() (Filterables, []error) {
	comic, err := p.fetchComic(false)
	if err != nil {
		return nil, []error{err}
	}

	lang := strings.ToLower(p.GetPreferredLanguage())
	hasLang := false
	for _, c := range comic.Chapters {
		hasLang = hasLang || strings.EqualFold(c.Language, lang)
	}

	chapters := Filterables{}
	for _, c := range comic.Chapters {
		if hasLang && !strings.EqualFold(c.Language, lang) {
			continue
		}
		number := 0.0
		if c.Chapter != nil {
			text := strconv.Itoa(*c.Chapter)
			if c.Subchapter != nil {
				text += "." + strconv.Itoa(*c.Subchapter)
			}
			number, _ = strconv.ParseFloat(text, 64)
		}
		title := c.Title
		if c.Chapter == nil && title == "" {
			title = c.FullTitle
		}
		label := Label{Style: LabelNumbered}
		if _, ok := chapterWords[c.Language]; ok {
			label.Lang = c.Language
		}
		chapter := &PizzaReaderChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    title,
				Language: c.Language,
				ID:       c.Key,
				Label:    label,
			},
			Path: c.URL,
		}
		if c.Volume != nil {
			chapter.Volume = *c.Volume
		}
		var teams []string
		for _, t := range c.Teams {
			if t != nil {
				teams = append(teams, t.Name)
			}
		}
		chapter.SetGroups(teams...)
		chapters = append(chapters, chapter)
	}

	return chapters, nil
}

// FetchChapter fetches the chapter's pages from the read API
func (p *PizzaReader) FetchChapter(f Filterable) (*Chapter, error) {
	pchap := f.(*PizzaReaderChapter)
	body, err := http.GetText(http.RequestParams{
		URL:     p.BaseUrl() + "/api" + pchap.Path,
		Referer: p.BaseUrl() + pchap.Path,
		Accept:  "application/json",
	})
	if err != nil {
		return nil, err
	}
	feed := struct {
		Chapter struct {
			Pages []string `json:"pages"`
		} `json:"chapter"`
	}{}
	if err := json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Volume:     pchap.Volume,
		Language:   pchap.Language,
		PagesCount: int64(len(feed.Chapter.Pages)),
		ID:         pchap.ID,
	}
	chapter.SetGroups(pchap.GetGroups()...)
	for i, page := range feed.Chapter.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    page,
		})
	}

	return chapter, nil
}
