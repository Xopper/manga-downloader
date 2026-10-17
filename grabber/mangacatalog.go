// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// mangaCatalogDomains are the franchise sites of the MangaCatalog network
// (Tachiyomi's mangacatalog multisrc) that still serve its Tailwind layout.
// Each hosts a handful of series of one franchise and rotates a wN./wwN.
// subdomain, so the registered domain is matched with any subdomain. Family
// members that moved to another theme (readberserk.com, readopm.com,
// thetowerofgod.com) are left out on purpose.
var mangaCatalogDomains = []string{
	"readonepiece.com",
	"read7deadlysins.com",
	"tokyoghoulre.com",
	"readsnk.com",
	"readchainsawman.com",
	"readfairytail.com",
	"readjujutsukaisen.com",
	"readkingdom.com",
	"readsololeveling.org",
}

// MangaCatalog is a grabber for the MangaCatalog network. A series page
// (/manga/{slug}/) lists every chapter in plain HTML, and a reader
// (/chapter/{slug}-chapter-N/) prints every page, lazy ones in data-src. The
// sites are single-franchise and share no API, so matching is by domain.
type MangaCatalog struct {
	*Grabber
	doc      *goquery.Document
	resolved bool
}

// MangaCatalogChapter is a chapter listed on a MangaCatalog series page
type MangaCatalogChapter struct {
	Chapter
	URL string
}

// NewMangaCatalog returns a new MangaCatalog grabber
func NewMangaCatalog(g *Grabber) *MangaCatalog {
	return &MangaCatalog{Grabber: g}
}

// Test matches the network's domains from the URL alone
func (m *MangaCatalog) Test() (bool, error) {
	for _, d := range mangaCatalogDomains {
		if hostMatchesDomain(m.URL, d) {
			return true, nil
		}
	}
	return false, nil
}

// resolveSeriesURL turns a reader URL into its series page. The chapter slug
// doesn't always share the series slug ("one-piece-digital-colored-comics"
// vs "one-piece-colored-chapter-1"), so the reader is fetched once and its
// chapter dropdown's first option ("View all chapters") gives the series.
func (m *MangaCatalog) resolveSeriesURL() error {
	if m.resolved {
		return nil
	}
	m.resolved = true
	u, err := url.Parse(m.URL)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(u.Path), "/chapter/") {
		return nil
	}
	doc, err := m.get(m.URL)
	if err != nil {
		return err
	}
	series := mangaCatalogSeriesLink(doc)
	if series == "" {
		return errSeriesNotFound(m.URL)
	}
	m.URL = series
	return nil
}

// mangaCatalogSeriesLink returns the series link of a reader page: the
// dropdown option pointing at a /manga/ path
func mangaCatalogSeriesLink(doc *goquery.Document) string {
	var link string
	doc.Find("select option").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		v := strings.TrimSpace(s.AttrOr("value", ""))
		if u, err := url.Parse(v); err == nil && strings.HasPrefix(u.Path, "/manga/") {
			link = v
			return false
		}
		return true
	})
	return link
}

// get fetches and parses a page of the site
func (m *MangaCatalog) get(uri string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: m.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

// series fetches the series page once for the title and the chapters
func (m *MangaCatalog) series() (*goquery.Document, error) {
	if m.doc != nil {
		return m.doc, nil
	}
	if err := m.resolveSeriesURL(); err != nil {
		return nil, err
	}
	doc, err := m.get(m.URL)
	if err != nil {
		return nil, err
	}
	m.doc = doc
	return doc, nil
}

// FetchTitle returns the series title: the page's second h1 (the first one is
// the site banner), else og:title
func (m *MangaCatalog) FetchTitle() (string, error) {
	doc, err := m.series()
	if err != nil {
		return "", err
	}
	return mangaCatalogTitle(doc), nil
}

// mangaCatalogTitle reads the series title, skipping the site banner's h1
func mangaCatalogTitle(doc *goquery.Document) string {
	if t := sanitizeTitle(doc.Find("div.container > h1").Last().Text()); t != "" {
		return t
	}
	return sanitizeTitle(doc.Find(`meta[property="og:title"]`).AttrOr("content", ""))
}

// FetchChapters returns the chapters listed on the series page
func (m *MangaCatalog) FetchChapters() (Filterables, []error) {
	doc, err := m.series()
	if err != nil {
		return nil, []error{err}
	}
	chapters := mangaCatalogChapters(doc)
	if len(chapters) == 0 {
		return nil, []error{fmt.Errorf("no chapters found in %s (is it a series page?)", m.URL)}
	}
	return chapters, nil
}

// mangaCatalogChapters parses the chapter rows. A row's link text repeats the
// series name ("MONSTERS Chapter 0"), so the number comes from the URL slug
// (-chapter-N) and the title from the row's subtitle line, which the site
// fills with the chapter's own name.
func mangaCatalogChapters(doc *goquery.Document) Filterables {
	var chapters Filterables
	doc.Find("div.bg-bg-secondary > div.grid").Each(func(_ int, row *goquery.Selection) {
		a := row.Find(".col-span-4 > a").First()
		href := strings.TrimSpace(a.AttrOr("href", ""))
		if href == "" {
			return
		}
		num, numStr, ok := parseChapterSlugNum(href)
		if !ok {
			return
		}
		title := sanitizeTitle(row.Find(".col-span-4 > div.text-xs").First().Text())
		chapters = append(chapters, &MangaCatalogChapter{
			Chapter{
				Number:   num,
				Title:    title,
				Language: "en",
				ID:       chapterURLID(href),
				Label:    Label{Style: LabelPrefixed, Number: numStr},
			},
			href,
		})
	})
	return chapters
}

// FetchChapter fetches a chapter's pages
func (m *MangaCatalog) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaCatalogChapter)
	doc, err := m.get(mchap.URL)
	if err != nil {
		return nil, err
	}
	pages := mangaCatalogPages(doc)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		ID:         mchap.ID,
		Pages:      pages,
	}, nil
}

// mangaCatalogPages reads the reader's images: the first page has a real src,
// the rest a placeholder gif with the image in data-src (and a <noscript>
// copy, which the parser keeps as text, so nothing is listed twice)
func mangaCatalogPages(doc *goquery.Document) []Page {
	var pages []Page
	seen := map[string]bool{}
	doc.Find("img.js-page").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("data-src", ""))
		if src == "" {
			src = strings.TrimSpace(s.AttrOr("src", ""))
		}
		if src == "" || strings.HasPrefix(src, "data:") || seen[src] {
			return
		}
		seen[src] = true
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src})
	})
	return pages
}
