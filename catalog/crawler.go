// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Crawler lists every series of a site family. The crawl runs without a
// browser (the nightly job has none), one page at a time per site.
type Crawler interface {
	// Crawl lists the site's series. limit > 0 stops after that many list
	// pages (smoke checks). It returns what it got plus the first error.
	Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error)
}

// crawlers maps a family name to its crawler; each crawl_*.go registers one
var crawlers = map[string]Crawler{}

// CrawlerFor returns the crawler of the site's family
func CrawlerFor(site SiteInfo) (Crawler, bool) {
	c, ok := crawlers[site.Family]

	return c, ok
}

// fetcher is how crawlers get a list page; tests replace it with fixtures
type fetcher func(ctx context.Context, url string) (*goquery.Document, error)

var fetch fetcher = httpFetch

// textFetcher is how crawlers get a raw body (a JSON API) rather than a
// page; tests replace it with fixtures too
type textFetcher func(ctx context.Context, url string) (string, error)

var fetchText textFetcher = httpFetchText

// pagePause spaces list page requests out (0 in tests)
var pagePause = 300 * time.Millisecond

// httpFetch GETs a page through the project's http package (same UA and
// headers as the grabbers), without the retry loop: a missing page is how a
// crawler learns it reached the end
func httpFetch(ctx context.Context, u string) (*goquery.Document, error) {
	text, err := httpFetchText(ctx, u)
	if err != nil {
		return nil, err
	}

	return goquery.NewDocumentFromReader(strings.NewReader(text))
}

// httpFetchText GETs a body the way httpFetch does, unparsed
func httpFetchText(ctx context.Context, u string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	return http.GetText(http.RequestParams{URL: u, Referer: homeOf(u), DisableRetry: true})
}

// homeOf is the origin of a URL with a trailing slash, the referer the sites
// expect
func homeOf(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return ""
	}

	return p.Scheme + "://" + p.Host + "/"
}

// pause waits pagePause or until ctx is done
func pause(ctx context.Context) error {
	if pagePause == 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(pagePause):
		return nil
	}
}

// absURL resolves href against base; "" when there's nothing to resolve
func absURL(base, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	h, err := url.Parse(href)
	if err != nil {
		return ""
	}
	abs := b.ResolveReference(h)
	if abs.Scheme == "" {
		abs.Scheme = "https"
	}
	abs.Fragment = ""

	return abs.String()
}

// urlKey identifies a series page whatever the www., trailing slash or scheme
func urlKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	key := host + strings.TrimSuffix(u.Path, "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}

	return key
}

// Dedupe drops entries without a title or URL and repeats of a URL, keeping
// the first and the order; a kept entry without a latest chapter takes a
// repeat's
func Dedupe(entries []Entry) []Entry { return dedupe(entries) }

// dedupe drops entries without a title or URL and repeats of a URL, keeping
// the first and the order; a kept entry without a latest chapter takes a
// repeat's
func dedupe(entries []Entry) []Entry {
	at := make(map[string]int, len(entries))
	out := entries[:0:0]
	for _, e := range entries {
		key := urlKey(e.URL)
		if key == "" || strings.TrimSpace(e.Title) == "" {
			continue
		}
		if i, ok := at[key]; ok {
			if out[i].Latest == 0 {
				out[i].Latest = e.Latest
			}
			continue
		}
		at[key] = len(out)
		out = append(out, e)
	}

	return out
}

// Union merges two crawls of a site: the entries of first, then those of
// second not in first (by URL), without repeats. An entry in both keeps
// first's title and URL and the newer crawl's latest chapter, unless that
// crawl gave none (0): a list page that stopped showing the number doesn't
// erase it.
func Union(first, second []Entry, secondIsNewer bool) []Entry {
	return union(first, second, secondIsNewer)
}

func union(first, second []Entry, secondIsNewer bool) []Entry {
	out := dedupe(append(append([]Entry{}, first...), second...))
	if secondIsNewer {
		// dedupe kept first's number: second's replaces it when it has one
		overlayLatest(out, second)
	}

	return out
}

// FillLatest gives the entries without a latest chapter the one an older
// crawl (from) had for the same URL, so a list page that stopped showing the
// number doesn't erase it
func FillLatest(entries, from []Entry) {
	var missing []Entry
	for _, e := range entries {
		if e.Latest == 0 {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return
	}
	known := make(map[string]float64, len(from))
	for _, e := range from {
		if e.Latest > 0 {
			known[urlKey(e.URL)] = e.Latest
		}
	}
	for i := range entries {
		if entries[i].Latest == 0 {
			entries[i].Latest = known[urlKey(entries[i].URL)]
		}
	}
}

// overlayLatest puts from's non-zero latest chapters onto the entries with
// the same URL
func overlayLatest(entries, from []Entry) {
	latest := make(map[string]float64, len(from))
	for _, e := range from {
		if e.Latest > 0 {
			latest[urlKey(e.URL)] = e.Latest
		}
	}
	for i := range entries {
		if n, ok := latest[urlKey(entries[i].URL)]; ok {
			entries[i].Latest = n
		}
	}
}

// pageURL puts the page number into a list URL template; a list without
// {page} is a single page and is returned as is
func pageURL(list string, page int) string {
	return strings.ReplaceAll(list, "{page}", strconv.Itoa(page))
}

// isNotFound reports whether a fetch failed with a 404, the end of a paged list
func isNotFound(err error) bool {
	var se *http.StatusError
	if errors.As(err, &se) {
		return se.Code == 404
	}

	return false
}

// RegisterCrawler adds (or replaces) the crawler of a family
func RegisterCrawler(family string, c Crawler) {
	crawlers[family] = c
}
