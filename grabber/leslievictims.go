// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// LeslieVictims is a grabber for Leslie&Victims (leslie-victims.pages.dev):
// one manga.json lists every series (?series={id}) with its chapter ids and,
// for most chapters, where their pictures are and how many. The others sit
// at content/{series}/{chapter}/01.webp, 02.webp… with no index, so they're
// probed one by one until the site answers with something that isn't a
// picture (it serves its HTML page for a missing file).
type LeslieVictims struct {
	*Grabber
	series *leslieSeries
}

// LeslieVictimsChapter is a chapter of the series
type LeslieVictimsChapter struct {
	Chapter
	Key string
}

// leslieSeries is a series of manga.json
type leslieSeries struct {
	ID       string                `json:"id"`
	Title    string                `json:"title"`
	Chapters []string              `json:"chapters"`
	Roots    map[string]leslieRoot `json:"chapter_roots"`
}

// leslieRoot is where a chapter's pictures are: "count" means
// {url}/01.webp…{url}/NN.webp, "list" the file names in data
type leslieRoot struct {
	URL  string          `json:"url"`
	Mode string          `json:"mode"`
	Data json.RawMessage `json:"data"`
}

var (
	// leslieNumberRe reads the number a chapter id starts with
	leslieNumberRe = regexp.MustCompile(`^\d+(?:\.\d+)?`)
)

// leslieMaxProbe bounds the pages probed for a chapter with no index
const leslieMaxProbe = 200

func NewLeslieVictims(g *Grabber) *LeslieVictims {
	return &LeslieVictims{Grabber: g}
}

// Test returns true for a leslie-victims.pages.dev URL naming a series
func (l *LeslieVictims) Test() (bool, error) {
	return hostMatchesDomain(l.URL, "leslie-victims.pages.dev") && l.seriesID() != "", nil
}

// seriesID returns the URL's ?series= value
func (l LeslieVictims) seriesID() string {
	u, err := url.Parse(l.URL)
	if err != nil {
		return ""
	}
	return u.Query().Get("series")
}

// fetchSeries reads the series off manga.json once
func (l *LeslieVictims) fetchSeries() (*leslieSeries, error) {
	if l.series != nil {
		return l.series, nil
	}
	body, err := http.GetText(http.RequestParams{URL: l.BaseUrl() + "/manga.json", Referer: l.URL, Accept: "application/json"})
	if err != nil {
		return nil, err
	}
	var all []leslieSeries
	if err := json.Unmarshal([]byte(body), &all); err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == l.seriesID() {
			l.series = &all[i]
			return l.series, nil
		}
	}
	return nil, fmt.Errorf("no series %q in manga.json", l.seriesID())
}

// FetchTitle returns the series title
func (l *LeslieVictims) FetchTitle() (string, error) {
	series, err := l.fetchSeries()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(series.Title), nil
}

// FetchChapters returns the chapters, numbered by the number their id
// starts with ("91.5 Special Smut Episode 3" is 91.5)
func (l *LeslieVictims) FetchChapters() (Filterables, []error) {
	series, err := l.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}
	chapters := Filterables{}
	for _, key := range series.Chapters {
		match := leslieNumberRe.FindString(key)
		if match == "" {
			continue
		}
		number, _ := strconv.ParseFloat(match, 64)
		title := strings.TrimSpace(strings.TrimPrefix(key, match))
		title = strings.TrimSpace(strings.TrimPrefix(title, "-"))
		chapters = append(chapters, &LeslieVictimsChapter{
			Chapter: Chapter{Number: number, Title: title, ID: key, Label: Label{Style: LabelPrefixed, Number: match}},
			Key:     key,
		})
	}
	return chapters, nil
}

// FetchChapter lists the chapter's pictures from its root, or probes them
func (l *LeslieVictims) FetchChapter(f Filterable) (*Chapter, error) {
	lchap := f.(*LeslieVictimsChapter)
	series, err := l.fetchSeries()
	if err != nil {
		return nil, err
	}
	var urls []string
	if root, ok := series.Roots[lchap.Key]; ok {
		urls, err = root.pages()
		if err != nil {
			return nil, err
		}
	} else {
		base := l.BaseUrl() + "/content/" + url.PathEscape(series.ID) + "/" + url.PathEscape(lchap.Key) + "/"
		for n := 1; n <= leslieMaxProbe; n++ {
			u := fmt.Sprintf("%s%02d.webp", base, n)
			if !isPicture(u, l.URL) {
				break
			}
			urls = append(urls, u)
		}
	}

	chapter := &Chapter{Title: f.GetTitle(), Label: labelOf(f), Number: f.GetNumber(), Language: "en", ID: lchap.ID, PagesCount: int64(len(urls))}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	return chapter, nil
}

// pages returns the root's picture URLs, in the order the site gives
func (r leslieRoot) pages() ([]string, error) {
	base := strings.TrimSuffix(r.URL, "/") + "/"
	// the url holds the series name with its spaces
	if u, err := url.Parse(base); err == nil {
		base = u.String()
	}
	var urls []string
	switch r.Mode {
	case "count":
		var count int
		if err := json.Unmarshal(r.Data, &count); err != nil {
			return nil, err
		}
		for n := 1; n <= count; n++ {
			urls = append(urls, fmt.Sprintf("%s%02d.webp", base, n))
		}
	case "list":
		var names []string
		if err := json.Unmarshal(r.Data, &names); err != nil {
			return nil, err
		}
		for _, name := range names {
			urls = append(urls, base+url.PathEscape(name))
		}
	default:
		return nil, fmt.Errorf("unknown chapter root mode %q", r.Mode)
	}
	return urls, nil
}

// isPicture fetches u and reports whether it's a picture (WebP, JPEG or PNG)
func isPicture(u, referer string) bool {
	body, err := http.Get(http.RequestParams{URL: u, Referer: referer, Accept: http.ImageAccept, DisableRetry: true})
	if err != nil {
		return false
	}
	defer body.Close()
	head := make([]byte, 12)
	n, _ := io.ReadFull(body, head)
	head = head[:n]
	return (bytes.HasPrefix(head, []byte("RIFF")) && bytes.HasSuffix(head, []byte("WEBP"))) ||
		bytes.HasPrefix(head, []byte{0xff, 0xd8, 0xff}) || bytes.HasPrefix(head, []byte("\x89PNG"))
}
