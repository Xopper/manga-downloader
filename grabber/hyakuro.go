// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Hyakuro is a grabber for hyakuro.net (Hyakuro Translations), a Next.js
// front for a Strapi CMS served at /backend/api. The series page's
// __NEXT_DATA__ lists chapters but no pages, so everything comes from
// Strapi: one /mangas query filtered by slug, with the chapters' page media
// populated, gives the title, the chapter list and every page at once.
// Media URLs are relative to /backend. Page order isn't stored, so pages
// are sorted by their uploaded file names ("... Page 001.png").
type Hyakuro struct {
	*Grabber
	manga *hyakuroManga
}

// HyakuroChapter represents a Hyakuro chapter
type HyakuroChapter struct {
	Chapter
	// Pages are the chapter's image URLs, in order
	Pages []string
}

// NewHyakuro returns a Hyakuro grabber
func NewHyakuro(g *Grabber) *Hyakuro {
	return &Hyakuro{Grabber: g}
}

// hyakuroPathRe matches series, reader (/manga/{slug}/read/{n}/{page}) and
// download (/manga/{slug}/download/{n}) paths and captures the slug
var hyakuroPathRe = regexp.MustCompile(`^/manga/([^/]+)(/.*)?$`)

// Test returns true for hyakuro.net URLs
func (h *Hyakuro) Test() (bool, error) {
	host, _, ok := chapterPath(h.URL)
	return ok && host == "hyakuro.net", nil
}

// resolveSeriesURL rewrites a reader or download URL to its series URL.
// No-op for series URLs.
func (h *Hyakuro) resolveSeriesURL() {
	u, err := url.Parse(h.URL)
	if err != nil {
		return
	}
	m := hyakuroPathRe.FindStringSubmatch(u.Path)
	if m == nil || m[2] == "" || m[2] == "/" {
		return
	}
	u.Path = "/manga/" + m[1]
	u.RawQuery = ""
	u.Fragment = ""
	h.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (h *Hyakuro) FetchTitle() (string, error) {
	manga, err := h.fetchManga()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(manga.Title), nil
}

// FetchChapters returns the chapters of the manga
func (h *Hyakuro) FetchChapters() (chapters Filterables, errs []error) {
	manga, err := h.fetchManga()
	if err != nil {
		return nil, []error{err}
	}
	return hyakuroChapters(manga, h.BaseUrl()), nil
}

// hyakuroChapters builds the listing, each chapter with its sorted pages
func hyakuroChapters(manga *hyakuroManga, base string) (chapters Filterables) {
	for _, c := range manga.Chapters {
		files := c.Pages.Data
		sort.SliceStable(files, func(i, j int) bool {
			return naturalLess(files[i].Attributes.Name, files[j].Attributes.Name)
		})
		chapter := &HyakuroChapter{Chapter: Chapter{
			Number:     c.Chapter,
			ID:         strconv.FormatInt(c.ID, 10),
			Language:   "en",
			PagesCount: int64(len(files)),
			Label:      Label{Style: LabelPrefixed},
		}}
		if c.Title != nil {
			chapter.Title = sanitizeTitle(*c.Title)
		}
		if c.Volume != nil {
			chapter.Volume = int(*c.Volume)
		}
		for _, f := range files {
			chapter.Pages = append(chapter.Pages, base+"/backend"+f.Attributes.URL)
		}
		chapters = append(chapters, chapter)
	}
	return chapters
}

// FetchChapter returns a chapter with the pages the listing already holds
func (h *Hyakuro) FetchChapter(f Filterable) (*Chapter, error) {
	hchap := f.(*HyakuroChapter)
	if len(hchap.Pages) == 0 {
		return nil, fmt.Errorf("chapter %s has no pages", hchap.ID)
	}
	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Label:      labelOf(f),
		Language:   hchap.Language,
		Volume:     hchap.Volume,
		ID:         hchap.ID,
		PagesCount: int64(len(hchap.Pages)),
	}
	for i, p := range hchap.Pages {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: p})
	}
	return chapter, nil
}

// fetchManga queries Strapi for the series, its chapters and their pages,
// once
func (h *Hyakuro) fetchManga() (*hyakuroManga, error) {
	h.resolveSeriesURL()
	if h.manga != nil {
		return h.manga, nil
	}
	u, err := url.Parse(h.URL)
	if err != nil {
		return nil, err
	}
	m := hyakuroPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, errSeriesNotFound(h.URL)
	}

	q := url.Values{
		"filters[slug][$eq]":           {m[1]},
		"populate[Chapters][populate]": {"*"},
	}
	body, err := http.Get(http.RequestParams{
		URL:     h.BaseUrl() + "/backend/api/mangas?" + q.Encode(),
		Referer: h.URL,
		Accept:  "application/json",
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	resp := struct {
		Data []struct {
			Attributes hyakuroManga `json:"attributes"`
		} `json:"data"`
	}{}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, errSeriesNotFound(h.URL)
	}
	h.manga = &resp.Data[0].Attributes
	return h.manga, nil
}

// hyakuroManga is a Strapi manga entry with its chapters populated
type hyakuroManga struct {
	Title    string `json:"Title"`
	Chapters []struct {
		ID      int64    `json:"id"`
		Title   *string  `json:"Title"`
		Chapter float64  `json:"Chapter"`
		Volume  *float64 `json:"Volume"`
		Pages   struct {
			Data []struct {
				Attributes struct {
					Name string `json:"name"`
					URL  string `json:"url"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"Pages"`
	} `json:"Chapters"`
}
