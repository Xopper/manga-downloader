// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// mangaboltChapterPathRe matches a mangabolt.com reader path,
// /chapter/{series}-chapter-{n}/, capturing the series slug
var mangaboltChapterPathRe = regexp.MustCompile(`^/chapter/(.+?)-chapter-[\d.-]+/?$`)

// Mangabolt is a grabber for mangabolt.com. Its Tailwind pages share no markup
// with a PlainHTML entry: each chapter is a card (div.grid) whose bold link
// names the series and number ("Gorgeous Irene Chapter 5") with an optional
// chapter title below it, and the reader lazy-loads its img.js-page images
// from data-src, with a <noscript> copy of each. The reader has no link back
// to its series, so a pasted chapter is mapped by its slug
// ({series}-chapter-{n}).
type Mangabolt struct {
	*Grabber
	doc *goquery.Document
}

// MangaboltChapter is a listed mangabolt.com chapter
type MangaboltChapter struct {
	Chapter
	// URL is the chapter's reader URL
	URL string
}

// NewMangabolt returns a mangabolt.com grabber
func NewMangabolt(g *Grabber) *Mangabolt {
	return &Mangabolt{Grabber: g}
}

// Test matches mangabolt.com series and chapter URLs by domain
func (m *Mangabolt) Test() (bool, error) {
	if !hostMatchesDomain(m.URL, "mangabolt.com") {
		return false, nil
	}
	u, err := url.Parse(m.URL)
	if err != nil {
		return false, nil
	}

	return strings.HasPrefix(u.Path, "/manga/") || strings.HasPrefix(u.Path, "/chapter/"), nil
}

// resolveSeriesURL turns a chapter URL into its series URL
func (m *Mangabolt) resolveSeriesURL() error {
	u, err := url.Parse(m.URL)
	if err != nil {
		return err
	}
	if strings.HasPrefix(u.Path, "/manga/") {
		return nil
	}
	match := mangaboltChapterPathRe.FindStringSubmatch(u.Path)
	if match == nil {
		return errSeriesNotFound(m.URL)
	}
	m.URL = m.BaseUrl() + "/manga/" + match[1] + "/"

	return nil
}

// series returns the series page, fetched once
func (m *Mangabolt) series() (*goquery.Document, error) {
	if m.doc != nil {
		return m.doc, nil
	}
	if err := m.resolveSeriesURL(); err != nil {
		return nil, err
	}
	doc, err := mangaboltFetch(m.URL, m.BaseUrl())
	if err != nil {
		return nil, err
	}
	m.doc = doc

	return doc, nil
}

// mangaboltFetch returns the document at uri
func mangaboltFetch(uri, referer string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: referer})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// FetchTitle fetches and returns the manga title
func (m *Mangabolt) FetchTitle() (string, error) {
	doc, err := m.series()
	if err != nil {
		return "", err
	}
	title := sanitizeTitle(doc.Find("#main-content h1").First().Text())
	if title == "" {
		return "", fmt.Errorf("mangabolt: no title found in %s", m.URL)
	}

	return title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangabolt) FetchChapters() (Filterables, []error) {
	doc, err := m.series()
	if err != nil {
		return nil, []error{err}
	}

	return mangaboltChapters(doc, m.URL)
}

// mangaboltChapters reads the chapter cards of a series page. The card's
// second link is a "Read" button to the same chapter, so only the first one
// per card counts.
func mangaboltChapters(doc *goquery.Document, seriesURL string) (chapters Filterables, errs []error) {
	base, _ := url.Parse(seriesURL)
	doc.Find("div.grid").Each(func(_ int, card *goquery.Selection) {
		link := card.Find(`a[href*="/chapter/"]`).First()
		if link.Length() == 0 {
			return
		}
		href := resolveURLRelative(link.AttrOr("href", ""), base)
		text := sanitizeTitle(link.Text())
		number, ok := parseChapterNumber(text)
		if !ok {
			if number, _, ok = parseChapterSlugNum(href); !ok {
				errs = append(errs, fmt.Errorf("mangabolt: no chapter number in %q", text))
				return
			}
		}
		title := sanitizeTitle(link.Parent().Find(".text-xs").First().Text())
		if strings.EqualFold(title, "read") {
			title = ""
		}
		chapters = append(chapters, &MangaboltChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    title,
				Label:    Label{Style: LabelNumbered},
				Language: "en",
				ID:       chapterURLID(href),
			},
			URL: href,
		})
	})

	return
}

// FetchChapter fetches a chapter and its pages
func (m *Mangabolt) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaboltChapter)
	doc, err := mangaboltFetch(mchap.URL, m.URL)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: mchap.Language,
		ID:       mchap.ID,
	}
	for i, img := range mangaboltPages(doc, mchap.URL) {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// mangaboltPages returns a reader's image URLs in order: each lazy image's
// data-src (its src is a 1px placeholder), the <noscript> copies left out
// (the HTML parser keeps those as text) and repeats dropped
func mangaboltPages(doc *goquery.Document, chapterURL string) (imgs []string) {
	base, _ := url.Parse(chapterURL)
	seen := map[string]bool{}
	doc.Find("img.js-page").Each(func(_ int, s *goquery.Selection) {
		if s.ParentsFiltered("noscript").Length() > 0 {
			return
		}
		src := strings.TrimSpace(s.AttrOr("data-src", ""))
		if src == "" {
			src = strings.TrimSpace(s.AttrOr("src", ""))
		}
		if strings.HasPrefix(src, "data:") {
			return
		}
		img := resolveURLRelative(src, base)
		if img == "" || seen[img] {
			return
		}
		seen[img] = true
		imgs = append(imgs, img)
	})

	return
}
