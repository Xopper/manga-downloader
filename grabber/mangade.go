// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// MangaDE is a grabber for mangade.io: its public API (api.mangade.io)
// gives a comic with all its chapters, and a chapter with its pictures. The
// comic id ends the series slug (/comic/{slug}-pid{id}; chapter links drop
// the "pid").
type MangaDE struct {
	*Grabber
	comic *mangadeComic
}

// MangaDEChapter is a chapter of the comic
type MangaDEChapter struct {
	Chapter
	APIID string
}

// mangadeComic is the part of a comic the API gives
type mangadeComic struct {
	Name     string `json:"name"`
	Chapters []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Number string `json:"chapter_number"`
	} `json:"news_chapters"`
}

var (
	// mangadeAPI is the site's API, a var so tests can point it elsewhere
	mangadeAPI = "https://api.mangade.io/api"
	// mangadeComicRe captures the comic id of a series or chapter path
	mangadeComicRe = regexp.MustCompile(`^/comic/[^/]*?-(?:pid)?(\d+)(?:/|$)`)
)

func NewMangaDE(g *Grabber) *MangaDE {
	return &MangaDE{Grabber: g}
}

// Test returns true for a mangade.io comic or chapter URL
func (m *MangaDE) Test() (bool, error) {
	return hostMatchesDomain(m.URL, "mangade.io") && m.comicID() != "", nil
}

// comicID returns the URL's comic id
func (m MangaDE) comicID() string {
	u, err := url.Parse(m.URL)
	if err != nil {
		return ""
	}
	matches := mangadeComicRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return ""
	}
	return matches[1]
}

// getJSON fetches an API path and decodes its data into v
func (m MangaDE) getJSON(path string, v any) error {
	body, err := http.GetText(http.RequestParams{URL: mangadeAPI + path, Referer: m.BaseUrl(), Accept: "application/json"})
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), &struct {
		Data any `json:"data"`
	}{v})
}

// fetchComic fetches the comic once
func (m *MangaDE) fetchComic() (*mangadeComic, error) {
	if m.comic == nil {
		comic := &mangadeComic{}
		if err := m.getJSON("/comics/"+m.comicID()+"/view", comic); err != nil {
			return nil, err
		}
		m.comic = comic
	}
	return m.comic, nil
}

// FetchTitle returns the comic's name
func (m *MangaDE) FetchTitle() (string, error) {
	comic, err := m.fetchComic()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(comic.Name), nil
}

// FetchChapters returns the comic's chapters
func (m *MangaDE) FetchChapters() (Filterables, []error) {
	comic, err := m.fetchComic()
	if err != nil {
		return nil, []error{err}
	}
	chapters := make(Filterables, 0, len(comic.Chapters))
	for _, c := range comic.Chapters {
		number, err := strconv.ParseFloat(c.Number, 64)
		if err != nil {
			continue
		}
		title := c.Name
		if title == NumberTextLabel(c.Number) {
			title = ""
		}
		chapters = append(chapters, &MangaDEChapter{
			Chapter: Chapter{Number: number, Title: title, ID: c.ID, Label: Label{Style: LabelNumbered}},
			APIID:   c.ID,
		})
	}
	return chapters, nil
}

// FetchChapter returns the chapter's pictures
func (m *MangaDE) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaDEChapter)
	var data struct {
		Images []struct {
			Image string `json:"image"`
		} `json:"chapter_images"`
	}
	if err := m.getJSON("/chapters/"+mchap.APIID+"/view", &data); err != nil {
		return nil, err
	}
	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   "en",
		ID:         mchap.ID,
		PagesCount: int64(len(data.Images)),
	}
	for i, img := range data.Images {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img.Image})
	}
	return chapter, nil
}
