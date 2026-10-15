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

// pagePause spaces list page requests out (0 in tests)
var pagePause = 300 * time.Millisecond

// httpFetch GETs a page through the project's http package (same UA and
// headers as the grabbers), without the retry loop: a missing page is how a
// crawler learns it reached the end
func httpFetch(ctx context.Context, u string) (*goquery.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	text, err := http.GetText(http.RequestParams{URL: u, Referer: homeOf(u), DisableRetry: true})
	if err != nil {
		return nil, err
	}

	return goquery.NewDocumentFromReader(strings.NewReader(text))
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

// dedupe drops entries without a title or URL and repeats of a URL, keeping
// the first and the order
func dedupe(entries []Entry) []Entry {
	seen := make(map[string]bool, len(entries))
	out := entries[:0:0]
	for _, e := range entries {
		key := urlKey(e.URL)
		if key == "" || strings.TrimSpace(e.Title) == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}

	return out
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
