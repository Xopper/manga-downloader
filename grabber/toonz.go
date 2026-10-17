// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Toonz is a grabber for toonz.to, a Next.js App Router site (series under
// /manhwa/, /manga/ or /manhua/{slug}). The series page only embeds the
// newest chapters in its RSC payload; the client loads the rest from
// /api/comics/{id}/chapters, which answers the whole list in one go, so the
// series page is read just for the comic's numeric id and title. Reader pages
// carry their images in the RSC payload too, as file names relative to the
// site's own /uploads/ folder.
type Toonz struct {
	*Grabber
	series *toonzSeries
}

// ToonzChapter represents a Toonz chapter
type ToonzChapter struct {
	Chapter
	// URL is the chapter's reader page
	URL string
	// ComicID is the series' numeric id, part of the image paths
	ComicID int64
}

// toonzSeries is what the series page says about the comic
type toonzSeries struct {
	ID    int64
	Title string
}

// NewToonz returns a Toonz grabber
func NewToonz(g *Grabber) *Toonz {
	return &Toonz{Grabber: g}
}

// toonzChapterRe matches reader paths (/{type}/{slug}/chapter/{number})
var toonzChapterRe = regexp.MustCompile(`^(/[^/]+/[^/]+)/chapter/[^/]+/?$`)

// toonzComicRe pulls the comic's id and title from the props of the series
// page's client components, which always list them in this order
var toonzComicRe = regexp.MustCompile(`"comicId":(\d+),"comicSlug":"[^"]*","comicType":"[^"]*","comicTitle":("(?:[^"\\]|\\.)*")`)

// Test returns true for toonz.to URLs
func (t *Toonz) Test() (bool, error) {
	host, _, ok := chapterPath(t.URL)
	return ok && host == "toonz.to", nil
}

// resolveSeriesURL rewrites a reader URL to its series URL. No-op for series
// URLs.
func (t *Toonz) resolveSeriesURL() {
	u, err := url.Parse(t.URL)
	if err != nil {
		return
	}
	m := toonzChapterRe.FindStringSubmatch(u.Path)
	if m == nil {
		return
	}
	u.Path = m[1]
	u.RawQuery = ""
	u.Fragment = ""
	t.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (t *Toonz) FetchTitle() (string, error) {
	series, err := t.fetchSeries()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(series.Title), nil
}

// FetchChapters returns the chapters of the manga
func (t *Toonz) FetchChapters() (chapters Filterables, errs []error) {
	series, err := t.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	body, err := http.Get(http.RequestParams{
		URL:     fmt.Sprintf("%s/api/comics/%d/chapters", t.BaseUrl(), series.ID),
		Referer: t.URL,
		Accept:  "application/json",
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	feed := toonzChapterList{}
	if err := json.NewDecoder(body).Decode(&feed); err != nil {
		return nil, []error{err}
	}

	u, _ := url.Parse(t.URL)
	seriesPath := strings.TrimSuffix(u.Path, "/")
	for _, c := range feed.Chapters {
		number, err := strconv.ParseFloat(c.ChapterNumber, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not parse chapter number %q: %w", c.ChapterNumber, err))
			continue
		}
		title := ""
		if c.Title != nil {
			title = sanitizeTitle(*c.Title)
		}
		chapters = append(chapters, &ToonzChapter{
			Chapter: Chapter{
				Number:     number,
				Title:      title,
				PagesCount: c.PageCount,
				ID:         strconv.FormatInt(c.ID, 10),
				Label:      Label{Style: LabelPrefixed},
			},
			// the reader's links drop the slug's "chapter-" prefix
			URL:     t.BaseUrl() + seriesPath + "/chapter/" + strings.TrimPrefix(c.Slug, "chapter-"),
			ComicID: series.ID,
		})
	}

	return chapters, errs
}

// FetchChapter fetches a chapter and its pages
func (t *Toonz) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*ToonzChapter)

	html, err := http.GetText(http.RequestParams{URL: tchap.URL, Referer: t.URL})
	if err != nil {
		return nil, err
	}
	reader, err := toonzReaderChapter(html, tchap.ID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tchap.URL, err)
	}

	chapter := &Chapter{
		Title:  f.GetTitle(),
		Number: f.GetNumber(),
		Label:  labelOf(f),
		ID:     tchap.ID,
	}
	for i, img := range reader.Images {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    toonzImageURL(t.BaseUrl(), tchap.ComicID, tchap.ID, img.Filename),
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// toonzReaderChapter reads the chapter object (with its images, sorted) out
// of a reader page's RSC payload. The id pins the object, since the payload
// also mentions the previous and next chapters.
func toonzReaderChapter(html, id string) (*toonzReader, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}
	idx := strings.Index(stream, `"chapter":{"id":`+id+`,`)
	if idx == -1 {
		return nil, fmt.Errorf("chapter %s not found in the reader data", id)
	}
	raw, err := extractBalancedJSON(stream[idx:], `"chapter":`)
	if err != nil {
		return nil, err
	}
	reader := &toonzReader{}
	if err := json.Unmarshal([]byte(raw), reader); err != nil {
		return nil, err
	}
	if len(reader.Images) == 0 {
		return nil, fmt.Errorf("chapter %s has no images", id)
	}
	sort.SliceStable(reader.Images, func(i, j int) bool {
		return reader.Images[i].SortOrder < reader.Images[j].SortOrder
	})
	return reader, nil
}

// toonzImageURL builds an image URL the way the reader does: a file name
// holding a slash is a path under /uploads/, a bare one sits in the chapter's
// own folder
func toonzImageURL(base string, comicID int64, chapterID, filename string) string {
	if strings.Contains(filename, "/") {
		return base + "/uploads/" + filename
	}
	return fmt.Sprintf("%s/uploads/chapters/%d/%s/%s", base, comicID, chapterID, filename)
}

// fetchSeries reads the comic's id and title off the series page, once
func (t *Toonz) fetchSeries() (*toonzSeries, error) {
	t.resolveSeriesURL()
	if t.series != nil {
		return t.series, nil
	}

	html, err := http.GetText(http.RequestParams{URL: t.URL})
	if err != nil {
		return nil, err
	}
	series, err := toonzParseSeries(html)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.URL, err)
	}
	t.series = series
	return series, nil
}

// toonzParseSeries extracts the comic's id and title from a series page
func toonzParseSeries(html string) (*toonzSeries, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}
	m := toonzComicRe.FindStringSubmatch(stream)
	if m == nil {
		return nil, fmt.Errorf("could not find the comic id in the page (is the URL a series page?)")
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return nil, err
	}
	series := &toonzSeries{ID: id}
	if err := json.Unmarshal([]byte(m[2]), &series.Title); err != nil {
		return nil, err
	}
	return series, nil
}

// toonzChapterList is the /api/comics/{id}/chapters response
type toonzChapterList struct {
	Chapters []struct {
		ID int64 `json:"id"`
		// ChapterNumber is a decimal string ("0.01", "12.00")
		ChapterNumber string  `json:"chapterNumber"`
		Title         *string `json:"title"`
		Slug          string  `json:"slug"`
		PageCount     int64   `json:"pageCount"`
	} `json:"chapters"`
}

// toonzReader is the chapter object of a reader page's RSC payload
type toonzReader struct {
	Images []struct {
		Filename  string `json:"filename"`
		SortOrder int    `json:"sortOrder"`
	} `json:"images"`
}
