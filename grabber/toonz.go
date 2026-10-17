// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

var (
	toonzHostRe    = regexp.MustCompile(`(?i)^https?://(?:www\.)?toonz\.to(?:[/?#]|$)`)
	toonzSeriesRe  = regexp.MustCompile(`^/([a-z]+)/([^/?#]+)`)
	toonzComicIDRe = regexp.MustCompile(`comicId[^:]{0,4}:(\d+)`)
	toonzTitleRe   = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	toonzTagRe     = regexp.MustCompile(`<[^>]+>`)
)

// toonzCookie lifts the reader's rating filter, which otherwise hides the
// adult series and their chapters
const toonzCookie = "reader_prefs=%7B%22rating%22%3A%22pornographic%22%7D"

// Toonz is a grabber for toonz.to, a Next.js manhwa site. The series page
// only holds the comic id (in its RSC payload); the chapters come from
// /api/comics/{id}/chapters, and a chapter's picture list from its reader
// page asked for as an RSC payload ("rsc: 1"), whose images are files under
// /uploads/.
type Toonz struct {
	*Grabber
	seriesURL string
	title     string
	comicID   string
}

func NewToonz(g *Grabber) *Toonz {
	return &Toonz{Grabber: g}
}

// ToonzChapter is a toonz.to chapter; Path is its reader page
type ToonzChapter struct {
	Chapter
	Path string
}

// toonzChapterJSON is a chapter of the chapters API
type toonzChapterJSON struct {
	ID            int     `json:"id"`
	ChapterNumber string  `json:"chapterNumber"`
	Title         *string `json:"title"`
	Slug          string  `json:"slug"`
}

// Test returns true for toonz.to URLs
func (t *Toonz) Test() (bool, error) {
	return toonzHostRe.MatchString(t.URL), nil
}

// toonzSeriesPath trims a chapter path (/manhwa/{slug}/chapter/{n}) back to
// its series (/manhwa/{slug}); the site links /manhua/ for /manhwa/
func toonzSeriesPath(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	m := toonzSeriesRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errSeriesNotFound(rawURL)
	}
	kind := m[1]
	if kind == "manhua" {
		kind = "manhwa"
	}

	return "/" + kind + "/" + m[2], nil
}

func (t *Toonz) headers() map[string]string {
	return map[string]string{"Cookie": toonzCookie}
}

// fetchSeries reads the title and comic id off the series page, once
func (t *Toonz) fetchSeries() error {
	if t.comicID != "" {
		return nil
	}
	path, err := toonzSeriesPath(t.URL)
	if err != nil {
		return err
	}
	t.seriesURL = t.BaseUrl() + path
	page, err := http.GetText(http.RequestParams{URL: t.seriesURL, Headers: t.headers()})
	if err != nil {
		return err
	}
	m := toonzComicIDRe.FindStringSubmatch(strings.ReplaceAll(page, `\"`, `"`))
	if m == nil {
		return fmt.Errorf("could not find the comic id in %s", t.seriesURL)
	}
	t.comicID = m[1]
	if h := toonzTitleRe.FindStringSubmatch(page); h != nil {
		t.title = html.UnescapeString(toonzTagRe.ReplaceAllString(h[1], ""))
	}

	return nil
}

// FetchTitle returns the series title
func (t *Toonz) FetchTitle() (string, error) {
	if err := t.fetchSeries(); err != nil {
		return "", err
	}

	return sanitizeTitle(t.title), nil
}

// FetchChapters returns the series' chapters from the chapters API
func (t *Toonz) FetchChapters() (Filterables, []error) {
	if err := t.fetchSeries(); err != nil {
		return nil, []error{err}
	}
	text, err := http.GetText(http.RequestParams{URL: t.BaseUrl() + "/api/comics/" + t.comicID + "/chapters", Headers: t.headers()})
	if err != nil {
		return nil, []error{err}
	}
	list := struct {
		Chapters []toonzChapterJSON `json:"chapters"`
	}{}
	if err = json.Unmarshal([]byte(text), &list); err != nil {
		return nil, []error{err}
	}

	return toonzChapters(t.seriesURL, list.Chapters), nil
}

func toonzChapters(seriesURL string, list []toonzChapterJSON) (chapters Filterables) {
	for _, c := range list {
		number, _ := strconv.ParseFloat(c.ChapterNumber, 64)
		title := ""
		if c.Title != nil {
			title = *c.Title
		}
		chapters = append(chapters, &ToonzChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    sanitizeTitle(title),
				ID:       strconv.Itoa(c.ID),
				Language: "en",
				Label:    Label{Style: LabelPrefixed, Number: strconv.FormatFloat(number, 'f', -1, 64)},
			},
			Path: seriesURL + "/chapter/" + strings.TrimPrefix(c.Slug, "chapter-"),
		})
	}

	return chapters
}

// FetchChapter reads the chapter's pictures off its reader's RSC payload
func (t *Toonz) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*ToonzChapter)
	headers := t.headers()
	headers["rsc"] = "1"
	payload, err := http.GetText(http.RequestParams{URL: tchap.Path, Headers: headers})
	if err != nil {
		return nil, err
	}
	files, err := toonzImages(payload)
	if err != nil {
		return nil, fmt.Errorf("%w in %s", err, tchap.Path)
	}

	chapter := &Chapter{
		Title:    tchap.GetTitle(),
		Number:   tchap.GetNumber(),
		Language: tchap.Language,
		ID:       tchap.ID,
		Label:    labelOf(f),
	}
	for i, file := range files {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    t.BaseUrl() + "/uploads/" + strings.TrimPrefix(file, "/"),
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// toonzImages decodes the first "images" array of an RSC payload
func toonzImages(payload string) ([]string, error) {
	i := strings.Index(payload, `"images":[`)
	if i < 0 {
		return nil, errors.New("no picture list")
	}
	images := []struct {
		Filename string `json:"filename"`
	}{}
	dec := json.NewDecoder(bytes.NewReader([]byte(payload[i+len(`"images":`):])))
	if err := dec.Decode(&images); err != nil {
		return nil, err
	}
	var files []string
	for _, img := range images {
		if img.Filename != "" {
			files = append(files, img.Filename)
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no pictures")
	}

	return files, nil
}
