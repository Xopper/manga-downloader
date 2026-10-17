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

// yskcomicsPathRe matches ysk-comics.com series (/en/comic/{slug}) and
// chapter (/en/chapter/{slug}) paths; the first segment is the edition's
// language (en or ar)
var yskcomicsPathRe = regexp.MustCompile(`^/([a-z]{2})/(comic|chapter)/([^/?#]+)`)

// YskComics is a grabber for ysk-comics.com, a Nuxt site with western comics
// in English and Arabic. Its pages are rendered from a JSON API the site
// proxies under its own /api/ (series details, a 12-per-page chapter list and
// each chapter's image URLs), so no HTML is parsed except to find a pasted
// chapter's series. The edition's language goes in an x-localization header.
type YskComics struct {
	*Grabber
	resolved bool
}

// YskComicsChapter is a listed ysk-comics.com chapter
type YskComicsChapter struct {
	Chapter
	// Slug is the chapter's slug, which the images API takes
	Slug string
}

// NewYskComics returns a ysk-comics.com grabber
func NewYskComics(g *Grabber) *YskComics {
	return &YskComics{Grabber: g}
}

// Test matches ysk-comics.com series and chapter URLs by domain
func (y *YskComics) Test() (bool, error) {
	if !hostMatchesDomain(y.URL, "ysk-comics.com") {
		return false, nil
	}
	u, err := url.Parse(y.URL)
	if err != nil {
		return false, nil
	}

	return yskcomicsPathRe.MatchString(u.Path), nil
}

// path returns the URL's language, kind ("comic" or "chapter") and slug
func (y *YskComics) path() (lang, kind, slug string, err error) {
	u, err := url.Parse(y.URL)
	if err != nil {
		return "", "", "", err
	}
	m := yskcomicsPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", "", fmt.Errorf("ysk-comics: not a series or chapter URL: %s", y.URL)
	}

	return m[1], m[2], m[3], nil
}

// resolveSeriesURL turns a chapter URL into its series URL. A chapter slug
// ("ghost-rider-2016-2") can't be turned into the series one
// ("Ghost-Rider-2016", case kept), so it's read off the reader's back link.
func (y *YskComics) resolveSeriesURL() error {
	if y.resolved {
		return nil
	}
	y.resolved = true
	lang, kind, _, err := y.path()
	if err != nil || kind != "chapter" {
		return err
	}

	body, err := http.Get(http.RequestParams{URL: y.URL, Referer: y.BaseUrl()})
	if err != nil {
		return err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}
	href, ok := doc.Find(`a[href^="/` + lang + `/comic/"]`).First().Attr("href")
	if !ok {
		return errSeriesNotFound(y.URL)
	}
	y.URL = y.BaseUrl() + href

	return nil
}

// api fetches a ysk-comics.com API path into v
func (y *YskComics) api(path, lang string, v any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     y.BaseUrl() + path,
		Referer: y.URL,
		Accept:  "application/json",
		Headers: map[string]string{"x-localization": lang},
	})
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(body), v); err != nil {
		return fmt.Errorf("ysk-comics: %s: %w", path, err)
	}

	return nil
}

// FetchTitle fetches and returns the series title
func (y *YskComics) FetchTitle() (string, error) {
	if err := y.resolveSeriesURL(); err != nil {
		return "", err
	}
	lang, _, slug, err := y.path()
	if err != nil {
		return "", err
	}
	res := yskcomicsComic{}
	if err := y.api("/api/comic/"+slug, lang, &res); err != nil {
		return "", err
	}
	title := sanitizeTitle(res.Data.FullName)
	if title == "" {
		return "", fmt.Errorf("ysk-comics: no title for %s", y.URL)
	}

	return title, nil
}

// FetchChapters returns the chapters of the series, walking the chapter
// list's pages until it says there's no next one
func (y *YskComics) FetchChapters() (chapters Filterables, errs []error) {
	if err := y.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}
	lang, _, slug, err := y.path()
	if err != nil {
		return nil, []error{err}
	}

	for page := 1; ; page++ {
		res := yskcomicsChapterList{}
		if err := y.api(fmt.Sprintf("/api/comic/chapter/%s?page=%d", slug, page), lang, &res); err != nil {
			return chapters, append(errs, err)
		}
		for _, c := range res.Data.Chapters {
			number, err := strconv.ParseFloat(strings.TrimSpace(c.Rank), 64)
			if err != nil {
				errs = append(errs, fmt.Errorf("ysk-comics: chapter %q has no number", c.Name))
				continue
			}
			chapters = append(chapters, &YskComicsChapter{
				Chapter: Chapter{
					Number: number,
					// the name is only the series and number ("Ghost Rider (2016) 1#")
					Label:    Label{Style: LabelNumbered},
					Language: lang,
					ID:       strconv.Itoa(c.ID),
				},
				Slug: c.Slug,
			})
		}
		if res.Data.Meta.LinkNext == nil || page >= res.Data.Meta.TotalPages {
			break
		}
	}

	return
}

// FetchChapter fetches a chapter's image list
func (y *YskComics) FetchChapter(f Filterable) (*Chapter, error) {
	ychap := f.(*YskComicsChapter)
	res := yskcomicsImages{}
	if err := y.api("/api/chapters/images/"+url.PathEscape(ychap.Slug), ychap.Language, &res); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   ychap.Language,
		ID:         ychap.ID,
		PagesCount: int64(len(res.Data)),
	}
	for i, img := range res.Data {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img})
	}

	return chapter, nil
}

// yskcomicsComic is the part of /api/comic/{slug} used
type yskcomicsComic struct {
	Data struct {
		FullName string `json:"full_name"`
	} `json:"data"`
}

// yskcomicsChapterList is a page of /api/comic/chapter/{slug}
type yskcomicsChapterList struct {
	Data struct {
		Chapters []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
			// Rank is the chapter number, as a string ("12")
			Rank string `json:"rank"`
		} `json:"data_messages"`
		Meta struct {
			LinkNext   *string `json:"link_next"`
			TotalPages int     `json:"total_pages"`
		} `json:"meta"`
	} `json:"data"`
}

// yskcomicsImages is /api/chapters/images/{slug}: the page URLs in order
type yskcomicsImages struct {
	Data []string `json:"data"`
}
