// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Toomics is a grabber for Toomics (global.toomics.com): a title's episode
// page lists every episode, the reader link in each row's onclick, and marks
// the paid ones (a "VIP ONLY" or coin badge); only free episodes open
// logged out. The image CDN wants the site's Referer, as every page gets.
type Toomics struct {
	*Grabber
	doc *goquery.Document
}

// ToomicsChapter is an episode
type ToomicsChapter struct {
	Chapter
	URL string
}

var (
	// toomicsTitleRe captures the language and title id of a title or reader
	// path (/en/webtoon/episode/toon/6723, /en/webtoon/detail/code/1/ep/1/toon/6723)
	toomicsTitleRe = regexp.MustCompile(`^/([a-z]{2}(?:-[a-z]{2})?)/webtoon/.*?/toon/(\d+)`)
	// toomicsReaderRe finds a row's reader path in its onclick
	toomicsReaderRe = regexp.MustCompile(`'(/[a-z-]+/webtoon/detail/[^']+)'`)
)

func NewToomics(g *Grabber) *Toomics {
	return &Toomics{Grabber: g}
}

// Test returns true for a toomics.com title or reader URL
func (t *Toomics) Test() (bool, error) {
	lang, _ := t.title()
	return hostMatchesDomain(t.URL, "toomics.com") && lang != "", nil
}

// title returns the URL's language and title id
func (t Toomics) title() (lang, id string) {
	u, err := url.Parse(t.URL)
	if err != nil {
		return "", ""
	}
	matches := toomicsTitleRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return "", ""
	}
	return matches[1], matches[2]
}

// episodes fetches the title's episode page once
func (t *Toomics) episodes() (*goquery.Document, error) {
	if t.doc == nil {
		lang, id := t.title()
		body, err := http.Get(http.RequestParams{URL: t.BaseUrl() + "/" + lang + "/webtoon/episode/toon/" + id})
		if err != nil {
			return nil, err
		}
		defer body.Close()
		if t.doc, err = goquery.NewDocumentFromReader(body); err != nil {
			return nil, err
		}
	}
	return t.doc, nil
}

// FetchTitle returns the title's name
func (t *Toomics) FetchTitle() (string, error) {
	doc, err := t.episodes()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find("h2.line-clamp-2").First().Text()), nil
}

// FetchChapters returns the episodes; a row whose coin badge isn't "free"
// (coin-type1) or "already read" (coin-type6) is locked
func (t *Toomics) FetchChapters() (Filterables, []error) {
	doc, err := t.episodes()
	if err != nil {
		return nil, []error{err}
	}
	lang, _ := t.title()
	chapters := Filterables{}
	doc.Find("li.normal_ep").Each(func(_ int, li *goquery.Selection) {
		a := li.Find("a[onclick]").First()
		path := toomicsReaderRe.FindStringSubmatch(a.AttrOr("onclick", ""))
		if path == nil {
			return
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(li.Find(".cell-num .num").First().Text()), 64)
		if err != nil {
			return
		}
		locked := false
		li.Find(`.cell-coin span[class^="coin-type"]`).Each(func(_ int, s *goquery.Selection) {
			class := s.AttrOr("class", "")
			locked = locked || (class != "coin-type1" && class != "coin-type6")
		})
		chapters = append(chapters, &ToomicsChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    strings.TrimSpace(li.Find(".cell-title strong").First().Text()),
				ID:       path[1],
				Language: lang,
				Locked:   locked,
				Label:    Label{Style: LabelPrefixed},
			},
			URL: t.BaseUrl() + path[1],
		})
	})
	return chapters, nil
}

// FetchChapter reads the reader's lazy-loaded pictures
func (t *Toomics) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*ToomicsChapter)
	body, err := http.Get(http.RequestParams{URL: tchap.URL, Referer: t.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: tchap.Language,
		ID:       tchap.ID,
	}
	doc.Find(`div[id^="load_image_"] img[data-src]`).Each(func(_ int, img *goquery.Selection) {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: img.AttrOr("data-src", "")})
	})
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}
