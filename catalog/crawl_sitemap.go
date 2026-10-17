// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

// sitemap is for the sites with no list page a family crawler reads but a
// sitemap, the file listing their pages for search engines: the series are
// the sitemap URLs the site's Series expression picks (a chapter URL maps to
// its series through the expression's first group), titled from their slug.
// In a sitemap index, only the child sitemaps Sitemaps picks are read;
// limit caps how many.
type sitemap struct{}

func init() {
	crawlers["sitemap"] = sitemap{}
}

func (sitemap) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	if site.List == "" || site.Series == "" {
		return nil, fmt.Errorf("%s: a sitemap site needs its list (the sitemap) and series", site.Site)
	}
	seriesRe, err := regexp.Compile(site.Series)
	if err != nil {
		return nil, fmt.Errorf("%s: series: %w", site.Site, err)
	}
	var childRe *regexp.Regexp
	if site.Sitemaps != "" {
		if childRe, err = regexp.Compile(site.Sitemaps); err != nil {
			return nil, fmt.Errorf("%s: sitemaps: %w", site.Site, err)
		}
	}

	root, err := sitemapLocs(ctx, site.List)
	if err != nil {
		return nil, err
	}
	urls := root.urls
	if root.index {
		urls = nil
		read := 0
		for _, child := range root.urls {
			if childRe != nil && !childRe.MatchString(child) {
				continue
			}
			if limit > 0 && read >= limit {
				break
			}
			if read > 0 {
				time.Sleep(pagePause)
			}
			read++
			got, err := sitemapLocs(ctx, child)
			if err != nil {
				if len(urls) == 0 {
					return nil, err
				}
				break
			}
			urls = append(urls, got.urls...)
		}
	}

	var entries []Entry
	seen := map[string]bool{}
	for _, u := range urls {
		m := seriesRe.FindStringSubmatch(u)
		if m == nil {
			continue
		}
		series := u
		if len(m) > 1 && m[1] != "" {
			series = m[1]
			if site.SeriesURL != "" {
				series = strings.ReplaceAll(site.SeriesURL, "{1}", m[1])
			}
		}
		if seen[series] {
			continue
		}
		seen[series] = true
		if title := slugTitle(series); title != "" {
			entries = append(entries, Entry{Title: title, URL: series})
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s: no series in its sitemap", site.Site)
	}

	return entries, nil
}

// sitemapList is one sitemap file's URLs, and whether it's an index of
// other sitemaps
type sitemapList struct {
	urls  []string
	index bool
}

// sitemapLocs reads a sitemap's <loc> URLs, unescaped
func sitemapLocs(ctx context.Context, u string) (sitemapList, error) {
	doc, err := fetch(ctx, u)
	if err != nil {
		return sitemapList{}, err
	}
	var list sitemapList
	list.index = doc.Find("sitemapindex").Length() > 0
	doc.Find("loc").Each(func(_ int, s *goquery.Selection) {
		if v := strings.TrimSpace(html.UnescapeString(s.Text())); v != "" {
			list.urls = append(list.urls, v)
		}
	})

	return list, nil
}

// slugIDRe matches the id a slug carries before its words ("264101--tonari",
// a hex "685152ec702284f834178042-title") or after them ("title-123456"):
// short numbers are part of the title ("17-sai-c", "one-piece-2025")
var slugIDRe = regexp.MustCompile(`^(?:\d{5,}|[0-9a-f]{16,})-+|-\d{5,}$`)

// slugTitle makes a title of a series URL's last path segment:
// "sekiro-manga" → "Sekiro Manga"
func slugTitle(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	slug := path.Base(strings.TrimRight(parsed.Path, "/"))
	if unescaped, err := url.PathUnescape(slug); err == nil {
		slug = unescaped
	}
	slug = strings.TrimSuffix(strings.TrimSuffix(slug, ".html"), ".php")
	slug = slugIDRe.ReplaceAllString(slug, "")
	words := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' || r == '+' || unicode.IsSpace(r) })
	if len(words) == 0 || slug == "/" || slug == "." {
		return ""
	}
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}

	return strings.Join(words, " ")
}
