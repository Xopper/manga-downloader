// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// The web comic grabbers (comiceasel.go, webcomicsites.go and the platform
// ones) share one model: the comic is the series, and its chapters are the
// site's own chapters or storylines when it has them; otherwise every strip
// (or archive page) is a chapter, numbered by its position from the first one.

// wcGetDoc fetches and parses a page, remembering its URL in the document for
// resolving its links. A probe sets disableRetry, so a missing page fails at once.
func wcGetDoc(rawURL, referer string, disableRetry bool) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: rawURL, Referer: referer, DisableRetry: disableRetry})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	doc.Url, _ = url.Parse(rawURL)
	return doc, nil
}

// docURL is the URL a document was fetched from, see wcGetDoc
func docURL(doc *goquery.Document) *url.URL {
	if doc.Url != nil {
		return doc.Url
	}
	return &url.URL{}
}

// wcStrip is a strip of a comic whose strips are its chapters
type wcStrip struct {
	URL   string
	Title string
}

// wcStripChapters makes a chapter of each strip, numbered from 1 in reading order
func wcStripChapters(strips []wcStrip) Filterables {
	chapters := make(Filterables, 0, len(strips))
	for i, s := range strips {
		ch := &ComicEaselChapter{Chapter: wcStripChapter(s, i+1), Strips: []string{s.URL}}
		chapters = append(chapters, ch)
	}
	return chapters
}

// wcStripChapter is the chapter of a strip numbered n: its title, if any,
// goes after the number ("Chapter 12 - Title"), and its ID is its link
func wcStripChapter(s wcStrip, n int) Chapter {
	return Chapter{
		Title:  s.Title,
		Number: float64(n),
		ID:     chapterURLID(s.URL),
		Label:  Label{Style: LabelPrefixed},
	}
}

// wcNumbers numbers named chapters by the numbers their names start with
// ("Chapter 3", "14. Zam's Amporiam") when at least half the names have one
// and no two share it, the others being 0 (a "Misc" chapter); otherwise by
// their position from 1
func wcNumbers(names []string) []float64 {
	numbers := make([]float64, len(names))
	seen := map[float64]bool{}
	numbered := 0
	for i, name := range names {
		n, found := wcNameNumber(name)
		if !found {
			continue
		}
		if seen[n] {
			numbered = 0
			break
		}
		seen[n] = true
		numbers[i] = n
		numbered++
	}
	if numbered == 0 || 2*numbered < len(names) {
		for i := range numbers {
			numbers[i] = float64(i + 1)
		}
	}
	return numbers
}

// hostName names a site after its host, for a site that names itself
// nowhere: "buttsmithy.com" is "Buttsmithy"
func hostName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name, _, _ := strings.Cut(strings.TrimPrefix(strings.ToLower(u.Hostname()), "www."), ".")
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// wcNameNumber reads the number a chapter's name opens with
func wcNameNumber(name string) (float64, bool) {
	for _, re := range []*regexp.Regexp{ChapterTitlePrefix, bareNumber} {
		if m := re.FindStringSubmatch(name); m != nil {
			n, err := strconv.ParseFloat(m[1], 64)
			return n, err == nil
		}
	}
	return 0, false
}

// wcChapter is a named chapter: the name says the chapter word and number
// itself ("Chapter 3: Title") when it opens with them, so only the rest is
// its title; any other name goes after the number
func wcChapter(name string, number float64) Chapter {
	ch := Chapter{Number: number, Title: name, Label: Label{Style: LabelPrefixed}}
	if name == "" {
		ch.Label.Style = LabelNumbered
		return ch
	}
	m := ChapterTitlePrefix.FindStringSubmatchIndex(name)
	if m == nil {
		m = wcNumberPrefix.FindStringSubmatchIndex(name)
	}
	if m == nil && number == 0 {
		// an unnumbered chapter among numbered ones ("Misc.") is its name
		ch.Label.Style = LabelTitle
	}
	if m != nil {
		if n, err := strconv.ParseFloat(name[m[2]:m[3]], 64); err == nil && n == number {
			ch.Title = strings.TrimSpace(name[m[1]:])
			ch.Label = Label{Style: LabelNumbered, Number: name[m[2]:m[3]]}
			if ch.Title != "" {
				ch.Label.Style = LabelPrefixed
			}
		}
	}
	return ch
}

// wcNumberPrefix is a number a name opens with and its separator ("9: Counting Down")
var wcNumberPrefix = regexp.MustCompile(`^0*(\d+(?:\.\d+)?)\s*(?:[:.\-–—]\s*|$)`)

// wcStripPages reads each strip's page for its pictures, one after another
func wcStripPages(strips []string, referer string) ([]Page, error) {
	var pages []Page
	for _, strip := range strips {
		doc, err := wcGetDoc(strip, referer, false)
		if err != nil {
			return nil, err
		}
		for _, src := range wcComicImages(doc, wcComicSelector) {
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src})
		}
	}
	return pages, nil
}

// wcComicSelector finds a strip's pictures on Comic Easel and ComicPress
// pages, and on the many hand-made sites that copied their #comic element
const wcComicSelector = "#comic img, #comic-wrap img, .comic-table img, #cc-comic, #comicbody img"

// wcComicImages returns the pictures a selector finds, once each and without
// the hidden ones. A strip with a picture per language (dumbingofage.com) keeps
// the default one.
func wcComicImages(doc *goquery.Document, selector string) []string {
	images := doc.Find(selector)
	if def := images.Filter(".default-lang img"); def.Length() > 0 {
		images = def
	}
	base := docURL(doc)
	var srcs []string
	seen := map[string]bool{}
	images.Each(func(_ int, s *goquery.Selection) {
		if wpHiddenImage(s) {
			return
		}
		src := wcImageURL(s, base)
		if src == "" || seen[src] {
			return
		}
		seen[src] = true
		srcs = append(srcs, src)
	})
	return srcs
}

// wcImageURL is an image's picture: a lazy loader's attribute when it has
// one, else its src, resolved against the page
func wcImageURL(s *goquery.Selection, base *url.URL) string {
	src := ""
	// file is the images-lazyload-and-slideshow plugin's (3600000.xyz), whose src is a blank gif
	for _, attr := range []string{"data-src", "data-lazy-src", "data-orig-file", "file", "src"} {
		if v := strings.TrimSpace(s.AttrOr(attr, "")); v != "" && !strings.HasPrefix(v, "data:") {
			src = v
			break
		}
	}
	if src == "" {
		return ""
	}
	return resolveURLRelative(wcCFImageRe.ReplaceAllString(html.UnescapeString(src), "/"), base)
}

// wcCFImageRe is Cloudflare's image resizing prefix (/cdn-cgi/image/width=1440,format=auto/),
// whose format=auto may answer a format the packer can't name; the picture
// behind it is the original
var wcCFImageRe = regexp.MustCompile(`/cdn-cgi/image/[^/]+/`)

// wcLargestImage is the largest picture an image's srcset offers, or its src
func wcLargestImage(s *goquery.Selection, base *url.URL) string {
	best, width := "", 0
	for _, cand := range strings.Split(s.AttrOr("srcset", ""), ",") {
		fields := strings.Fields(cand)
		if len(fields) != 2 || !strings.HasSuffix(fields[1], "w") {
			continue
		}
		if w, err := strconv.Atoi(strings.TrimSuffix(fields[1], "w")); err == nil && w > width {
			best, width = fields[0], w
		}
	}
	if best == "" {
		return wcImageURL(s, base)
	}
	return resolveURLRelative(best, base)
}

// wpFeedNameRe reads a WordPress site's name from its feed link's title
// ("Kill Six Billion Demons » Feed", "Sandra and Woo RSS Feed")
var wpFeedNameRe = regexp.MustCompile(`^(.*?[\p{L}\p{N}].*?)\s*(?:»\s*)?(?:RSS\s*)?(?:2\.0\s*)?Feed$`)

// wpSiteName is a WordPress site's name: its main feed's, which every
// theme links in the head, else the one it gives Open Graph, else its title's
// last part
func wpSiteName(doc *goquery.Document) string {
	var name string
	doc.Find(`link[rel="alternate"][type="application/rss+xml"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if m := wpFeedNameRe.FindStringSubmatch(strings.TrimSpace(s.AttrOr("title", ""))); m != nil {
			name = m[1]
		}
		return name == ""
	})
	if name == "" {
		name = doc.Find(`meta[property="og:site_name"]`).AttrOr("content", "")
	}
	if name == "" {
		name = wcTitleSiteName(doc.Find("title").First().Text())
	}
	return sanitizeTitle(html.UnescapeString(name))
}

// wcTitleSeparators part a page title from the site's name
var wcTitleSeparators = []string{" | ", " – ", " — ", " - ", " :: "}

// wcTitleSiteName is the site's name in a page title: what follows its last
// separator, or the whole title
func wcTitleSiteName(title string) string {
	title = sanitizeTitle(title)
	for _, sep := range wcTitleSeparators {
		if i := strings.LastIndex(title, sep); i >= 0 {
			return strings.TrimSpace(title[i+len(sep):])
		}
	}
	return title
}

// errNoStrips is the error of a comic whose archive lists nothing
func errNoStrips(site string) error {
	return fmt.Errorf("no strips found in the archive of %s", site)
}

// noReferrer tells whether a page asks the browser to load its pictures
// without a referrer, which their host may require (images.mangafreak.me
// answers 403 to one, for w20.1punchman.com)
func noReferrer(doc *goquery.Document) bool {
	return doc.Find(`meta[name="referrer"][content="no-referrer"], meta[name="Referrer"][content="no-referrer"]`).Length() > 0
}
