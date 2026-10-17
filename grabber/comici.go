// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/elboletaire/manga-downloader/http"
)

// comiciDomains are the sites on the comici viewer's current (Next.js)
// front end, whose JSON API sits under the site's own /api/
var comiciDomains = []string{"comic-growl.com"}

// comiciSeriesRe takes the series hash out of a series URL
// (/series/{hash}, /series/{hash}/new or /{magazine}/series/{hash})
var comiciSeriesRe = regexp.MustCompile(`/series/([0-9a-f]+)`)

// comiciEpisodeRe takes the episode hash out of a reader URL (/episodes/{hash})
var comiciEpisodeRe = regexp.MustCompile(`/episodes/([0-9a-f]+)`)

// comiciNumberRe reads the episode titles the sites number by hand: "12",
// "2-①", "24‐②" (a split episode's part after a dash) or "3.5", with the
// rest, if any, the episode's own title
var comiciNumberRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(?:[-‐－–ー]\s*(\d+|[①-⑳]))?\s*(.*)$`)

// comiciGrid is the side, in tiles, of a page's scramble grid
const comiciGrid = 4

// Comici is a grabber for the comici viewer (comici.jp's white-label manga
// sites, comic-growl.com among them). Everything is JSON under /api/: the
// series and its episodes (/episodes?seriesHash=), which episodes a logged-out
// reader may open (/series/access: most are "waitAndFree" or paid, only a
// few are free), an episode's viewer id (/episodes/{hash}) and its pages
// (/book/contentsInfo). Pages are signed CloudFront URLs that need the site's
// Referer, and each is cut into a 4x4 grid of shuffled tiles, undone in
// Transform from the order the page list gives.
type Comici struct {
	*Grabber
	series *comiciSeries
}

// ComiciChapter is a listed comici episode
type ComiciChapter struct {
	Chapter
}

// NewComici returns a comici viewer grabber
func NewComici(g *Grabber) *Comici {
	return &Comici{Grabber: g}
}

// Test matches the comici sites by domain
func (c *Comici) Test() (bool, error) {
	return slices.ContainsFunc(comiciDomains, func(d string) bool {
		return hostMatchesDomain(c.URL, d)
	}), nil
}

// api fetches a comici API path into v
func (c *Comici) api(path string, v any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     c.BaseUrl() + "/api" + path,
		Referer: c.URL,
		Accept:  "application/json",
	})
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(body), v); err != nil {
		return fmt.Errorf("comici: %s: %w", path, err)
	}

	return nil
}

// seriesHash returns the series of the URL. A reader URL only names its
// episode, whose details name the series.
func (c *Comici) seriesHash() (string, error) {
	if m := comiciSeriesRe.FindStringSubmatch(c.URL); m != nil {
		return m[1], nil
	}
	m := comiciEpisodeRe.FindStringSubmatch(c.URL)
	if m == nil {
		return "", errSeriesNotFound(c.URL)
	}
	res := comiciEpisodeDetails{}
	if err := c.api("/episodes/"+m[1], &res); err != nil {
		return "", err
	}
	if res.Episode.Series.ID == "" {
		return "", errSeriesNotFound(c.URL)
	}

	return res.Episode.Series.ID, nil
}

// fetchSeries fetches the series and its episodes once
func (c *Comici) fetchSeries() (*comiciSeries, error) {
	if c.series != nil {
		return c.series, nil
	}
	hash, err := c.seriesHash()
	if err != nil {
		return nil, err
	}
	c.URL = c.BaseUrl() + "/series/" + hash
	query := "?seriesHash=" + url.QueryEscape(hash) + "&episodeFrom=1&episodeTo=9999"

	res := comiciSeriesResponse{}
	if err := c.api("/episodes"+query, &res); err != nil {
		return nil, err
	}
	access := comiciAccessResponse{}
	if err := c.api("/series/access"+query, &access); err != nil {
		return nil, err
	}
	res.Series.locked = map[string]bool{}
	for _, a := range access.SeriesAccess.EpisodeAccesses {
		res.Series.locked[a.EpisodeID] = !a.HasAccess
	}
	c.series = &res.Series

	return c.series, nil
}

// FetchTitle fetches and returns the series title
func (c *Comici) FetchTitle() (string, error) {
	series, err := c.fetchSeries()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(series.Summary.Name), nil
}

// FetchChapters returns the series' episodes, the ones a logged-out reader
// can't open marked locked
func (c *Comici) FetchChapters() (chapters Filterables, errs []error) {
	series, err := c.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}
	language := comiciLanguage(series.Summary.Name)
	for _, e := range series.Episodes {
		title, number, part := comiciTitle(e.Title, series.Summary.Name)
		chapters = append(chapters, &ComiciChapter{Chapter{
			Number:   number,
			Title:    title,
			Label:    Label{Style: LabelNumbered, Part: part},
			Language: language,
			ID:       e.ID,
			Locked:   series.locked[e.ID],
		}})
	}

	return
}

// comiciLanguage returns a series' language: Japanese, but for the English
// editions the sites list among the rest, named "[ENG]…"
func comiciLanguage(name string) string {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(name)), "[ENG]") {
		return "en"
	}

	return "ja"
}

// comiciTitle splits an episode title into its own title, number and part:
// the sites write "2-①", which gigaviewerTitle doesn't read, or "第3話 …",
// which it does
func comiciTitle(raw, series string) (title string, number float64, part string) {
	raw = cleanTitle(raw)
	m := comiciNumberRe.FindStringSubmatch(gigaviewerDigits.Replace(raw))
	if m == nil {
		return gigaviewerTitle(raw, series)
	}
	number, _ = strconv.ParseFloat(m[1], 64)
	part = m[2]
	if r, size := utf8.DecodeRuneInString(part); size > 1 && size == len(part) {
		part, _ = gigaviewerCircled(r)
	}
	title = strings.TrimSpace(m[3])
	if title == series {
		title = ""
	}

	return title, number, part
}

// FetchChapter fetches an episode's pages
func (c *Comici) FetchChapter(f Filterable) (*Chapter, error) {
	cchap := f.(*ComiciChapter)
	details := comiciEpisodeDetails{}
	if err := c.api("/episodes/"+url.PathEscape(cchap.ID), &details); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: cchap.Language,
		ID:       cchap.ID,
	}
	viewer := ""
	for _, content := range details.Episode.Content {
		switch content.Type {
		case "viewer":
			viewer = content.ViewerID
		case "image":
			// a few episodes are a plain image list, with no viewer
			chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: content.URL})
		}
	}
	if viewer != "" {
		pages, err := c.viewerPages(viewer, details.Episode.ContentID)
		if err != nil {
			return nil, err
		}
		chapter.Pages = pages
	}
	if len(chapter.Pages) == 0 {
		return nil, fmt.Errorf("comici: %s has no pages a logged-out reader can open", ChapterLabel(f))
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// viewerPages lists a viewer's pages: one request learns the page count (the
// list is ranged), a second lists them all
func (c *Comici) viewerPages(viewer string, contentID int) ([]Page, error) {
	query := fmt.Sprintf("/book/contentsInfo?comici-viewer-id=%s&contentId=%d&page-from=0&page-to=", url.QueryEscape(viewer), contentID)
	first := comiciContents{}
	if err := c.api(query+"0", &first); err != nil {
		return nil, err
	}
	all := comiciContents{}
	if err := c.api(query+strconv.Itoa(first.TotalPages), &all); err != nil {
		return nil, err
	}
	sort.SliceStable(all.Result, func(i, j int) bool { return all.Result[i].Sort < all.Result[j].Sort })

	pages := make([]Page, 0, len(all.Result))
	for i, p := range all.Result {
		page := Page{Number: int64(i + 1), URL: p.ImageURL}
		tiles, err := comiciTiles(p.Scramble)
		if err != nil {
			return nil, err
		}
		if tiles != nil {
			page.Transform = func(data []byte) ([]byte, error) { return comiciUnscramble(data, tiles) }
		}
		pages = append(pages, page)
	}

	return pages, nil
}

// comiciTiles reads a page's scramble ("[13, 6, 15, …]"), nil for a page that
// isn't scrambled
func comiciTiles(scramble string) ([]int, error) {
	scramble = strings.TrimSpace(scramble)
	if scramble == "" || scramble == "[]" {
		return nil, nil
	}
	var tiles []int
	if err := json.Unmarshal([]byte(scramble), &tiles); err != nil {
		return nil, fmt.Errorf("comici: scramble %q: %w", scramble, err)
	}
	if len(tiles) != comiciGrid*comiciGrid {
		return nil, fmt.Errorf("comici: scramble %q isn't a %dx%d grid", scramble, comiciGrid, comiciGrid)
	}

	return tiles, nil
}

// comiciUnscramble puts a page's tiles back. Tiles are numbered down the
// columns (tile i is at column i/4, row i%4), the tile at i comes from
// tiles[i], and the strips past the grid (the size isn't a multiple of 4)
// stay where they are.
func comiciUnscramble(data []byte, tiles []int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("comici: decoding scrambled image: %w", err)
	}

	bounds := src.Bounds()
	tileW, tileH := bounds.Dx()/comiciGrid, bounds.Dy()/comiciGrid
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
	for to, from := range tiles {
		toPt := image.Pt(bounds.Min.X+to/comiciGrid*tileW, bounds.Min.Y+to%comiciGrid*tileH)
		fromPt := image.Pt(bounds.Min.X+from/comiciGrid*tileW, bounds.Min.Y+from%comiciGrid*tileH)
		draw.Draw(dst, image.Rectangle{Min: toPt, Max: toPt.Add(image.Pt(tileW, tileH))}, src, fromPt, draw.Src)
	}

	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, dst, &jpeg.Options{Quality: 95}); err != nil {
		return nil, fmt.Errorf("comici: encoding descrambled image: %w", err)
	}

	return buf.Bytes(), nil
}

// comiciSeriesResponse is /api/episodes?seriesHash=
type comiciSeriesResponse struct {
	Series comiciSeries `json:"series"`
}

// comiciSeries is a series and its episodes, oldest first
type comiciSeries struct {
	Summary struct {
		Name string `json:"name"`
	} `json:"summary"`
	Episodes []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"episodes"`
	// locked are the episodes /series/access denies a logged-out reader
	locked map[string]bool
}

// comiciAccessResponse is /api/series/access?seriesHash=
type comiciAccessResponse struct {
	SeriesAccess struct {
		EpisodeAccesses []struct {
			EpisodeID string `json:"episodeId"`
			HasAccess bool   `json:"hasAccess"`
		} `json:"episodeAccesses"`
	} `json:"seriesAccess"`
}

// comiciEpisodeDetails is /api/episodes/{hash}
type comiciEpisodeDetails struct {
	Episode struct {
		Series struct {
			ID string `json:"id"`
		} `json:"series"`
		ContentID int `json:"contentId"`
		Content   []struct {
			Type     string `json:"type"`
			ViewerID string `json:"viewerId"`
			URL      string `json:"url"`
		} `json:"content"`
	} `json:"episode"`
}

// comiciContents is /api/book/contentsInfo
type comiciContents struct {
	TotalPages int `json:"totalPages"`
	Result     []struct {
		ImageURL string `json:"imageUrl"`
		Scramble string `json:"scramble"`
		Sort     int    `json:"sort"`
	} `json:"result"`
}
