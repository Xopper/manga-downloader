// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/elboletaire/manga-downloader/http"
)

// ReadVagabond is a grabber for readbagabondo.com, a one-series site whose
// API made for Mihon (/api/mihon) lists the chapters with their page counts.
// Pages are /chapter-{number}/page-{n}.png on the image host that serves the
// series cover, so no request per chapter is needed.
type ReadVagabond struct {
	*Grabber
	manga *readVagabondManga
}

// ReadVagabondChapter is a chapter listed by the API
type ReadVagabondChapter struct {
	Chapter
	PageCount int
}

// readVagabondManga is the site's one series
type readVagabondManga struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Cover string `json:"cover"`
}

func NewReadVagabond(g *Grabber) *ReadVagabond {
	return &ReadVagabond{Grabber: g}
}

// Test returns true for any readbagabondo.com URL: the site has one series,
// so a chapter URL lists it too
func (r *ReadVagabond) Test() (bool, error) {
	return hostMatchesDomain(r.URL, "readbagabondo.com"), nil
}

// getJSON fetches an API path and decodes it into v
func (r ReadVagabond) getJSON(path string, v any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     r.BaseUrl() + path,
		Referer: r.BaseUrl(),
		Accept:  "application/json",
	})
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), v)
}

// fetchManga fetches the series once
func (r *ReadVagabond) fetchManga() (*readVagabondManga, error) {
	if r.manga != nil {
		return r.manga, nil
	}
	var mangas []readVagabondManga
	if err := r.getJSON("/api/mihon/mangas", &mangas); err != nil {
		return nil, err
	}
	if len(mangas) == 0 {
		return nil, errors.New("readbagabondo.com lists no series")
	}
	r.manga = &mangas[0]

	return r.manga, nil
}

// FetchTitle returns the series title
func (r *ReadVagabond) FetchTitle() (string, error) {
	manga, err := r.fetchManga()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(manga.Title), nil
}

// FetchChapters returns the chapters with their page counts
func (r *ReadVagabond) FetchChapters() (Filterables, []error) {
	manga, err := r.fetchManga()
	if err != nil {
		return nil, []error{err}
	}
	var list []struct {
		Number    float64 `json:"number"`
		Title     string  `json:"title"`
		Volume    int     `json:"volume"`
		PageCount int     `json:"pageCount"`
	}
	if err := r.getJSON(fmt.Sprintf("/api/mihon/mangas/%d/chapters", manga.ID), &list); err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(list))
	for _, c := range list {
		chapters = append(chapters, &ReadVagabondChapter{
			Chapter: Chapter{
				Number: c.Number,
				Title:  c.Title,
				Volume: c.Volume,
				Label:  Label{Style: LabelNumbered},
			},
			PageCount: c.PageCount,
		})
	}

	return chapters, nil
}

// FetchChapter builds the chapter's page URLs on the cover's image host
func (r *ReadVagabond) FetchChapter(f Filterable) (*Chapter, error) {
	rchap := f.(*ReadVagabondChapter)
	manga, err := r.fetchManga()
	if err != nil {
		return nil, err
	}
	cover, err := url.Parse(manga.Cover)
	if err != nil || cover.Host == "" {
		return nil, fmt.Errorf("no image host in the cover %q", manga.Cover)
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Volume:     rchap.Volume,
		Language:   "en",
		PagesCount: int64(rchap.PageCount),
	}
	for i := 1; i <= rchap.PageCount; i++ {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i),
			URL:    fmt.Sprintf("%s://%s/chapter-%v/page-%d.png", cover.Scheme, cover.Host, f.GetNumber(), i),
		})
	}

	return chapter, nil
}
