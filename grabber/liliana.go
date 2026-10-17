// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Liliana is a grabber for the Liliana manga theme (manhuaplus.org): series at
// /manga/{slug} with every chapter in ul#myUL, readers at
// /manga/{slug}/{chapter}. A reader page names its CHAPTER_ID, and its pages
// come from /ajax/image/list/chap/{id} as an HTML fragment whose
// div.separator blocks are shuffled and carry their place in data-index.
type Liliana struct {
	*Grabber
	doc    *goquery.Document
	series string
}

// LilianaChapter is a chapter of a Liliana site
type LilianaChapter struct {
	Chapter
	URL string
}

func NewLiliana(g *Grabber) *Liliana {
	return &Liliana{Grabber: g}
}

var (
	lilianaSeriesRe    = regexp.MustCompile(`^(/manga/[^/?#]+)`)
	lilianaChapterIDRe = regexp.MustCompile(`const\s+CHAPTER_ID\s*=\s*(\d+)`)
)

// lilianaRows are the series page's chapter rows
const lilianaRows = "ul#myUL li.chapter a[href]"

// Test fetches /manga/ pages and accepts the theme's series page; a reader
// URL is reduced to its series first
func (l *Liliana) Test() (bool, error) {
	u, err := url.Parse(l.URL)
	if err != nil {
		return false, nil
	}
	m := lilianaSeriesRe.FindStringSubmatch(u.Path)
	if m == nil {
		return false, nil
	}
	l.series = l.BaseUrl() + m[1]

	body, err := http.GetText(http.RequestParams{
		URL:          l.series,
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		return false, err
	}
	if !strings.Contains(body, "/themes/liliana/") {
		return false, nil
	}
	l.doc, err = goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return false, err
	}
	return l.doc.Find(lilianaRows).Length() > 0, nil
}

// FetchTitle returns the series title
func (l *Liliana) FetchTitle() (string, error) {
	title := sanitizeTitle(l.doc.Find("h1").First().Text())
	if title == "" {
		return "", errors.New("could not find the series title")
	}
	return title, nil
}

// FetchChapters reads the series page's chapter list
func (l *Liliana) FetchChapters() (chapters Filterables, errs []error) {
	l.doc.Find(lilianaRows).Each(func(_ int, a *goquery.Selection) {
		href := a.AttrOr("href", "")
		text := sanitizeTitle(a.Text())
		number, ok := parseChapterNumber(text)
		if !ok {
			if m := urlChapterNumberRe.FindStringSubmatch(href); m != nil {
				number, _ = strconv.ParseFloat(m[1], 64)
			}
		}
		if chapterNumberOnlyRe.MatchString(text) {
			text = ""
		}
		u := l.resolve(href)
		chapters = append(chapters, &LilianaChapter{
			Chapter{
				Number: number,
				Title:  text,
				ID:     chapterURLID(u),
				Label:  Label{Style: LabelNumbered},
			},
			u,
		})
	})
	return chapters, nil
}

// chapterNumberOnlyRe matches a row title that is only "Chapter N"
var chapterNumberOnlyRe = regexp.MustCompile(`(?i)^chapter\s*\d+(\.\d+)?$`)

func (l *Liliana) resolve(href string) string {
	base, _ := url.Parse(l.series)
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// FetchChapter reads the reader's CHAPTER_ID and asks the image list for it
func (l *Liliana) FetchChapter(f Filterable) (*Chapter, error) {
	lchap := f.(*LilianaChapter)
	reader, err := http.GetText(http.RequestParams{
		URL:     lchap.URL,
		Referer: l.series,
	})
	if err != nil {
		return nil, err
	}
	id := lilianaChapterIDRe.FindStringSubmatch(reader)
	if id == nil {
		return nil, fmt.Errorf("could not find the chapter id of %s", ChapterLabel(f))
	}
	list, err := http.GetText(http.RequestParams{
		URL:     l.BaseUrl() + "/ajax/image/list/chap/" + id[1],
		Referer: lchap.URL,
		Headers: map[string]string{"X-Requested-With": "XMLHttpRequest"},
	})
	if err != nil {
		return nil, err
	}
	urls, err := lilianaImages(list)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		ID:       lchap.ID,
		Language: "en",
	}
	for _, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    u,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	if chapter.PagesCount == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return chapter, nil
}

// lilianaImages returns the page URLs of an image list answer, put back in
// the order of their data-index
func lilianaImages(body string) ([]string, error) {
	var res struct {
		Status bool   `json:"status"`
		HTML   string `json:"html"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return nil, fmt.Errorf("could not read the image list: %w", err)
	}
	if !res.Status {
		return nil, errors.New("the image list answered with an error")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.HTML))
	if err != nil {
		return nil, err
	}
	type page struct {
		index int
		url   string
	}
	var pages []page
	doc.Find("div.separator").Each(func(i int, s *goquery.Selection) {
		u := strings.TrimSpace(s.Find("a.readImg").AttrOr("href", ""))
		if u == "" || u == "#" {
			u = strings.TrimSpace(s.Find("img").AttrOr("src", ""))
		}
		if u == "" {
			return
		}
		index, err := strconv.Atoi(s.AttrOr("data-index", ""))
		if err != nil {
			index = i
		}
		pages = append(pages, page{index, u})
	})
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].index < pages[j].index })
	urls := make([]string, len(pages))
	for i, p := range pages {
		urls[i] = p.url
	}
	return urls, nil
}
