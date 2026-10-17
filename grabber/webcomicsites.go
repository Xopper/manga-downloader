// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// WebComic is a grabber for hand-made web comic sites, one comic each, that
// no engine shares (xkcd.com, questionablecontent.net, gunnerkrigg.com): a
// table entry per site says where its archive is, which of its links are
// strips and which pictures of a strip's page are the comic. It matches by
// domain, as nothing in their markup is common. The series is the page the
// user gives when the site splits its archive (darthsanddroids.net's
// archive4.html is Episode IV), else the whole comic.
type WebComic struct {
	*Grabber
	site     *webComicSite
	doc      *goquery.Document
	chapters Filterables
	listed   bool
}

// WebComicChapter is a strip, or a chapter of strips, of a WebComic site
type WebComicChapter struct {
	Chapter
	// Strips are the pages holding the chapter's pictures, in reading order
	Strips []string
	// Pictures are the chapter's pictures, when the archive gives them
	Pictures []string
}

// webComicSite is how a WebComic site lists its strips
type webComicSite struct {
	// Hosts are the site's domains; their subdomains match too
	Hosts []string
	// Archive is the archive's path, when the series URL isn't the archive
	Archive string
	// Links selects the archive's links to strips
	Links string
	// NumberRe captures a strip's number from its link; without it, or when
	// it doesn't match, strips are numbered by their position
	NumberRe *regexp.Regexp
	// TitleRe captures a strip's title from its link's text, else the whole text is
	TitleRe *regexp.Regexp
	// NewestFirst is set when the archive lists the newest strip first
	NewestFirst bool
	// Image selects a strip page's pictures
	Image string
	// Name is the comic's name, else the archive's title is
	Name string
	// Title names a comic whose series are parts of its archive, from the
	// series URL
	Title func(series *url.URL) string
	// List reads an archive that isn't a flat list of strip links, in
	// reading order
	List func(doc *goquery.Document) ([]wcEntry, error)
}

// wcEntry is a chapter as an archive lists it
type wcEntry struct {
	Title    string
	Number   float64
	Volume   int
	Strips   []string
	Pictures []string
}

// NewWebComic returns a new WebComic grabber
func NewWebComic(g *Grabber) *WebComic {
	return &WebComic{Grabber: g}
}

// Test tells whether the URL is one of the sites webComicSites lists
func (w *WebComic) Test() (bool, error) {
	for i := range webComicSites {
		for _, host := range webComicSites[i].Hosts {
			if hostMatchesDomain(w.URL, host) {
				w.site = &webComicSites[i]
				return true, nil
			}
		}
	}
	return false, nil
}

// archive fetches the archive page once
func (w *WebComic) archive() (*goquery.Document, error) {
	if w.doc != nil {
		return w.doc, nil
	}
	if w.site == nil {
		if ok, _ := w.Test(); !ok {
			return nil, errors.New("not a known web comic")
		}
	}
	page := w.URL
	if w.site.Archive != "" {
		page = w.BaseUrl() + w.site.Archive
	}
	doc, err := wcGetDoc(page, w.BaseUrl(), false)
	if err != nil {
		return nil, err
	}
	w.doc = doc
	return doc, nil
}

// FetchTitle returns the comic's name, or its archive page's title
func (w *WebComic) FetchTitle() (string, error) {
	doc, err := w.archive()
	if err != nil {
		return "", err
	}
	if w.site.Title != nil {
		if u, err := url.Parse(w.URL); err == nil {
			return w.site.Title(u), nil
		}
	}
	if w.site.Name != "" {
		return w.site.Name, nil
	}
	return sanitizeTitle(doc.Find("title").First().Text()), nil
}

// FetchChapters returns the archive's strips or chapters
func (w *WebComic) FetchChapters() (Filterables, []error) {
	if w.listed {
		return w.chapters, nil
	}
	doc, err := w.archive()
	if err != nil {
		return nil, []error{err}
	}

	var entries []wcEntry
	if w.site.List != nil {
		entries, err = w.site.List(doc)
	} else {
		entries = w.site.links(doc)
	}
	if err != nil {
		return nil, []error{err}
	}
	if len(entries) == 0 {
		return nil, []error{errNoStrips(w.BaseUrl())}
	}

	language := pageLanguage(doc)
	for i, e := range entries {
		number := e.Number
		if number == 0 {
			number = float64(i + 1)
		}
		ch := &WebComicChapter{
			Chapter:  wcChapter(e.Title, number),
			Strips:   e.Strips,
			Pictures: e.Pictures,
		}
		ch.Volume = e.Volume
		ch.Language = language
		if len(e.Strips) == 1 {
			ch.ID = chapterURLID(e.Strips[0])
		}
		w.chapters = append(w.chapters, ch)
	}
	w.listed = true
	return w.chapters, nil
}

// links reads a flat archive: its strip links, oldest first, each numbered
// by its link when the site's links carry a number. A link listed twice is
// the first one.
func (s *webComicSite) links(doc *goquery.Document) []wcEntry {
	base := docURL(doc)
	var entries []wcEntry
	seen := map[string]bool{}
	numbered := s.NumberRe != nil
	doc.Find(s.Links).Each(func(_ int, a *goquery.Selection) {
		link := resolveURLRelative(a.AttrOr("href", ""), base)
		if link == "" || seen[link] {
			return
		}
		seen[link] = true
		title := sanitizeTitle(a.Text())
		if title == "" {
			title = sanitizeTitle(a.AttrOr("title", ""))
		}
		if s.TitleRe != nil {
			if m := s.TitleRe.FindStringSubmatch(title); m != nil {
				title = strings.TrimSpace(m[1])
			}
		}
		e := wcEntry{Title: title, Strips: []string{link}}
		if numbered {
			if m := s.NumberRe.FindStringSubmatch(link); m != nil {
				e.Number, _ = strconv.ParseFloat(m[1], 64)
			} else {
				numbered = false
			}
		}
		entries = append(entries, e)
	})
	if s.NewestFirst {
		slices.Reverse(entries)
	}
	if !numbered {
		for i := range entries {
			entries[i].Number = 0
		}
	}
	return entries
}

// FetchChapter returns a chapter's pictures: those the archive gave, else the
// comic pictures of each of its strips' pages
func (w *WebComic) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*WebComicChapter)
	var pages []Page
	for _, p := range ch.Pictures {
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: p})
	}
	for _, strip := range ch.Strips {
		doc, err := wcGetDoc(strip, w.BaseUrl(), false)
		if err != nil {
			return nil, err
		}
		for _, src := range wcComicImages(doc, w.site.Image) {
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src})
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      ch.Title,
		Number:     ch.Number,
		Label:      ch.Label,
		Volume:     ch.Volume,
		ID:         ch.ID,
		PagesCount: int64(len(pages)),
		Language:   ch.Language,
		Pages:      pages,
	}, nil
}

// webComicSites are the sites WebComic reads
var webComicSites = []webComicSite{
	{
		// xkcd: every strip is listed in /archive/, numbered in its link
		Hosts:       []string{"xkcd.com"},
		Name:        "xkcd",
		Archive:     "/archive/",
		Links:       `#middleContainer a[href]`,
		NumberRe:    regexp.MustCompile(`/(\d+)/?$`),
		NewestFirst: true,
		Image:       "#comic img",
	},
	{
		Hosts:       []string{"questionablecontent.net"},
		Name:        "Questionable Content",
		Archive:     "/archive.php",
		Links:       `a[href*="view.php?comic="]`,
		NumberRe:    regexp.MustCompile(`comic=(\d+)`),
		TitleRe:     regexp.MustCompile(`^See #\d+:\s*"?(.*?)"?(?:\s+with newspost)?$`),
		NewestFirst: true,
		Image:       "img#strip",
	},
	{
		// darthsanddroids.net: one archive page per film, the series
		Hosts:    []string{"darthsanddroids.net"},
		Links:    `a[href*="/episodes/"]`,
		NumberRe: regexp.MustCompile(`/episodes/(\d+)\.html`),
		Image:    `img[src*="/comics/"]`,
	},
	{
		Hosts:    []string{"existentialcomics.com"},
		Name:     "Existential Comics",
		Archive:  "/archive/byDate",
		Links:    `#date-comics li > a[href*="/comic/"]:first-child`,
		NumberRe: regexp.MustCompile(`/comic/(\d+)$`),
		Image:    "img.comicImg",
	},
	{
		Hosts:   []string{"swordscomic.com"},
		Name:    "Swords",
		Archive: "/archive/pages/",
		Links:   `a[href^="/comic/"][href$="/"]:not([href="/comic/"])`,
		TitleRe: regexp.MustCompile(`^(.*?)\s+\d\d \w{3} \d{4}$`),
		Image:   "img#comic-image",
	},
	{
		// reallifecomics.com: one archive page per year, the series
		Hosts: []string{"reallifecomics.com"},
		Title: func(series *url.URL) string {
			return strings.TrimSpace("Real Life Comics " + series.Query().Get("year"))
		},
		Links: `a[href*="comic.php?comic="], a[href*="comic-mobile.php?comic="]`,
		Image: `img[src*="/wp-content/uploads/"], img[src*="/comics/"]`,
	},
	{
		// gunnerkrigg.com: the archive names each chapter and lists its
		// pages, whose pictures are numbered after them
		Hosts:   []string{"gunnerkrigg.com"},
		Name:    "Gunnerkrigg Court",
		Archive: "/archives/",
		List:    gunnerkriggChapters,
	},
}

// gunnerkriggChapters reads Gunnerkrigg Court's archive: a link to each
// chapter's first page followed by a dropdown of its pages
func gunnerkriggChapters(doc *goquery.Document) ([]wcEntry, error) {
	var entries []wcEntry
	doc.Find("a.chapter_button").Each(func(_ int, a *goquery.Selection) {
		var pictures []string
		a.NextFilteredUntil("select", "a.chapter_button").First().Find("option").Each(func(_ int, o *goquery.Selection) {
			if n, err := strconv.Atoi(o.AttrOr("value", "")); err == nil {
				pictures = append(pictures, fmt.Sprintf("https://www.gunnerkrigg.com/comics/%08d.jpg", n))
			}
		})
		if len(pictures) == 0 {
			return
		}
		entries = append(entries, wcEntry{Title: sanitizeTitle(a.Find("strong").Text()), Pictures: pictures})
	})
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Title
	}
	for i, n := range wcNumbers(names) {
		entries[i].Number = n
	}
	return entries, nil
}
