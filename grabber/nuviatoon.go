// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Nuviatoon is a grabber for nuviatoon.net, a Next.js site with comics under
// /comics/{slug} (and novels, which aren't downloadable, under /novels/).
// Its reader renders no images server side: the client asks the site's own
// JSON API, /api/comic-chapters/{id}/content, which answers presigned R2
// URLs, or a CHAPTER_LOCKED error for chapters bought with coins. The series
// endpoint, /api/comics/{slug}, lists every chapter with its priceCoins, so
// paid ones are marked locked.
type Nuviatoon struct {
	*Grabber
	comic *nuviatoonComic
}

// NewNuviatoon returns a Nuviatoon grabber
func NewNuviatoon(g *Grabber) *Nuviatoon {
	return &Nuviatoon{Grabber: g}
}

// nuviatoonPathRe matches series (/comics/{slug}) and reader
// (/comics/{slug}/chapter/{number}) paths
var nuviatoonPathRe = regexp.MustCompile(`^/comics/([^/]+)(/chapter/[^/]+)?/?$`)

// Test returns true for nuviatoon.net URLs
func (n *Nuviatoon) Test() (bool, error) {
	host, _, ok := chapterPath(n.URL)
	return ok && host == "nuviatoon.net", nil
}

// resolveSeriesURL rewrites a reader URL to its series URL. No-op for series
// URLs.
func (n *Nuviatoon) resolveSeriesURL() {
	u, err := url.Parse(n.URL)
	if err != nil {
		return
	}
	m := nuviatoonPathRe.FindStringSubmatch(u.Path)
	if m == nil || m[2] == "" {
		return
	}
	u.Path = "/comics/" + m[1]
	u.RawQuery = ""
	u.Fragment = ""
	n.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (n *Nuviatoon) FetchTitle() (string, error) {
	comic, err := n.fetchComic()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(comic.Title), nil
}

// FetchChapters returns the chapters of the manga
func (n *Nuviatoon) FetchChapters() (chapters Filterables, errs []error) {
	comic, err := n.fetchComic()
	if err != nil {
		return nil, []error{err}
	}
	for _, c := range comic.Chapters {
		chapter := &Chapter{
			Number: c.Number,
			ID:     c.ID,
			Locked: c.PriceCoins > 0,
			Label:  Label{Style: LabelPrefixed},
		}
		if c.Title != nil {
			chapter.Title = sanitizeTitle(*c.Title)
		}
		chapters = append(chapters, chapter)
	}
	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (n *Nuviatoon) FetchChapter(f Filterable) (*Chapter, error) {
	id := f.(*Chapter).ID

	resp := struct {
		Data struct {
			Pages []struct {
				URL string `json:"url"`
			} `json:"pages"`
		} `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	if err := n.get("/api/comic-chapters/"+url.PathEscape(id)+"/content", &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("chapter %s: %s (%s)", strconv.FormatFloat(f.GetNumber(), 'f', -1, 64), resp.Error.Message, resp.Error.Code)
	}
	if len(resp.Data.Pages) == 0 {
		return nil, fmt.Errorf("chapter %s has no pages", id)
	}

	chapter := &Chapter{
		Title:  f.GetTitle(),
		Number: f.GetNumber(),
		Label:  labelOf(f),
		ID:     id,
	}
	for i, p := range resp.Data.Pages {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: p.URL})
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}

// fetchComic loads the series record, once
func (n *Nuviatoon) fetchComic() (*nuviatoonComic, error) {
	n.resolveSeriesURL()
	if n.comic != nil {
		return n.comic, nil
	}
	u, err := url.Parse(n.URL)
	if err != nil {
		return nil, err
	}
	m := nuviatoonPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, errSeriesNotFound(n.URL)
	}
	resp := struct {
		Data *nuviatoonComic `json:"data"`
	}{}
	if err := n.get("/api/comics/"+url.PathEscape(m[1]), &resp); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		return nil, errSeriesNotFound(n.URL)
	}
	n.comic = resp.Data
	return n.comic, nil
}

// get calls the site's API. Its errors come as JSON with a 4xx status, which
// the http package turns into a status error, so a locked chapter fails there.
func (n *Nuviatoon) get(path string, out any) error {
	body, err := http.Get(http.RequestParams{
		URL:     n.BaseUrl() + path,
		Referer: n.URL,
		Accept:  "application/json",
	})
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(out)
}

// nuviatoonComic is the /api/comics/{slug} payload
type nuviatoonComic struct {
	Title    string `json:"title"`
	Chapters []struct {
		ID         string  `json:"id"`
		Number     float64 `json:"number"`
		Title      *string `json:"title"`
		PriceCoins int     `json:"priceCoins"`
	} `json:"chapters"`
}
