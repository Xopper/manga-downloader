// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Lustoon is a grabber for lustoon.com, a Next.js App Router site built on a
// Spanish scanlation CMS (its data keys are Spanish: "serie", "sinopsis",
// "pageches"). Everything is server rendered into the RSC payload: the series
// page holds a "serie" object with the whole chapter list, and a reader page
// a "pageches" object whose urlImg is a JSON array encoded as a string. The
// CMS's own "idioma" field says "es" on English series, so no language is
// set. Chapters carry a "patreon" flag, presumably for patron-only releases,
// but no live listing sets it, so it doesn't lock anything yet.
type Lustoon struct {
	*Grabber
	serie *lustoonSerie
}

// LustoonChapter represents a Lustoon chapter
type LustoonChapter struct {
	Chapter
	// URL is the chapter's reader page
	URL string
}

// NewLustoon returns a Lustoon grabber
func NewLustoon(g *Grabber) *Lustoon {
	return &Lustoon{Grabber: g}
}

// lustoonPathRe matches series (/comic/{slug}) and reader
// (/comic/{slug}/{chapter-slug}) paths
var lustoonPathRe = regexp.MustCompile(`^/comic/([^/]+)(/[^/]+)?/?$`)

// Test returns true for lustoon.com URLs
func (l *Lustoon) Test() (bool, error) {
	host, _, ok := chapterPath(l.URL)
	return ok && host == "lustoon.com", nil
}

// resolveSeriesURL rewrites a reader URL to its series URL. No-op for series
// URLs.
func (l *Lustoon) resolveSeriesURL() {
	u, err := url.Parse(l.URL)
	if err != nil {
		return
	}
	m := lustoonPathRe.FindStringSubmatch(u.Path)
	if m == nil || m[2] == "" {
		return
	}
	u.Path = "/comic/" + m[1]
	u.RawQuery = ""
	u.Fragment = ""
	l.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (l *Lustoon) FetchTitle() (string, error) {
	serie, err := l.fetchSerie()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(serie.Name), nil
}

// FetchChapters returns the chapters of the manga
func (l *Lustoon) FetchChapters() (chapters Filterables, errs []error) {
	serie, err := l.fetchSerie()
	if err != nil {
		return nil, []error{err}
	}
	for _, c := range serie.Chapters {
		chapter := &LustoonChapter{
			Chapter: Chapter{
				Number: c.Num,
				ID:     strconv.FormatInt(c.ID, 10),
				Label:  Label{Style: LabelPrefixed},
			},
			URL: l.BaseUrl() + "/comic/" + serie.Slug + "/" + c.Slug,
		}
		if c.Name != nil {
			chapter.Title = sanitizeTitle(*c.Name)
		}
		chapters = append(chapters, chapter)
	}
	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (l *Lustoon) FetchChapter(f Filterable) (*Chapter, error) {
	lchap := f.(*LustoonChapter)

	html, err := http.GetText(http.RequestParams{URL: lchap.URL, Referer: l.URL})
	if err != nil {
		return nil, err
	}
	images, err := lustoonPages(html)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", lchap.URL, err)
	}

	chapter := &Chapter{
		Title:  f.GetTitle(),
		Number: f.GetNumber(),
		Label:  labelOf(f),
		ID:     lchap.ID,
	}
	for i, img := range images {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img})
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}

// lustoonPages reads the image list out of a reader page's RSC payload
func lustoonPages(html string) ([]string, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}
	raw, err := extractBalancedJSON(stream, `"pageches":`)
	if err != nil {
		return nil, err
	}
	pageches := struct {
		URLImg string `json:"urlImg"`
	}{}
	if err := json.Unmarshal([]byte(raw), &pageches); err != nil {
		return nil, err
	}
	list := pageches.URLImg
	// a long list is sent as its own text row and referenced as "$<id>"
	if strings.HasPrefix(list, "$") {
		if list, err = lustoonTextRow(stream, list[1:]); err != nil {
			return nil, err
		}
	}
	var images []string
	if err := json.Unmarshal([]byte(list), &images); err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("the chapter has no images")
	}
	// the CMS stores http:// URLs, but the media host answers plain http
	// with a Cloudflare 521; the browser only gets the images as https
	for i, img := range images {
		if rest, ok := strings.CutPrefix(img, "http://"); ok {
			images[i] = "https://" + rest
		}
	}
	return images, nil
}

// lustoonTextRow returns the RSC text row with the given id: a line
// "{id}:T{hex byte length},{text}" of the flight stream
func lustoonTextRow(stream, id string) (string, error) {
	marker := "\n" + id + ":T"
	idx := strings.Index(stream, marker)
	if idx == -1 {
		return "", fmt.Errorf("text row %q not found in the page data", id)
	}
	rest := stream[idx+len(marker):]
	hexLen, text, ok := strings.Cut(rest, ",")
	if !ok {
		return "", fmt.Errorf("malformed text row %q", id)
	}
	n, err := strconv.ParseInt(hexLen, 16, 64)
	if err != nil || int(n) > len(text) {
		return "", fmt.Errorf("malformed text row %q", id)
	}
	return text[:n], nil
}

// fetchSerie reads the "serie" object off the series page, once
func (l *Lustoon) fetchSerie() (*lustoonSerie, error) {
	l.resolveSeriesURL()
	if l.serie != nil {
		return l.serie, nil
	}
	html, err := http.GetText(http.RequestParams{URL: l.URL})
	if err != nil {
		return nil, err
	}
	serie, err := lustoonParseSerie(html)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", l.URL, err)
	}
	l.serie = serie
	return serie, nil
}

// lustoonParseSerie extracts the series object from a series page
func lustoonParseSerie(html string) (*lustoonSerie, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}
	raw, err := extractBalancedJSON(stream, `"serie":`)
	if err != nil {
		return nil, err
	}
	serie := &lustoonSerie{}
	if err := json.Unmarshal([]byte(raw), serie); err != nil {
		return nil, err
	}
	if strings.TrimSpace(serie.Slug) == "" {
		return nil, fmt.Errorf("no series data in the page (is the URL a series page?)")
	}
	return serie, nil
}

// lustoonSerie is the series object of a series page's RSC payload
type lustoonSerie struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Chapters []struct {
		ID      int64   `json:"id"`
		Num     float64 `json:"num"`
		Name    *string `json:"name"`
		Slug    string  `json:"slug"`
		Patreon int     `json:"patreon"`
	} `json:"chapters"`
}
