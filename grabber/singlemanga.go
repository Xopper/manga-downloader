// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// SingleManga is a grabber for the "read X manga online" WordPress sites that
// host one series (w20.1punchman.com, imanevilgod.com, w9.kaijimanga.com,
// thegirlfromrandomchatting.com), or a few with a page each
// (ww7.readopm.com). Their themes differ (Toivo Lite, Kadence, a home-made
// one), but each lists its chapters as plain links on the series page, every
// one to a post holding the chapter's pages: it matches a page whose
// chapter links mostly share one series path. A chapter page links its
// series' home. thegirlfromrandomchatting.com draws its list from a REST
// route instead (`mg/v1/chapters-list`).
type SingleManga struct {
	*Grabber
	doc      *goquery.Document
	links    []smLink
	language string
}

// SingleMangaChapter is a chapter post of a SingleManga site
type SingleMangaChapter struct {
	Chapter
	URL string
}

// smLink is a chapter link of the series page
type smLink struct {
	URL   string
	Text  string
	Group string
}

// NewSingleManga returns a new SingleManga grabber
func NewSingleManga(g *Grabber) *SingleManga {
	return &SingleManga{Grabber: g}
}

// smMinChapters is how many chapters a page must link to be a series page
const smMinChapters = 5

// smChapterSlugRe finds the chapter number in a link's last path segment
// ("one-punch-man-chapter-240-1"); the series path is what comes before it
var smChapterSlugRe = regexp.MustCompile(`(?i)^(.*?)(?:chapter|ch|episode)[-_]?(\d+)(?:[-_.](\d+))?/?$`)

// smTextNumberRe reads the chapter number a link's text gives
var smTextNumberRe = regexp.MustCompile(`(?i)\b(?:chapter|ch\.?|episode)\s*(\d+(?:\.\d+)?)`)

// smListRoute is the REST route of the plugin that draws the list in the
// browser (#mg-chapters-list-container)
const smListRoute = "/wp-json/mg/v1/chapters-list"

// Test fetches the page and tells whether it's a WordPress page listing a
// series' chapters. A chapter page is tried through the site's home page.
func (s *SingleManga) Test() (bool, error) {
	doc, err := wcGetDoc(s.URL, "", true)
	if err != nil {
		return false, err
	}
	if doc.Find(`link[href*="/wp-content/"], script[src*="/wp-content/"], link[href*="/wp-json/"]`).Length() == 0 {
		return false, nil
	}
	if s.setPage(doc) {
		return true, nil
	}

	u, err := url.Parse(s.URL)
	if err != nil || strings.Trim(u.Path, "/") == "" {
		return false, nil
	}
	home := s.BaseUrl() + "/"
	if doc, err = wcGetDoc(home, s.URL, true); err != nil {
		return false, nil
	}
	if s.setPage(doc) {
		s.URL = home
		return true, nil
	}
	return false, nil
}

// setPage keeps doc when it lists a series' chapters
func (s *SingleManga) setPage(doc *goquery.Document) bool {
	if doc.Find("#mg-chapters-list-container").Length() > 0 {
		s.doc = doc
		s.language = pageLanguage(doc)
		return true
	}
	links := smChapterLinks(doc)
	if len(links) < smMinChapters {
		return false
	}
	s.doc, s.links = doc, links
	s.language = pageLanguage(doc)
	return true
}

// smChapterLinks returns the page's links to chapters of the series most of
// them belong to, once each, or nil when no series has most of them. A site
// that hosts a few series (w9.kaijimanga.com) is its main one.
func smChapterLinks(doc *goquery.Document) []smLink {
	base := docURL(doc)
	var all []smLink
	counts := map[string]int{}
	seen := map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		link, err := url.Parse(resolveURLRelative(a.AttrOr("href", ""), base))
		if err != nil || !sameSite(link.Hostname(), base.Hostname()) {
			return
		}
		m := smChapterSlugRe.FindStringSubmatch(link.Path)
		if m == nil {
			return
		}
		link.Fragment = ""
		u := link.String()
		if seen[u] {
			return
		}
		seen[u] = true
		group := strings.ToLower(m[1])
		counts[group]++
		all = append(all, smLink{URL: u, Text: sanitizeTitle(a.Text()), Group: group})
	})

	best := ""
	for group, n := range counts {
		if n > counts[best] || (n == counts[best] && group < best) {
			best = group
		}
	}
	if counts[best] < smMinChapters || 2*counts[best] <= len(all) {
		return nil
	}
	var links []smLink
	for _, l := range all {
		if l.Group == best {
			links = append(links, l)
		}
	}
	return links
}

// sameSite tells whether two hosts are the same site: the same last two
// labels, as these sites move between numbered subdomains (1punchman.com
// redirects to w20.1punchman.com)
func sameSite(a, b string) bool {
	return strings.EqualFold(siteDomain(a), siteDomain(b))
}

// siteDomain is a host's last two labels
func siteDomain(host string) string {
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	if len(labels) > 2 {
		labels = labels[len(labels)-2:]
	}
	return strings.Join(labels, ".")
}

// FetchTitle returns the series' name, as its chapter links give it before
// the chapter word ("One-Punch Man – Chapter 242"), else the page's heading
func (s *SingleManga) FetchTitle() (string, error) {
	if s.doc == nil {
		if _, err := s.Test(); err != nil {
			return "", err
		}
	}
	links, err := s.chapterLinks()
	if err != nil {
		return "", err
	}
	if name := smSeriesName(links); name != "" {
		return name, nil
	}
	if name := smPageName(s.doc.Find("title").First().Text()); name != "" {
		return name, nil
	}
	return wpSiteName(s.doc), nil
}

// smReadRe is what these sites wrap a series' name in, in their titles
// ("Read One Punch Man Manga Online")
var smReadRe = regexp.MustCompile(`(?i)^read\s+|\s+(?:manga|manhwa|manhua)?\s*online\b.*$`)

// smPageName is the series' name in a page's title: its first part ("Mob
// Psycho 100 - Read One Punch Man Manga Online")
func smPageName(title string) string {
	title = sanitizeTitle(html.UnescapeString(title))
	for _, sep := range wcTitleSeparators {
		if i := strings.Index(title, sep); i > 0 {
			title = title[:i]
			break
		}
	}
	return smKindRe.ReplaceAllString(smReadRe.ReplaceAllString(title, ""), "")
}

// smSeriesWordRe finds the chapter word a link's text names the series before
var smSeriesWordRe = regexp.MustCompile(`(?i)\s*[,:–—-]?\s*\b(?:chapter|ch\.|episode)\b`)

// smSeriesName is the name most chapter links give before the chapter word
func smSeriesName(links []smLink) string {
	counts := map[string]int{}
	for _, l := range links {
		if loc := smSeriesWordRe.FindStringIndex(l.Text); loc != nil && loc[0] > 0 {
			counts[strings.TrimSpace(l.Text[:loc[0]])]++
		}
	}
	best := ""
	for name, n := range counts {
		if n > counts[best] || (n == counts[best] && name < best) {
			best = name
		}
	}
	if 2*counts[best] <= len(links) {
		return ""
	}
	return smKindRe.ReplaceAllString(best, "")
}

// smKindRe is the kind word a series name may end with ("I'm An Evil God Manhua")
var smKindRe = regexp.MustCompile(`(?i)\s+(?:manga|manhwa|manhua|webtoon|comics?)$`)

// chapterLinks returns the series' chapter links, read from the REST route
// for a site that draws the list in the browser
func (s *SingleManga) chapterLinks() ([]smLink, error) {
	if s.links != nil || s.doc.Find("#mg-chapters-list-container").Length() == 0 {
		return s.links, nil
	}
	body, err := http.GetText(http.RequestParams{URL: s.BaseUrl() + smListRoute, Referer: s.URL})
	if err != nil {
		return nil, err
	}
	var list []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return nil, err
	}
	for _, it := range list {
		s.links = append(s.links, smLink{URL: it.URL, Text: sanitizeTitle(html.UnescapeString(it.Title))})
	}
	return s.links, nil
}

// FetchChapters returns the series' chapters, numbered by their links' text,
// else by their URLs, whose trailing "-1" may be a decimal (readopm's
// "chapter-100-7" is 100.7) or WordPress telling two posts apart
func (s *SingleManga) FetchChapters() (Filterables, []error) {
	if s.doc == nil {
		if _, err := s.Test(); err != nil {
			return nil, []error{err}
		}
	}
	links, err := s.chapterLinks()
	if err != nil {
		return nil, []error{err}
	}
	var chapters Filterables
	var errs []error
	for _, l := range links {
		num, ok := smChapterNumber(l)
		if !ok {
			errs = append(errs, fmt.Errorf("couldn't read the chapter number of %s", l.URL))
			continue
		}
		chapters = append(chapters, &SingleMangaChapter{
			Chapter: Chapter{
				Number:   num,
				ID:       chapterURLID(l.URL),
				Language: s.language,
				Label:    Label{Style: LabelNumbered},
			},
			URL: l.URL,
		})
	}
	// oldest first, as a reader goes; the pages list the newest first
	return chapters.SortByNumber(), errs
}

// smChapterNumber reads a chapter link's number, from its text, else its URL
func smChapterNumber(l smLink) (float64, bool) {
	if m := smTextNumberRe.FindStringSubmatch(l.Text); m != nil {
		n, err := strconv.ParseFloat(m[1], 64)
		return n, err == nil
	}
	u, err := url.Parse(l.URL)
	if err != nil {
		return 0, false
	}
	m := smChapterSlugRe.FindStringSubmatch(u.Path)
	if m == nil {
		return 0, false
	}
	num := m[2]
	if m[3] != "" {
		num += "." + m[3]
	}
	n, err := strconv.ParseFloat(num, 64)
	return n, err == nil
}

// smReaderImages finds a chapter's pages outside a post's content (readopm's reader)
const smReaderImages = "img.pages__img, .reading-content img, #readerarea img"

// FetchChapter fetches a chapter post's pages
func (s *SingleManga) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*SingleMangaChapter)
	doc, err := wcGetDoc(ch.URL, s.BaseUrl()+"/", false)
	if err != nil {
		return nil, err
	}
	pages := wpPageImages(doc)
	if len(pages) == 0 {
		for _, src := range wcComicImages(doc, smReaderImages) {
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src})
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	if noReferrer(doc) {
		for i := range pages {
			pages[i].NoReferer = true
		}
	}
	return &Chapter{
		Title:      ch.Title,
		Number:     ch.Number,
		Label:      ch.Label,
		ID:         ch.ID,
		PagesCount: int64(len(pages)),
		Language:   s.language,
		Pages:      pages,
	}, nil
}
