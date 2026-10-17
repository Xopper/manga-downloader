// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

const lolobunDomain = "lolobun.com"

// lolobunPathRe captures a comic id and, for a reader URL, its chapter id
var lolobunPathRe = regexp.MustCompile(`^/c/(\d+)(?:/(\d+))?/?$`)

// lolobunChapterNumRe reads a row's number: "chapter 1", "chapter173"
var lolobunChapterNumRe = regexp.MustCompile(`(?i)^chapter\s*(\d+(?:\.\d+)?)$`)

// Lolobun is a grabber for lolobun.com, the English storefront of SFACG's
// comics. The comic page (/c/{id}) lists every chapter, paid ones marked by
// a lock icon; a chapter's pages come from a form POST to
// ajax/comic.ashx?op=getChapterPic, which answers an empty list for a
// locked chapter.
type Lolobun struct {
	*Grabber
	doc *goquery.Document
}

// LolobunChapter is a row of the comic's catalog
type LolobunChapter struct {
	Chapter
	ChapID string
}

// NewLolobun returns a new Lolobun grabber
func NewLolobun(g *Grabber) *Lolobun {
	return &Lolobun{Grabber: g}
}

// Test matches the site's domain
func (l *Lolobun) Test() (bool, error) {
	return hostMatchesDomain(l.URL, lolobunDomain), nil
}

// seriesURL cuts a reader URL (/c/{comic}/{chapter}) back to its comic
func (l *Lolobun) seriesURL() (string, error) {
	_, path, ok := chapterPath(l.URL)
	if !ok {
		return "", errSeriesNotFound(l.URL)
	}
	m := lolobunPathRe.FindStringSubmatch(path)
	if m == nil {
		return "", errSeriesNotFound(l.URL)
	}
	return "https://www." + lolobunDomain + "/c/" + m[1], nil
}

// series fetches the comic page once
func (l *Lolobun) series() (*goquery.Document, error) {
	if l.doc != nil {
		return l.doc, nil
	}
	uri, err := l.seriesURL()
	if err != nil {
		return nil, err
	}
	body, err := http.Get(http.RequestParams{URL: uri})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	l.doc = doc
	return doc, nil
}

// FetchTitle returns the comic's name
func (l *Lolobun) FetchTitle() (string, error) {
	doc, err := l.series()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find(".header-item-info .name").First().Text()), nil
}

// FetchChapters returns the catalog rows
func (l *Lolobun) FetchChapters() (Filterables, []error) {
	doc, err := l.series()
	if err != nil {
		return nil, []error{err}
	}
	chapters := lolobunChapters(doc)
	if len(chapters) == 0 {
		return nil, []error{fmt.Errorf("no chapters found in %s", l.URL)}
	}
	return chapters, nil
}

// lolobunChapters parses the catalog. Unnumbered rows ("Extra 01") sit
// between numbered chapters, so they're numbered just after the chapter they
// follow (as Tachiyomi does), keeping their own title, rather than by the
// digits in their name.
func lolobunChapters(doc *goquery.Document) Filterables {
	var chapters Filterables
	prev, extras := 0.0, 0
	doc.Find(".catalog-list .catalog-item").Each(func(_ int, row *goquery.Selection) {
		a := row.Find(".title a").First()
		href := a.AttrOr("href", "")
		u, err := url.Parse(href)
		if err != nil {
			return
		}
		m := lolobunPathRe.FindStringSubmatch(u.Path)
		if m == nil || m[2] == "" {
			return
		}
		name := sanitizeTitle(a.Text())
		c := &LolobunChapter{
			Chapter: Chapter{
				Language: "en",
				ID:       m[2],
				Locked:   row.Find(`.icon-box img[src*="lock"]`).Length() > 0,
				Label:    Label{Style: LabelNumbered},
			},
			ChapID: m[2],
		}
		if nm := lolobunChapterNumRe.FindStringSubmatch(name); nm != nil {
			c.Number, _ = strconv.ParseFloat(nm[1], 64)
			c.Label.Number = nm[1]
			prev, extras = c.Number, 0
		} else {
			extras++
			c.Number = prev + float64(extras)/100
			c.Title = name
		}
		chapters = append(chapters, c)
	})
	return chapters
}

// FetchChapter asks getChapterPic for the chapter's images
func (l *Lolobun) FetchChapter(f Filterable) (*Chapter, error) {
	lchap := f.(*LolobunChapter)
	series, err := l.seriesURL()
	if err != nil {
		return nil, err
	}
	body, err := http.Post(http.RequestParams{
		URL:     "https://www." + lolobunDomain + "/ajax/comic.ashx?op=getChapterPic",
		Referer: series + "/" + lchap.ChapID,
		Form:    url.Values{"chapId": {lchap.ChapID}},
		Accept:  "application/json",
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	pages, err := lolobunPages(body)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s (a locked chapter has none)", ChapterLabel(f))
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		ID:         lchap.ID,
		Pages:      pages,
	}, nil
}

// lolobunPages reads getChapterPic's answer
func lolobunPages(r io.Reader) ([]Page, error) {
	var res struct {
		Status struct {
			ErrorCode int     `json:"errorCode"`
			Msg       *string `json:"msg"`
		} `json:"status"`
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(r).Decode(&res); err != nil {
		return nil, err
	}
	if res.Status.ErrorCode != 200 {
		msg := ""
		if res.Status.Msg != nil {
			msg = *res.Status.Msg
		}
		return nil, fmt.Errorf("getChapterPic failed (%d): %s", res.Status.ErrorCode, msg)
	}
	pages := make([]Page, 0, len(res.Data))
	for _, u := range res.Data {
		if u = strings.TrimSpace(u); u != "" {
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: u})
		}
	}
	return pages, nil
}
