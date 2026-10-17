// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// ZinManga is a grabber for the Laravel copy of the Madara theme behind
// zinmanga.net and kunmanga.online. Its pages look like Madara's, but there's
// no ajax/chapters: the series page names a JSON API (window.getChaptersUrl)
// that lists the chapters 100 at a time. Readers are plain HTML with every
// page in div.reading-content. The platform's older installs ship the list in
// the HTML instead, which a PlainHTML entry reads (topmanhua.fan).
type ZinManga struct {
	*Grabber
	doc      *goquery.Document
	api      string // the chapter list API, from window.getChaptersUrl
	slug     string // the series slug, from window.comicSlug
	title    string
	fetched  bool
	chapters Filterables
}

// ZinMangaChapter is a chapter of a ZinManga site
type ZinMangaChapter struct {
	Chapter
	URL string
}

func NewZinManga(g *Grabber) *ZinManga {
	return &ZinManga{Grabber: g}
}

var (
	zinMangaAPIRe  = regexp.MustCompile(`window\.getChaptersUrl\s*=\s*'([^']+)'`)
	zinMangaSlugRe = regexp.MustCompile(`window\.comicSlug\s*=\s*'([^']+)'`)
)

// Test fetches the page and accepts it when it names the chapter list API,
// which both series and reader pages do
func (z *ZinManga) Test() (bool, error) {
	body, err := http.GetText(http.RequestParams{
		URL:          z.URL,
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		return false, err
	}
	api := zinMangaAPIRe.FindStringSubmatch(body)
	slug := zinMangaSlugRe.FindStringSubmatch(body)
	if api == nil || slug == nil {
		return false, nil
	}
	z.api, z.slug = api[1], slug[1]
	z.doc, err = goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return false, err
	}
	return true, nil
}

// seriesURL is the series page, which a pasted reader URL is reduced to
func (z *ZinManga) seriesURL() string {
	return z.BaseUrl() + "/manga/" + z.slug
}

// FetchTitle returns the series title, read from the series page (a reader
// page doesn't print it on its own)
func (z *ZinManga) FetchTitle() (string, error) {
	if z.title != "" {
		return z.title, nil
	}
	h1 := z.doc.Find(".post-title h1").First()
	if h1.Length() == 0 {
		body, err := http.Get(http.RequestParams{URL: z.seriesURL()})
		if err != nil {
			return "", err
		}
		defer body.Close()
		doc, err := goquery.NewDocumentFromReader(body)
		if err != nil {
			return "", err
		}
		h1 = doc.Find(".post-title h1").First()
	}
	z.title = sanitizeTitle(h1.Text())
	if z.title == "" {
		return "", errors.New("could not find the series title")
	}
	return z.title, nil
}

// zinMangaPage is one page of the chapter list API
type zinMangaPage struct {
	Success bool `json:"success"`
	Data    struct {
		Chapters []struct {
			Num  float64 `json:"chapter_num"`
			Name string  `json:"chapter_name"`
			Slug string  `json:"chapter_slug"`
			ID   int     `json:"chapter_id"`
		} `json:"chapters"`
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
	} `json:"data"`
}

// zinMangaMaxPages bounds the walk in case the API never reports a last page
const zinMangaMaxPages = 100

// FetchChapters walks the chapter list API, newest first
func (z *ZinManga) FetchChapters() (Filterables, []error) {
	if z.fetched {
		return z.chapters, nil
	}
	var chapters Filterables
	for page := 1; page <= zinMangaMaxPages; page++ {
		body, err := http.GetText(http.RequestParams{
			URL:     z.api + "?page=" + strconv.Itoa(page) + "&per_page=100&order=desc",
			Referer: z.seriesURL(),
		})
		if err != nil {
			return chapters, []error{err}
		}
		list, err := parseZinMangaPage(body)
		if err != nil {
			return chapters, []error{err}
		}
		for _, c := range list.Data.Chapters {
			title := sanitizeTitle(c.Name)
			if chapterNumberOnlyRe.MatchString(title) {
				// "Chapter 21" says nothing the label doesn't
				title = ""
			}
			chapters = append(chapters, &ZinMangaChapter{
				Chapter{
					Number: c.Num,
					Title:  title,
					ID:     strconv.Itoa(c.ID),
					Label:  Label{Style: LabelNumbered},
				},
				z.seriesURL() + "/" + url.PathEscape(c.Slug),
			})
		}
		if len(list.Data.Chapters) == 0 || list.Data.CurrentPage >= list.Data.LastPage {
			break
		}
	}
	z.chapters, z.fetched = chapters, true
	return chapters, nil
}

// parseZinMangaPage decodes one page of the chapter list API
func parseZinMangaPage(body string) (zinMangaPage, error) {
	var page zinMangaPage
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		return page, fmt.Errorf("could not read the chapter list: %w", err)
	}
	if !page.Success {
		return page, errors.New("the chapter list API answered with an error")
	}
	return page, nil
}

// FetchChapter fetches a reader page and returns its pages
func (z *ZinManga) FetchChapter(f Filterable) (*Chapter, error) {
	zchap := f.(*ZinMangaChapter)
	body, err := http.Get(http.RequestParams{
		URL:     zchap.URL,
		Referer: z.seriesURL(),
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		ID:       zchap.ID,
		Language: "en",
	}
	doc.Find("div.reading-content img").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("data-src", ""))
		if src == "" {
			src = strings.TrimSpace(s.AttrOr("src", ""))
		}
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))
	if chapter.PagesCount == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return chapter, nil
}
