// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// webcomicSpec describes a small single-series webcomic: an archive page
// lists every strip (or batch of strips), and each one's page shows its
// images. As Tachiyomi does, the whole comic is one series and each archive
// entry a chapter. Each site's quirks fit in these few functions.
type webcomicSpec struct {
	// domains the site answers on (any subdomain matches)
	domains []string
	// title is the series name; titleOf, when set, reads it off the archive
	title   string
	titleOf func(doc *goquery.Document) string
	// listURL maps any link of the site (archive, strip, home) to its
	// archive page, without fetching
	listURL func(u *url.URL) (string, error)
	// fetchList, when set, replaces the plain GET of the archive
	fetchList func(uri string) (*goquery.Document, error)
	// entries reads the archive, in reading order
	entries func(doc *goquery.Document, base *url.URL) []webcomicEntry
	// images reads a strip page's images (resolved by the caller)
	images func(doc *goquery.Document) []string
}

// webcomicEntry is one archive entry
type webcomicEntry struct {
	Number float64
	Title  string
	URL    string
	// Prefixed labels "Chapter N - Title" (strips numbered by the site);
	// otherwise the title alone, or "Chapter N" without one
	Prefixed bool
}

// Webcomic is the grabber for the single-series webcomics in webcomicSpecs,
// matched by domain (each is one site with its own hand-made archive)
type Webcomic struct {
	*Grabber
	spec *webcomicSpec
	doc  *goquery.Document
}

// WebcomicChapter is an archive entry
type WebcomicChapter struct {
	Chapter
	URL string
}

// NewWebcomic returns a new Webcomic grabber
func NewWebcomic(g *Grabber) *Webcomic {
	return &Webcomic{Grabber: g}
}

// Test matches the domains of webcomicSpecs
func (w *Webcomic) Test() (bool, error) {
	for _, s := range webcomicSpecs {
		for _, d := range s.domains {
			if hostMatchesDomain(w.URL, d) {
				w.spec = s
				return true, nil
			}
		}
	}
	return false, nil
}

// archive fetches the archive page once
func (w *Webcomic) archive() (*goquery.Document, *url.URL, error) {
	if w.spec == nil {
		if ok, _ := w.Test(); !ok {
			return nil, nil, errSeriesNotFound(w.URL)
		}
	}
	u, err := url.Parse(w.URL)
	if err != nil {
		return nil, nil, err
	}
	list, err := w.spec.listURL(u)
	if err != nil {
		return nil, nil, err
	}
	base, err := url.Parse(list)
	if err != nil {
		return nil, nil, err
	}
	if w.doc != nil {
		return w.doc, base, nil
	}
	fetch := w.spec.fetchList
	if fetch == nil {
		fetch = webcomicGet
	}
	doc, err := fetch(list)
	if err != nil {
		return nil, nil, err
	}
	w.doc = doc
	return doc, base, nil
}

// webcomicGet fetches and parses a page
func webcomicGet(uri string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

// FetchTitle returns the comic's name
func (w *Webcomic) FetchTitle() (string, error) {
	doc, _, err := w.archive()
	if err != nil {
		return "", err
	}
	if w.spec.titleOf != nil {
		if t := sanitizeTitle(w.spec.titleOf(doc)); t != "" {
			return t, nil
		}
	}
	return w.spec.title, nil
}

// FetchChapters returns the archive entries
func (w *Webcomic) FetchChapters() (Filterables, []error) {
	doc, base, err := w.archive()
	if err != nil {
		return nil, []error{err}
	}
	var chapters Filterables
	for _, e := range w.spec.entries(doc, base) {
		style := LabelNumbered
		if e.Prefixed {
			style = LabelPrefixed
		}
		chapters = append(chapters, &WebcomicChapter{
			Chapter{
				Number:   e.Number,
				Title:    sanitizeTitle(e.Title),
				Language: "en",
				ID:       chapterURLID(e.URL),
				Label:    Label{Style: style},
			},
			e.URL,
		})
	}
	if len(chapters) == 0 {
		return nil, []error{fmt.Errorf("no strips found in the archive of %s", w.URL)}
	}
	return chapters, nil
}

// FetchChapter reads the entry's page for its images
func (w *Webcomic) FetchChapter(f Filterable) (*Chapter, error) {
	wchap := f.(*WebcomicChapter)
	if w.spec == nil {
		if ok, _ := w.Test(); !ok {
			return nil, errSeriesNotFound(w.URL)
		}
	}
	doc, err := webcomicGet(wchap.URL)
	if err != nil {
		return nil, err
	}
	pages := webcomicPages(w.spec.images(doc), wchap.URL)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		ID:         wchap.ID,
		Pages:      pages,
	}, nil
}

// webcomicPages resolves image links against their page, dropping repeats
func webcomicPages(srcs []string, pageURL string) []Page {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	var pages []Page
	seen := map[string]bool{}
	for _, src := range srcs {
		src = strings.TrimSpace(src)
		if src == "" || strings.HasPrefix(src, "data:") {
			continue
		}
		ref, err := url.Parse(src)
		if err != nil {
			continue
		}
		u := base.ResolveReference(ref).String()
		if seen[u] {
			continue
		}
		seen[u] = true
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: u})
	}
	return pages
}

// webcomicAttrs collects an attribute of every match
func webcomicAttrs(doc *goquery.Document, selector, attr string) []string {
	var out []string
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		out = append(out, s.AttrOr(attr, ""))
	})
	return out
}

// webcomicFixed is a listURL for sites with one archive whatever the link
func webcomicFixed(list string) func(*url.URL) (string, error) {
	return func(*url.URL) (string, error) { return list, nil }
}

// webcomicSpecs are the supported webcomics
var webcomicSpecs = []*webcomicSpec{
	webcomicCollectedCurios,
	webcomicDarkLegacy,
	webcomicSabrina,
	webcomicBlastwave,
	webcomicSMBC,
	webcomicBroccoliSoup,
}

// webcomicCollectedCuriosSeries are Jolly Jack's comics on collectedcurios.com
var webcomicCollectedCuriosSeries = []string{"sequentialart.php", "battlebunnies.php", "spiderandscorpion.php"}

// webcomicCollectedCurios: three comics, each a PHP viewer showing one strip
// (?s=N). There's no archive: the viewer's "Jump to number" box (or its Last
// link) holds the newest number, and strips run 1…N.
var webcomicCollectedCurios = &webcomicSpec{
	domains: []string{"collectedcurios.com"},
	title:   "Sequential Art",
	titleOf: func(doc *goquery.Document) string { return doc.Find("title").First().Text() },
	listURL: func(u *url.URL) (string, error) {
		page := strings.ToLower(strings.TrimPrefix(u.Path, "/"))
		for _, s := range webcomicCollectedCuriosSeries {
			if page == s {
				return "https://www.collectedcurios.com/" + s, nil
			}
		}
		return "", errSeriesNotFound(u.String())
	},
	entries: func(doc *goquery.Document, base *url.URL) []webcomicEntry {
		last := webcomicCollectedCuriosLast(doc)
		entries := make([]webcomicEntry, 0, last)
		for i := 1; i <= last; i++ {
			entries = append(entries, webcomicEntry{
				Number: float64(i),
				URL:    base.String() + "?s=" + strconv.Itoa(i),
			})
		}
		return entries
	},
	images: func(doc *goquery.Document) []string {
		return webcomicAttrs(doc, "img.w3-image, img#strip", "src")
	},
}

// webcomicCollectedCuriosLast reads the newest strip number off the viewer:
// its jump box, else its Last link, else one past its Back one link (the
// newest strip of Spider and Scorpion has neither of the others)
func webcomicCollectedCuriosLast(doc *goquery.Document) int {
	if v, err := strconv.Atoi(doc.Find(`input[title="Jump to number"]`).AttrOr("value", "")); err == nil {
		return v
	}
	nav := func(title string) int {
		href := doc.Find(`img[title="`+title+`"]`).Parent().AttrOr("href", "")
		if _, n, ok := strings.Cut(href, "s="); ok {
			if v, err := strconv.Atoi(n); err == nil {
				return v
			}
		}
		return 0
	}
	if v := nav("Last"); v > 0 {
		return v
	}
	if v := nav("Back one"); v > 0 {
		return v + 1
	}
	return 0
}

// webcomicDarkLegacy: the archive lists every comic twice (ascending and
// descending, for its sort toggle), so entries are kept once by number
var webcomicDarkLegacy = &webcomicSpec{
	domains: []string{"darklegacycomics.com"},
	title:   "Dark Legacy Comics",
	listURL: webcomicFixed("https://www.darklegacycomics.com/archive"),
	entries: func(doc *goquery.Document, base *url.URL) []webcomicEntry {
		var entries []webcomicEntry
		seen := map[string]bool{}
		doc.Find(".archive_link").Each(func(_ int, s *goquery.Selection) {
			idx := strings.TrimSpace(s.Find(".index").First().Text())
			n, err := strconv.ParseFloat(idx, 64)
			if err != nil || seen[idx] {
				return
			}
			seen[idx] = true
			entries = append(entries, webcomicEntry{
				Number:   n,
				Title:    s.Find(".name").First().Text(),
				URL:      "https://www.darklegacycomics.com/" + idx,
				Prefixed: true,
			})
		})
		return entries
	},
	images: func(doc *goquery.Document) []string {
		return webcomicAttrs(doc, ".comic > img", "src")
	},
}

// webcomicSabrinaPageRe matches the archive's quarter pages (1997-02.html)
var webcomicSabrinaPageRe = regexp.MustCompile(`^(\d{4})-\d{2}\.html$`)

// webcomicSabrina: a 1996 hand-written site whose archive links one page
// per quarter (a year's "All" in 1996); each page shows thumbnails linking
// the full strips, so a chapter is a quarter and a page a strip
var webcomicSabrina = &webcomicSpec{
	domains: []string{"sabrina-online.com"},
	title:   "Sabrina Online",
	listURL: webcomicFixed("https://www.sabrina-online.com/archive.html"),
	entries: func(doc *goquery.Document, base *url.URL) []webcomicEntry {
		var entries []webcomicEntry
		doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
			href := s.AttrOr("href", "")
			m := webcomicSabrinaPageRe.FindStringSubmatch(href)
			if m == nil {
				return
			}
			ref, _ := url.Parse(href)
			entries = append(entries, webcomicEntry{
				Number: float64(len(entries) + 1),
				Title:  m[1] + " " + s.Text(),
				URL:    base.ResolveReference(ref).String(),
			})
		})
		return entries
	},
	images: func(doc *goquery.Document) []string {
		var out []string
		doc.Find("img").Each(func(_ int, s *goquery.Selection) {
			src := s.AttrOr("src", "")
			if !strings.Contains(src, "strips/") && !strings.Contains(src, "pages/") {
				return
			}
			// the thumbnail links the full-size strip
			if p := s.Parent(); goquery.NodeName(p) == "a" {
				if href := p.AttrOr("href", ""); href != "" {
					src = href
				}
			}
			out = append(out, src)
		})
		return out
	},
}

// webcomicBlastwave: every strip is in the viewer's jump box, valued by the
// site's running number (nro), which the strip titles ("#81 Knife Fight")
// don't share, so the nro numbers the chapters
var webcomicBlastwave = &webcomicSpec{
	domains: []string{"blastwave-comic.com"},
	title:   "Gone with the Blastwave",
	listURL: webcomicFixed("https://www.blastwave-comic.com/index.php"),
	entries: func(doc *goquery.Document, base *url.URL) []webcomicEntry {
		var entries []webcomicEntry
		doc.Find(`select[name="nro"] option`).Each(func(_ int, s *goquery.Selection) {
			v := strings.TrimSpace(s.AttrOr("value", ""))
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return
			}
			entries = append(entries, webcomicEntry{
				Number: n,
				Title:  s.Text(),
				URL:    "https://www.blastwave-comic.com/index.php?p=comic&nro=" + v,
			})
		})
		return entries
	},
	images: func(doc *goquery.Document) []string {
		return webcomicAttrs(doc, `img[src^="comics/"]`, "src")
	},
}

// webcomicSMBC: a ComicControl site whose archive dropdown lists every comic
// ("September 5, 2002 - Title") but is served with a 500 status, so it's
// read through webcomicGetAnyStatus. A comic is its strip plus the hidden
// "after comic" bonus panel.
var webcomicSMBC = &webcomicSpec{
	domains:   []string{"smbc-comics.com"},
	title:     "Saturday Morning Breakfast Cereal",
	listURL:   webcomicFixed("https://www.smbc-comics.com/comic/archive"),
	fetchList: webcomicGetAnyStatus,
	entries: func(doc *goquery.Document, base *url.URL) []webcomicEntry {
		var entries []webcomicEntry
		doc.Find(`option[value*="comic/"]`).Each(func(_ int, s *goquery.Selection) {
			// the values are root-relative without their slash ("comic/x")
			ref, err := url.Parse("/" + strings.TrimPrefix(s.AttrOr("value", ""), "/"))
			if err != nil {
				return
			}
			_, title, _ := strings.Cut(s.Text(), " - ")
			entries = append(entries, webcomicEntry{
				Number: float64(len(entries) + 1),
				Title:  title,
				URL:    base.ResolveReference(ref).String(),
			})
		})
		return entries
	},
	images: func(doc *goquery.Document) []string {
		return append(webcomicAttrs(doc, "img#cc-comic", "src"), webcomicAttrs(doc, "#aftercomic img", "src")...)
	},
}

// webcomicGetAnyStatus reads a page whatever its status. The shared http
// client fails (and retries) any non-200 answer, and SMBC's archive is a
// complete page sent as a 500, so this one request goes through net/http.
func webcomicGetAnyStatus(uri string) (*goquery.Document, error) {
	req, err := stdhttp.NewRequest(stdhttp.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	client := &stdhttp.Client{Timeout: time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 500 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, &http.StatusError{Code: resp.StatusCode}
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

// webcomicBroccoliSoupNumRe reads a Grawlix comic link's number (/comic/12)
var webcomicBroccoliSoupNumRe = regexp.MustCompile(`/comic/(\d+)$`)

// webcomicBroccoliSoup: a Grawlix CMS site; the archive groups pages by
// story arc, and a page may hold several images
var webcomicBroccoliSoup = &webcomicSpec{
	domains: []string{"politeandgood.com"},
	title:   "Broccoli Soup",
	titleOf: func(doc *goquery.Document) string {
		t, _, _ := strings.Cut(doc.Find("title").First().Text(), " - ")
		return t
	},
	listURL: webcomicFixed("https://politeandgood.com/comic/archive"),
	entries: func(doc *goquery.Document, base *url.URL) []webcomicEntry {
		var entries []webcomicEntry
		doc.Find("li.archive-page a[href]").Each(func(_ int, s *goquery.Selection) {
			href := s.AttrOr("href", "")
			m := webcomicBroccoliSoupNumRe.FindStringSubmatch(href)
			if m == nil {
				return
			}
			n, _ := strconv.ParseFloat(m[1], 64)
			ref, _ := url.Parse(href)
			entries = append(entries, webcomicEntry{
				Number:   n,
				Title:    s.Find(".page-title").Text(),
				URL:      base.ResolveReference(ref).String(),
				Prefixed: true,
			})
		})
		return entries
	},
	images: func(doc *goquery.Document) []string {
		return webcomicAttrs(doc, "#comic img", "src")
	},
}
