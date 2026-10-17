// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"html"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
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
	// Index is the page listing the chapter's strips, when the archive didn't
	Index string
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
	// ArchiveURL is the archive of a site that hosts many comics, from the
	// URL of any of its pages
	ArchiveURL func(series *url.URL) string
	// IndexLinks selects a chapter index's links to its strips, for an
	// archive that lists chapters whose strips are listed on a page of their
	// own (wcEntry.Index)
	IndexLinks string
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
	// series URL and its archive page
	Title func(series *url.URL, doc *goquery.Document) string
	// Next selects the link to the archive's next page, for an archive split
	// over pages
	Next string
	// OneChapter makes a single chapter of everything the archive lists, for
	// a series that is itself one chapter (a storyline of comicaurora.com)
	OneChapter bool
	// NoReferer downloads the pictures without the site's Referer, for an
	// image host that refuses it (kiutaku.com's mitaku.net)
	NoReferer bool
	// SortPictures puts a strip page's pictures in the order of the numbers
	// their files end with, for a page that lays them out backwards
	SortPictures bool
	// List reads an archive that isn't a flat list of strip links, in
	// reading order
	List func(doc *goquery.Document) ([]wcEntry, error)
}

// wcEntry is a chapter as an archive lists it
type wcEntry struct {
	// Index is the page listing the chapter's strips, read when it's fetched
	Index  string
	Title  string
	Number float64
	// HasNumber is set when Number is the site's, else the position numbers it
	HasNumber bool
	Volume    int
	Strips    []string
	Pictures  []string
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
	if w.site.ArchiveURL != nil {
		u, err := url.Parse(w.URL)
		if err != nil {
			return nil, err
		}
		if page = w.site.ArchiveURL(u); page == "" {
			return nil, errSeriesNotFound(w.URL)
		}
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
			return w.site.Title(u, doc), nil
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

	docs := []*goquery.Document{doc}
	if w.site.Next != "" {
		docs = append(docs, w.nextPages(doc)...)
	}
	var entries []wcEntry
	for _, doc := range docs {
		var more []wcEntry
		if w.site.List != nil {
			more, err = w.site.List(doc)
		} else {
			more = w.site.links(doc)
		}
		if err != nil {
			return nil, []error{err}
		}
		entries = append(entries, more...)
	}
	if w.site.NewestFirst {
		slices.Reverse(entries)
	}
	if w.site.OneChapter && len(entries) > 0 {
		one := wcEntry{Number: 1, HasNumber: true}
		for _, e := range entries {
			one.Strips = append(one.Strips, e.Strips...)
			one.Pictures = append(one.Pictures, e.Pictures...)
		}
		entries = []wcEntry{one}
	}
	if len(entries) == 0 {
		return nil, []error{errNoStrips(w.BaseUrl())}
	}

	language := pageLanguage(doc)
	numbered := !slices.ContainsFunc(entries, func(e wcEntry) bool { return !e.HasNumber })
	for i, e := range entries {
		number := e.Number
		if !numbered {
			number = float64(i + 1)
		}
		ch := &WebComicChapter{
			Chapter:  wcChapter(e.Title, number),
			Strips:   e.Strips,
			Pictures: e.Pictures,
			Index:    e.Index,
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

// links reads a flat archive page: its strip links, each numbered by its
// link when the site's links carry a number. A link listed twice is the first one.
func (s *webComicSite) links(doc *goquery.Document) []wcEntry {
	base := docURL(doc)
	var entries []wcEntry
	seen := map[string]bool{}
	doc.Find(s.Links).Each(func(_ int, a *goquery.Selection) {
		link := resolveURLRelative(a.AttrOr("href", ""), base)
		if link == "" || seen[link] {
			return
		}
		seen[link] = true
		title := sanitizeTitle(a.Text())
		if title == "" {
			title = sanitizeTitle(a.AttrOr("title", a.Find("img").AttrOr("alt", "")))
		}
		if s.TitleRe != nil {
			if m := s.TitleRe.FindStringSubmatch(title); m != nil {
				title = strings.TrimSpace(m[1])
			}
		}
		e := wcEntry{Title: title, Strips: []string{link}}
		if s.NumberRe != nil {
			if m := s.NumberRe.FindStringSubmatch(link); m != nil {
				e.Number, _ = strconv.ParseFloat(m[1], 64)
				e.HasNumber = true
			}
		}
		entries = append(entries, e)
	})
	return entries
}

// wcMaxArchivePages bounds the pages of an archive split over pages
const wcMaxArchivePages = 100

// nextPages follows the archive's next page links, and returns the pages
// after the first
func (w *WebComic) nextPages(doc *goquery.Document) []*goquery.Document {
	var docs []*goquery.Document
	seen := map[string]bool{docURL(doc).String(): true}
	for len(docs) < wcMaxArchivePages {
		next := resolveURLRelative(doc.Find(w.site.Next).First().AttrOr("href", ""), docURL(doc))
		if next == "" || seen[next] {
			break
		}
		seen[next] = true
		var err error
		if doc, err = wcGetDoc(next, w.BaseUrl(), false); err != nil {
			break
		}
		docs = append(docs, doc)
	}
	return docs
}

// FetchChapter returns a chapter's pictures: those the archive gave, else the
// comic pictures of each of its strips' pages
func (w *WebComic) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*WebComicChapter)
	strips := ch.Strips
	if ch.Index != "" {
		doc, err := wcGetDoc(ch.Index, w.BaseUrl(), false)
		if err != nil {
			return nil, err
		}
		strips = wcLinks(doc, w.site.IndexLinks)
	}
	var pages []Page
	for _, p := range ch.Pictures {
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: p})
	}
	for _, strip := range strips {
		doc, err := wcGetDoc(strip, w.BaseUrl(), false)
		if err != nil {
			return nil, err
		}
		noRef := w.site.NoReferer || noReferrer(doc)
		srcs := wcComicImages(doc, w.site.Image)
		if w.site.SortPictures {
			slices.SortStableFunc(srcs, func(a, b string) int { return wcFileNumber(a) - wcFileNumber(b) })
		}
		for _, src := range srcs {
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src, NoReferer: noRef})
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

// wcLinks returns the links a selector finds, resolved, once each
func wcLinks(doc *goquery.Document, selector string) []string {
	var links []string
	seen := map[string]bool{}
	doc.Find(selector).Each(func(_ int, a *goquery.Selection) {
		if u := resolveURLRelative(a.AttrOr("href", ""), docURL(doc)); u != "" && !seen[u] {
			seen[u] = true
			links = append(links, u)
		}
	})
	return links
}

// wcFileNumberRe reads the number a picture's file name ends with ("img_4.png")
var wcFileNumberRe = regexp.MustCompile(`(\d+)\.\w+(?:\?.*)?$`)

// wcFileNumber is the number a picture's file name ends with, 0 without one
func wcFileNumber(src string) int {
	n := 0
	if m := wcFileNumberRe.FindStringSubmatch(src); m != nil {
		n, _ = strconv.Atoi(m[1])
	}
	return n
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
		Title: func(series *url.URL, _ *goquery.Document) string {
			return strings.TrimSpace("Real Life Comics " + series.Query().Get("year"))
		},
		Links: `a[href*="comic.php?comic="], a[href*="comic-mobile.php?comic="]`,
		Image: `img[src*="/wp-content/uploads/"], img[src*="/comics/"]`,
	},
	{
		// peppercarrot.com: a page per series, newest episode first
		Hosts:       []string{"peppercarrot.com"},
		Links:       `figure.thumbnail a[href$=".html"]`,
		NumberRe:    regexp.MustCompile(`/(\d+)\.html$`),
		TitleRe:     regexp.MustCompile(`^\d+\.\s*(.*?)(?:\s*\(click to open the episode\))?$`),
		NewestFirst: true,
		Image:       `img[src*="/0_sources/"]`,
	},
	{
		// dragonball-multiverse.com: a chapters page per comic, the series
		Hosts: []string{"dragonball-multiverse.com"},
		List:  dbmChapters,
		Image: `img[alt="[DBM PAGE IMAGE]"], #balloonsimg img`,
		Title: func(_ *url.URL, doc *goquery.Document) string {
			return sanitizeTitle(doc.Find("h1.horscadrelect").First().Text())
		},
	},
	{
		// sas.ewanb.me (Solar and Sundry): the home page lists every page
		Hosts:    []string{"sas.ewanb.me"},
		Name:     "Solar and Sundry",
		Archive:  "/",
		Links:    `a[href^="/comic/"]`,
		NumberRe: regexp.MustCompile(`/comic/(\d+)$`),
		TitleRe:  regexp.MustCompile(`^p\d+\s*[—–-]\s*(.*)$`),
		Image:    "img.comic:not(.thumbnail)",
	},
	{
		// commitstrip.com: a year's archive (/en/2017) is the series, 20
		// strips a page, newest first
		Hosts:       []string{"commitstrip.com"},
		Links:       ".excerpt a[href]",
		Next:        "a.nextpostslink",
		NewestFirst: true,
		Image:       ".entry-content img",
		Title: func(series *url.URL, _ *goquery.Document) string {
			year := regexp.MustCompile(`/((?:19|20)\d\d)\b`).FindStringSubmatch(series.Path)
			if year == nil {
				return "CommitStrip"
			}
			return "CommitStrip " + year[1]
		},
	},
	{
		// elan.school: the chapters page, a dozen pages of chapter posts
		Hosts: []string{"elan.school"},
		Name:  "Elan School",
		Links: ".listing-item a.title",
		Next:  `a:contains("Next")`,
		Image: `.cb-entry img[class*="wp-image-"], .entry-content img[class*="wp-image-"]`,
	},
	{
		// holoearth.com: each manga's pages list its episodes, whose
		// pictures the page lays out last first
		Hosts:        []string{"holoearth.com"},
		List:         holoearthChapters,
		Image:        `img[src*="/comic/"]`,
		SortPictures: true,
		Title: func(_ *url.URL, doc *goquery.Document) string {
			return sanitizeTitle(holoearthTitleRe.ReplaceAllString(doc.Find("title").First().Text(), ""))
		},
	},
	{
		// comicfury.com hosts many comics, at /read/{slug}/
		Hosts: []string{"comicfury.com"},
		ArchiveURL: func(series *url.URL) string {
			m := comicFurySlugRe.FindStringSubmatch(series.Path)
			if m == nil {
				return ""
			}
			return "https://comicfury.com/read/" + m[1] + "/archive"
		},
		List:       comicFuryChapters,
		IndexLinks: comicFuryPages,
		Image:      `img[src*="img.comicfury.com/comics/"]`,
		Title: func(_ *url.URL, doc *goquery.Document) string {
			return sanitizeTitle(doc.Find(`a[href^="/comicprofile.php"]`).First().Text())
		},
	},
	{
		// theduckwebcomics.com (Drunk Duck) hosts many comics, at
		// /{Comic}/; every page is a chapter
		Hosts:       []string{"theduckwebcomics.com"},
		List:        duckChapters,
		NewestFirst: true,
		Image:       "img.page-image",
		Title: func(series *url.URL, _ *goquery.Document) string {
			comic := strings.SplitN(strings.Trim(series.Path, "/"), "/", 2)[0]
			return strings.ReplaceAll(comic, "_", " ")
		},
	},
	{
		// comicaurora.com (Webcomic plugin): a storyline's archive is a
		// series of one chapter, its pages shown 10 at a time
		Hosts:      []string{"comicaurora.com"},
		List:       webcomicPluginPictures,
		Next:       `link[rel="next"]`,
		OneChapter: true,
		Title: func(_ *url.URL, doc *goquery.Document) string {
			name, site, _ := strings.Cut(sanitizeTitle(doc.Find("title").First().Text()), " - ")
			return strings.TrimSpace(site + " - " + strings.TrimSuffix(name, " Archives"))
		},
	},
	{
		// mehgazone.com hosts a comic per subdomain, whose posts are filed
		// under chapters
		Hosts: []string{"mehgazone.com"},
		List:  wpChapterTaxonomy,
		Title: func(_ *url.URL, doc *goquery.Document) string {
			return wpSiteName(doc)
		},
	},
	// Photo galleries (NSFW): each post is a series of one chapter
	{
		Hosts: []string{"everia.club"},
		List:  galleryList(".page-links a, a.post-page-numbers"),
		Image: ".wp-block-image img, .entry-content img",
		Title: galleryTitle,
	},
	{
		Hosts: []string{"jjcos.com"},
		List:  galleryList("a.post-page-numbers"),
		Image: ".post-content img",
		Title: galleryTitle,
	},
	{
		Hosts: []string{"misskon.com"},
		List:  galleryList("a.post-page-numbers"),
		Image: "img.aligncenter",
		Title: galleryTitle,
	},
	{
		Hosts:     []string{"kiutaku.com"},
		NoReferer: true,
		List:      galleryList("a.pagination-link"),
		Image:     ".article-fulltext img",
		Title:     galleryTitle,
	},
	{
		Hosts: []string{"3600000.xyz"},
		List:  galleryList("a.post-page-numbers"),
		Image: ".entry-content img",
		Title: galleryTitle,
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
		entries[i].Number, entries[i].HasNumber = n, true
	}
	return entries, nil
}

// dbmChapters reads a Dragon Ball Multiverse comic's chapters page: each
// chapter's heading and the links to its pages
func dbmChapters(doc *goquery.Document) ([]wcEntry, error) {
	base := docURL(doc)
	var entries []wcEntry
	doc.Find(".chapters-wrapper .chapter").Each(func(_ int, ch *goquery.Selection) {
		var strips []string
		ch.Find("p a[href]").Each(func(_ int, a *goquery.Selection) {
			if u := resolveURLRelative(a.AttrOr("href", ""), base); u != "" {
				strips = append(strips, u)
			}
		})
		if len(strips) > 0 {
			entries = append(entries, wcEntry{Title: sanitizeTitle(ch.Find("h4").First().Text()), Strips: strips})
		}
	})
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Title
	}
	for i, n := range wcNumbers(names) {
		entries[i].Number, entries[i].HasNumber = n, true
	}
	return entries, nil
}

// comicFurySlugRe reads a Comic Fury comic's slug from any of its pages
var comicFurySlugRe = regexp.MustCompile(`^/read/([^/]+)`)

// comicFuryChapters reads a Comic Fury comic's archive: its chapters, each
// with an index of its pages, or its pages when it has no chapters
func comicFuryChapters(doc *goquery.Document) ([]wcEntry, error) {
	base := docURL(doc)
	var entries []wcEntry
	for _, link := range wcLinks(doc, `a[href*="/archive/chapter/"]`) {
		a := doc.Find(`a[href="` + strings.TrimPrefix(link, base.Scheme+"://"+base.Host) + `"]`).First()
		entries = append(entries, wcEntry{Title: sanitizeTitle(a.Text()), Index: link})
	}
	if len(entries) == 0 {
		for _, link := range wcLinks(doc, comicFuryPages) {
			entries = append(entries, wcEntry{Strips: []string{link}})
		}
		return entries, nil
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Title
	}
	for i, n := range wcNumbers(names) {
		entries[i].Number, entries[i].HasNumber = n, true
	}
	return entries, nil
}

// comicFuryPages selects a Comic Fury index's links to its pages, which
// end in their ID ("first" and "last" don't)
const comicFuryPages = `a[href*="/comics/"]:not([href$="/first"]):not([href$="/last"])`

// duckPageRe reads the page number The Duck's dropdown names a page with
var duckPageRe = regexp.MustCompile(`(?i)page\s*(\d+)\s*$`)

// duckChapters reads the page dropdown of a comic on The Duck, which lists
// every page, newest first
func duckChapters(doc *goquery.Document) ([]wcEntry, error) {
	base := docURL(doc)
	comic := strings.SplitN(strings.Trim(base.Path, "/"), "/", 2)[0]
	var entries []wcEntry
	doc.Find("select#page_dropdown option[value]").Each(func(_ int, o *goquery.Selection) {
		link := resolveURLRelative("/"+comic+"/"+o.AttrOr("value", "")+"/", base)
		text := sanitizeTitle(o.Text())
		e := wcEntry{Strips: []string{link}}
		if m := duckPageRe.FindStringSubmatchIndex(text); m != nil {
			e.Number, _ = strconv.ParseFloat(text[m[2]:m[3]], 64)
			e.HasNumber = true
			e.Title = strings.Trim(text[:m[0]], " -")
		}
		entries = append(entries, e)
	})
	return entries, nil
}

// galleryPageRe reads the page number a gallery's pager link ends with:
// WordPress' /2/ or ?page=2
var galleryPageRe = regexp.MustCompile(`^(.*?[/=])(\d+)(/?)$`)

// galleryList reads a photo gallery post, which is a series of one chapter:
// its pages are the post and the pages its pager (the selector) links to,
// counted up to the last one when the pager skips some ("1 2 3 … 9")
func galleryList(pager string) func(doc *goquery.Document) ([]wcEntry, error) {
	return func(doc *goquery.Document) ([]wcEntry, error) {
		first := docURL(doc).String()
		strips := []string{first}
		last, prefix, suffix := 1, "", ""
		for _, link := range wcLinks(doc, pager) {
			// a page of this post, not of another listing
			if !strings.HasPrefix(link, strings.TrimSuffix(first, "/")) || link == first {
				continue
			}
			if m := galleryPageRe.FindStringSubmatch(link); m != nil {
				if n, _ := strconv.Atoi(m[2]); n > last {
					last, prefix, suffix = n, m[1], m[3]
				}
			}
		}
		for n := 2; n <= last; n++ {
			strips = append(strips, prefix+strconv.Itoa(n)+suffix)
		}
		return []wcEntry{{Number: 1, HasNumber: true, Strips: strips}}, nil
	}
}

// galleryTitle names a gallery after its heading, else its page title
func galleryTitle(_ *url.URL, doc *goquery.Document) string {
	if h1 := sanitizeTitle(doc.Find("h1").First().Text()); h1 != "" {
		return h1
	}
	return smPageName(doc.Find("title").First().Text())
}

// webcomicPluginPictures reads an archive page of the Webcomic plugin
// (comicaurora.com), which shows each page's picture, as a thumbnail whose
// srcset holds the original
func webcomicPluginPictures(doc *goquery.Document) ([]wcEntry, error) {
	var entries []wcEntry
	doc.Find(`article[class*="type-webcomic"] .entry a.webcomic-link img`).Each(func(_ int, img *goquery.Selection) {
		if src := wcLargestImage(img, docURL(doc)); src != "" {
			entries = append(entries, wcEntry{Pictures: []string{src}})
		}
	})
	return entries, nil
}

// wpChapterTaxonomy reads a WordPress comic that files its posts under a
// "chapter" taxonomy (the mehgazone.com comics) through the REST API: every
// post, oldest first, with its pictures, grouped by chapter in the order the
// chapters start
func wpChapterTaxonomy(doc *goquery.Document) ([]wcEntry, error) {
	base := docURL(doc).Scheme + "://" + docURL(doc).Host + "/wp-json/wp/v2/"
	get := func(path string, v any) error {
		body, err := http.GetText(http.RequestParams{URL: base + path, Referer: base})
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(body), v)
	}

	var terms []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := get("chapter?per_page=100&_fields=id,name", &terms); err != nil {
		return nil, err
	}
	names := map[int]string{}
	for _, t := range terms {
		names[t.ID] = sanitizeTitle(html.UnescapeString(t.Name))
	}

	byTerm := map[int]*wcEntry{}
	var entries []*wcEntry
	for page := 1; page <= 100; page++ {
		var posts []struct {
			Chapter []int `json:"chapter"`
			Content struct {
				Rendered string `json:"rendered"`
			} `json:"content"`
		}
		err := get(fmt.Sprintf("posts?per_page=%d&page=%d&orderby=date&order=asc&_fields=chapter,content", wpPerPage, page), &posts)
		var statusErr *http.StatusError
		if page > 1 && errors.As(err, &statusErr) && statusErr.Code == 400 {
			break
		}
		if err != nil {
			return nil, err
		}
		for _, p := range posts {
			if len(p.Chapter) == 0 {
				continue
			}
			e := byTerm[p.Chapter[0]]
			if e == nil {
				e = &wcEntry{Title: names[p.Chapter[0]]}
				byTerm[p.Chapter[0]] = e
				entries = append(entries, e)
			}
			content, err := goquery.NewDocumentFromReader(strings.NewReader(p.Content.Rendered))
			if err != nil {
				continue
			}
			content.Url = docURL(doc)
			e.Pictures = append(e.Pictures, wcComicImages(content, `img[class*="wp-image-"]`)...)
		}
		if len(posts) < wpPerPage {
			break
		}
	}

	list := make([]wcEntry, 0, len(entries))
	titles := make([]string, 0, len(entries))
	for _, e := range entries {
		list = append(list, *e)
		titles = append(titles, e.Title)
	}
	for i, n := range wcNumbers(titles) {
		list[i].Number, list[i].HasNumber = n, true
	}
	return list, nil
}

// holoearthEpisodeRe reads an episode link of a holoearth.com manga: the
// manga's path and the episode's number
var holoearthEpisodeRe = regexp.MustCompile(`^(.*/manga/[^/]+/)ep(\d+)/?$`)

// holoearthChapters reads the episode list a holoearth.com manga's pages
// carry, keeping the episodes of the manga the page belongs to
func holoearthChapters(doc *goquery.Document) ([]wcEntry, error) {
	base := docURL(doc)
	series := base.Path
	if m := holoearthEpisodeRe.FindStringSubmatch(series); m != nil {
		series = m[1]
	}
	var entries []wcEntry
	seen := map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		link := resolveURLRelative(a.AttrOr("href", ""), base)
		u, err := url.Parse(link)
		if err != nil || seen[link] {
			return
		}
		m := holoearthEpisodeRe.FindStringSubmatch(u.Path)
		if m == nil || !strings.HasPrefix(series, m[1]) {
			return
		}
		seen[link] = true
		n, _ := strconv.ParseFloat(m[2], 64)
		entries = append(entries, wcEntry{Number: n, HasNumber: true, Strips: []string{link}})
	})
	slices.SortFunc(entries, func(a, b wcEntry) int { return int(a.Number - b.Number) })
	return entries, nil
}

// holoearthTitleRe is the chapter and site a holoearth.com page title wraps
// the manga's name in ("【Chapter 0】Vesta de Cooking | HOLONOMETRIA")
var holoearthTitleRe = regexp.MustCompile(`【[^】]*】|\s*\|.*$`)
