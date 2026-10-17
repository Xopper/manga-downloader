// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// bakkinDomains are the hosts of Bakkin Reader X (Tachiyomi's bakkin
// multisrc), each serving it under /reader/
var bakkinDomains = []string{"bakkin.moe"}

// Bakkin is a grabber for Bakkin Reader X, the JS reader of the bakkin.moe
// Yuru Yuri scanlations. The page is a shell: main.php returns every series
// with its volumes, chapters and page paths in one JSON (?fullsize gives the
// original files instead of the recompressed caches/), and the reader picks
// a series from the URL fragment (#m=1_YRYR&v=v01&c=c001).
type Bakkin struct {
	*Grabber
	series *bakkinSeries
}

// BakkinChapter is a chapter of a series, with its page paths
type BakkinChapter struct {
	Chapter
	Paths []string
}

// bakkinSeries is a series of main.php
type bakkinSeries struct {
	Dir     string `json:"dir"`
	Name    string `json:"name"`
	Volumes []struct {
		Dir      string `json:"dir"`
		Name     string `json:"name"`
		Chapters []struct {
			Dir   string   `json:"dir"`
			Name  string   `json:"name"`
			Pages []string `json:"pages"`
		} `json:"chapters"`
	} `json:"volumes"`
}

// NewBakkin returns a new Bakkin grabber
func NewBakkin(g *Grabber) *Bakkin {
	return &Bakkin{Grabber: g}
}

// Test matches the reader's hosts
func (b *Bakkin) Test() (bool, error) {
	for _, d := range bakkinDomains {
		if hostMatchesDomain(b.URL, d) {
			return true, nil
		}
	}
	return false, nil
}

// readerBase is the reader's root, where main.php and the pages live
func (b *Bakkin) readerBase() string {
	return b.BaseUrl() + "/reader/"
}

// bakkinSeriesDir reads the series folder from the URL fragment, the part a
// chapter link shares with its series link
func bakkinSeriesDir(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	q, err := url.ParseQuery(u.Fragment)
	if err != nil {
		return ""
	}
	return q.Get("m")
}

// load finds the URL's series in main.php
func (b *Bakkin) load() (*bakkinSeries, error) {
	if b.series != nil {
		return b.series, nil
	}
	dir := bakkinSeriesDir(b.URL)
	if dir == "" {
		return nil, errSeriesNotFound(b.URL)
	}
	body, err := http.Get(http.RequestParams{URL: b.readerBase() + "main.php?fullsize", Referer: b.readerBase(), Accept: "application/json"})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	s, err := bakkinFind(body, dir)
	if err != nil {
		return nil, err
	}
	b.series = s
	return s, nil
}

// bakkinFind decodes main.php and returns the series in folder dir
func bakkinFind(r io.Reader, dir string) (*bakkinSeries, error) {
	var all map[string]*bakkinSeries
	if err := json.NewDecoder(r).Decode(&all); err != nil {
		return nil, err
	}
	for _, s := range all {
		if s.Dir == dir {
			return s, nil
		}
	}
	return nil, fmt.Errorf("series %q not found in the reader", dir)
}

// FetchTitle returns the series name
func (b *Bakkin) FetchTitle() (string, error) {
	s, err := b.load()
	if err != nil {
		return "", err
	}
	if s.Name == "" {
		return s.Dir, nil
	}
	return sanitizeTitle(s.Name), nil
}

// FetchChapters returns the series' chapters, volume by volume
func (b *Bakkin) FetchChapters() (Filterables, []error) {
	s, err := b.load()
	if err != nil {
		return nil, []error{err}
	}
	return bakkinChapters(s), nil
}

// bakkinChapters lists a series' chapters. A chapter folder is numbered
// "c012"; extras between them (covers "aCover", interludes "int01",
// afterwords) are numbered just after the chapter they follow, keeping
// their own title. The one-shot collection restarts at "c000" in every
// folder, so when the "c" numbers repeat the chapters are numbered by
// position instead. A "v02" folder gives the volume.
func bakkinChapters(s *bakkinSeries) Filterables {
	var chapters Filterables
	seen := map[float64]bool{}
	byPosition := false
	prev, extras := 0.0, 0
	for _, v := range s.Volumes {
		vol := 0
		if n, err := strconv.Atoi(strings.TrimPrefix(v.Dir, "v")); err == nil {
			vol = n
		}
		for _, c := range v.Chapters {
			num, err := strconv.ParseFloat(strings.TrimPrefix(c.Dir, "c"), 64)
			if err == nil && strings.HasPrefix(c.Dir, "c") {
				byPosition = byPosition || seen[num]
				seen[num] = true
				prev, extras = num, 0
			} else {
				extras++
				num = prev + float64(extras)/100
			}
			title := sanitizeTitle(c.Name)
			if vname := sanitizeTitle(v.Name); vname != "" && title != "" {
				title = vname + " - " + title
			}
			chapters = append(chapters, &BakkinChapter{
				Chapter{
					Number:   num,
					Title:    title,
					Language: "en",
					Volume:   vol,
					ID:       s.Dir + "/" + v.Dir + "/" + c.Dir,
					Label:    Label{Style: LabelNumbered},
				},
				append([]string(nil), c.Pages...),
			})
		}
	}
	if byPosition {
		for i, c := range chapters {
			c.(*BakkinChapter).Number = float64(i + 1)
		}
	}
	return chapters
}

// FetchChapter builds the page URLs from the listing, with no request
func (b *Bakkin) FetchChapter(f Filterable) (*Chapter, error) {
	bchap := f.(*BakkinChapter)
	pages := make([]Page, 0, len(bchap.Paths))
	for _, p := range bchap.Paths {
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: b.readerBase() + bakkinEscapePath(p)})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		Volume:     bchap.Volume,
		ID:         bchap.ID,
		Pages:      pages,
	}, nil
}

// bakkinEscapePath escapes each segment of a page path, as some file names
// hold spaces or brackets
func bakkinEscapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
