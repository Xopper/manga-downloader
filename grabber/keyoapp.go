// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// keyoappDomains are the sites on the second version of the Keyoapp theme
// (Tachiyomi's KeyoappV2). The first version (grimscans, writerscans) is a
// PlainHTML selector: it lists every chapter in the series page's #chapters,
// which this one no longer does.
var keyoappDomains = []string{"sirenscans.org"}

// keyoappNumberPrefixRe is the "Chapter 12" a row label starts with; what
// follows, if anything, is the chapter's own name
var keyoappNumberPrefixRe = regexp.MustCompile(`(?i)^\s*chapter\s*\d+(?:\.\d+)?\s*[:\-–]?\s*`)

// Keyoapp is a grabber for the Keyoapp v2 theme. Series and chapters both
// live at the site root (/{series} and /{series}-chapter-12), so a pasted
// link is told apart by the page's data-series-slug. The series page renders
// no chapters: they come from /?_chapters_html=1&series_slug={slug}, a JSON
// whose rows_html holds every row, each with its label, uid and a
// data-ch-locked flag for the coin chapters. The reader is a plain
// #strip-reader of <img>s.
type Keyoapp struct {
	*Grabber
	doc      *goquery.Document
	slug     string
	resolved bool
}

// KeyoappChapter is a listed Keyoapp v2 chapter
type KeyoappChapter struct {
	Chapter
	// URL is the chapter's reader URL
	URL string
}

// NewKeyoapp returns a Keyoapp v2 grabber
func NewKeyoapp(g *Grabber) *Keyoapp {
	return &Keyoapp{Grabber: g}
}

// Test matches the Keyoapp v2 sites by domain
func (k *Keyoapp) Test() (bool, error) {
	return slices.ContainsFunc(keyoappDomains, func(d string) bool {
		return hostMatchesDomain(k.URL, d)
	}), nil
}

// fetch returns the document at uri
func (k *Keyoapp) fetch(uri string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: k.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// resolveSeriesURL reads the series slug off the pasted page (series and
// reader pages both carry data-series-slug) and loads the series page, once
func (k *Keyoapp) resolveSeriesURL() error {
	if k.resolved {
		return nil
	}
	doc, err := k.fetch(k.URL)
	if err != nil {
		return err
	}
	slug := strings.TrimSpace(doc.Find("[data-series-slug]").First().AttrOr("data-series-slug", ""))
	if slug == "" {
		return errSeriesNotFound(k.URL)
	}
	series := k.BaseUrl() + "/" + slug
	if !keyoappSamePath(k.URL, series) {
		if doc, err = k.fetch(series); err != nil {
			return err
		}
	}
	k.URL, k.slug, k.doc, k.resolved = series, slug, doc, true

	return nil
}

// keyoappSamePath reports whether two URLs point at the same path, ignoring a
// trailing slash, the query and the fragment
func keyoappSamePath(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}

	return strings.TrimSuffix(ua.Path, "/") == strings.TrimSuffix(ub.Path, "/")
}

// FetchTitle fetches and returns the series title
func (k *Keyoapp) FetchTitle() (string, error) {
	if err := k.resolveSeriesURL(); err != nil {
		return "", err
	}
	title := sanitizeTitle(k.doc.Find("h1").First().Text())
	if title == "" {
		return "", fmt.Errorf("keyoapp: no title found in %s", k.URL)
	}

	return title, nil
}

// FetchChapters returns the chapters of the series
func (k *Keyoapp) FetchChapters() (Filterables, []error) {
	if err := k.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}
	body, err := http.GetText(http.RequestParams{
		URL:     k.BaseUrl() + "/?_chapters_html=1&series_slug=" + url.QueryEscape(k.slug),
		Referer: k.URL,
		Accept:  "application/json",
	})
	if err != nil {
		return nil, []error{err}
	}
	res := keyoappChapterList{}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return nil, []error{fmt.Errorf("keyoapp: chapter list: %w", err)}
	}

	return keyoappChapters(res.RowsHTML, k.URL)
}

// keyoappChapters reads the chapter rows of a rows_html fragment
func keyoappChapters(rowsHTML, seriesURL string) (chapters Filterables, errs []error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rowsHTML))
	if err != nil {
		return nil, []error{err}
	}
	base, _ := url.Parse(seriesURL)
	doc.Find("a.chapter-row[href]").Each(func(_ int, s *goquery.Selection) {
		label := sanitizeTitle(s.AttrOr("data-ch-label", ""))
		number, ok := parseChapterNumber(label)
		if !ok {
			errs = append(errs, fmt.Errorf("keyoapp: no chapter number in %q", label))
			return
		}
		chapters = append(chapters, &KeyoappChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    keyoappNumberPrefixRe.ReplaceAllString(label, ""),
				Label:    Label{Style: LabelNumbered},
				Language: "en",
				ID:       s.AttrOr("data-ch-uid", ""),
				Locked:   s.AttrOr("data-ch-locked", "") == "1",
			},
			URL: resolveURLRelative(s.AttrOr("href", ""), base),
		})
	})

	return
}

// FetchChapter fetches a chapter and its pages
func (k *Keyoapp) FetchChapter(f Filterable) (*Chapter, error) {
	kchap := f.(*KeyoappChapter)
	doc, err := k.fetch(kchap.URL)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: kchap.Language,
		ID:       kchap.ID,
	}
	for i, img := range keyoappPages(doc, kchap.URL) {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// keyoappPages returns the image URLs of a reader page
func keyoappPages(doc *goquery.Document, chapterURL string) (imgs []string) {
	base, _ := url.Parse(chapterURL)
	doc.Find("#strip-reader img.reader-page").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" || strings.HasPrefix(src, "data:") {
			src = strings.TrimSpace(s.AttrOr("data-src", ""))
		}
		if img := resolveURLRelative(src, base); img != "" {
			imgs = append(imgs, img)
		}
	})

	return
}

// keyoappChapterList is the /?_chapters_html=1 answer
type keyoappChapterList struct {
	RowsHTML string `json:"rows_html"`
}
