// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"sync"

	"github.com/elboletaire/manga-downloader/http"
)

// ComicsKingdom is a grabber for comicskingdom.com, whose newspaper strips
// live in the WordPress REST API of wp.comicskingdom.com: a feature (the
// comic, /{slug}) is a ck_feature term, and each day's strip a ck_comic post
// filed under it, with its picture in assets.featured. Every strip is a
// chapter, oldest first.
type ComicsKingdom struct {
	*Grabber
	slug string
}

// ComicsKingdomChapter is a strip of a Comics Kingdom feature
type ComicsKingdomChapter struct {
	Chapter
	Picture string
}

// NewComicsKingdom returns a new ComicsKingdom grabber
func NewComicsKingdom(g *Grabber) *ComicsKingdom {
	return &ComicsKingdom{Grabber: g}
}

// comicsKingdomAPI is the REST API's root
var comicsKingdomAPI = "https://wp.comicskingdom.com/wp-json/wp/v2/"

// Test tells whether the URL is a Comics Kingdom feature, and reads its slug
func (c *ComicsKingdom) Test() (bool, error) {
	if !hostMatchesDomain(c.URL, "comicskingdom.com") {
		return false, nil
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return false, nil
	}
	c.slug, _, _ = strings.Cut(strings.Trim(u.Path, "/"), "/")
	return c.slug != "", nil
}

// get asks the REST API for path and decodes it into v
func (c *ComicsKingdom) get(path string, v any) error {
	body, err := http.GetText(http.RequestParams{URL: comicsKingdomAPI + path, Referer: "https://comicskingdom.com/"})
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), v)
}

// FetchTitle returns the feature's name
func (c *ComicsKingdom) FetchTitle() (string, error) {
	var features []struct {
		Title struct {
			Rendered string `json:"rendered"`
		} `json:"title"`
	}
	if err := c.get("ck_feature?slug="+url.QueryEscape(c.slug)+"&_fields=title", &features); err != nil {
		return "", err
	}
	if len(features) == 0 {
		return "", errSeriesNotFound(c.URL)
	}
	return sanitizeTitle(html.UnescapeString(features[0].Title.Rendered)), nil
}

// ckMaxPages bounds the strip list's pages (100 strips each)
const ckMaxPages = 400

// ckParallel is how many pages of the strip list are asked for at once: a
// feature can have ten thousand strips, and one page takes about a second
const ckParallel = 4

// ckStrip is the part of a strip the grabber reads
type ckStrip struct {
	ID    int `json:"id"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Assets struct {
		Featured struct {
			URL string `json:"url"`
		} `json:"featured"`
	} `json:"assets"`
}

// stripsPage reads a page of the feature's strips; past the last page,
// which WordPress answers with a 400, it's empty
func (c *ComicsKingdom) stripsPage(page int) ([]ckStrip, error) {
	var strips []ckStrip
	err := c.get(fmt.Sprintf("ck_comic?ck_feature=%s&per_page=%d&page=%d&orderby=date&order=asc&_fields=id,title,assets.featured.url",
		url.QueryEscape(c.slug), wpPerPage, page), &strips)
	var statusErr *http.StatusError
	if page > 1 && errors.As(err, &statusErr) && statusErr.Code == 400 {
		return nil, nil
	}
	return strips, err
}

// FetchChapters returns the feature's strips, oldest first, each named after
// its date. The list's pages are read a few at a time, until one comes back short.
func (c *ComicsKingdom) FetchChapters() (Filterables, []error) {
	var chapters Filterables
	for first := 1; first <= ckMaxPages; first += ckParallel {
		pages := make([][]ckStrip, ckParallel)
		errs := make([]error, ckParallel)
		var wg sync.WaitGroup
		for i := range ckParallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pages[i], errs[i] = c.stripsPage(first + i)
			}()
		}
		wg.Wait()
		done := false
		for i, strips := range pages {
			if errs[i] != nil {
				return chapters, []error{errs[i]}
			}
			for _, s := range strips {
				if s.Assets.Featured.URL == "" {
					continue
				}
				ch := wcStripChapter(wcStrip{Title: sanitizeTitle(html.UnescapeString(s.Title.Rendered))}, len(chapters)+1)
				ch.ID = fmt.Sprint(s.ID)
				ch.Language = "en"
				chapters = append(chapters, &ComicsKingdomChapter{Chapter: ch, Picture: s.Assets.Featured.URL})
			}
			if len(strips) < wpPerPage {
				done = true
				break
			}
		}
		if done {
			break
		}
	}
	if len(chapters) == 0 {
		return nil, []error{errNoStrips(c.URL)}
	}
	return chapters, nil
}

// FetchChapter returns a strip's picture, which the list gave
func (c *ComicsKingdom) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*ComicsKingdomChapter)
	return &Chapter{
		Title:      ch.Title,
		Number:     ch.Number,
		Label:      ch.Label,
		ID:         ch.ID,
		PagesCount: 1,
		Language:   "en",
		Pages:      []Page{{Number: 1, URL: ch.Picture}},
	}, nil
}
