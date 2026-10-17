// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

const honkaiImpactDomain = "manga.honkaiimpact3.com"

// honkaiImpactBookRe captures a book id and, for a reader URL, its chapter
// id: /book/1011 is a series, /book/1011/3 its third chapter
var honkaiImpactBookRe = regexp.MustCompile(`(?i)^/book/(\d+)(?:/(\d+))?/?$`)

// HonkaiImpact is a grabber for manga.honkaiimpact3.com, miHoYo's official
// Honkai Impact 3rd comics. The series page renders its chapters with jQuery
// from /book/{id}/get_chapter, a JSON list the grabber reads directly; the
// reader prints every page as a lazy <img data-original> on hoyoverse's CDN.
type HonkaiImpact struct {
	*Grabber
	title string
}

// HonkaiImpactChapter is a chapter listed by get_chapter
type HonkaiImpactChapter struct {
	Chapter
	URL string
}

// honkaiImpactChapterDTO is one row of get_chapter; ids come as strings
type honkaiImpactChapterDTO struct {
	Title     string `json:"title"`
	BookID    string `json:"bookid"`
	ChapterID string `json:"chapterid"`
}

// NewHonkaiImpact returns a new HonkaiImpact grabber
func NewHonkaiImpact(g *Grabber) *HonkaiImpact {
	return &HonkaiImpact{Grabber: g}
}

// Test matches the site's domain
func (h *HonkaiImpact) Test() (bool, error) {
	return hostMatchesDomain(h.URL, honkaiImpactDomain), nil
}

// honkaiImpactBookID returns the book id of a series or reader URL
func honkaiImpactBookID(rawURL string) (string, bool) {
	_, path, ok := chapterPath(rawURL)
	if !ok {
		return "", false
	}
	m := honkaiImpactBookRe.FindStringSubmatch(path)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// seriesURL is the series page of the pasted URL: a reader URL is cut back
// to its /book/{id}, no fetch needed
func (h *HonkaiImpact) seriesURL() (string, error) {
	id, ok := honkaiImpactBookID(h.URL)
	if !ok {
		return "", errSeriesNotFound(h.URL)
	}
	return "https://" + honkaiImpactDomain + "/book/" + id, nil
}

// FetchTitle returns the series title (div.title on the series page)
func (h *HonkaiImpact) FetchTitle() (string, error) {
	if h.title != "" {
		return h.title, nil
	}
	uri, err := h.seriesURL()
	if err != nil {
		return "", err
	}
	body, err := http.Get(http.RequestParams{URL: uri})
	if err != nil {
		return "", err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}
	h.title = sanitizeTitle(doc.Find("div.title").First().Text())
	if h.title == "" {
		return "", fmt.Errorf("no title found in %s", uri)
	}
	return h.title, nil
}

// FetchChapters reads the get_chapter JSON list
func (h *HonkaiImpact) FetchChapters() (Filterables, []error) {
	uri, err := h.seriesURL()
	if err != nil {
		return nil, []error{err}
	}
	body, err := http.Get(http.RequestParams{URL: uri + "/get_chapter", Referer: uri, Accept: "application/json"})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()
	chapters, err := honkaiImpactParseChapters(body)
	if err != nil {
		return nil, []error{err}
	}
	return chapters, nil
}

// honkaiImpactParseChapters turns get_chapter into chapters. The site's
// titles carry their own running number ("13 His Reply - END" is chapterid
// 14, after a prologue), so the title is kept as is and the id numbers it.
func honkaiImpactParseChapters(r io.Reader) (Filterables, error) {
	var rows []honkaiImpactChapterDTO
	if err := json.NewDecoder(r).Decode(&rows); err != nil {
		return nil, err
	}
	var chapters Filterables
	for _, row := range rows {
		num, err := strconv.ParseFloat(row.ChapterID, 64)
		if err != nil {
			continue
		}
		chapters = append(chapters, &HonkaiImpactChapter{
			Chapter{
				Number:   num,
				Title:    sanitizeTitle(row.Title),
				Language: "en",
				ID:       row.BookID + "/" + row.ChapterID,
				Label:    Label{Style: LabelNumbered, Number: row.ChapterID},
			},
			"https://" + honkaiImpactDomain + "/book/" + row.BookID + "/" + row.ChapterID,
		})
	}
	return chapters, nil
}

// FetchChapter reads the reader's lazy images
func (h *HonkaiImpact) FetchChapter(f Filterable) (*Chapter, error) {
	hchap := f.(*HonkaiImpactChapter)
	body, err := http.Get(http.RequestParams{URL: hchap.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	pages := honkaiImpactPages(doc)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		ID:         hchap.ID,
		Pages:      pages,
	}, nil
}

// honkaiImpactPages reads img.comic_img's data-original (src is a spinner)
func honkaiImpactPages(doc *goquery.Document) []Page {
	var pages []Page
	doc.Find("img.comic_img").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("data-original", ""))
		if src == "" {
			return
		}
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src})
	})
	return pages
}
