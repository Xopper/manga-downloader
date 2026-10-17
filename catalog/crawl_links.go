// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// links is for the sites whose list page is plain HTML no family reads: the
// site's Links selector picks the series links on its list ({page} is the
// page number; without it the list is one page), each titled by its title
// attribute, else its text. Series, when set, keeps only the links whose URL
// it matches.
type links struct{}

func init() {
	crawlers["links"] = links{}
}

func (links) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	if site.List == "" || site.Links == "" {
		return nil, fmt.Errorf("%s: a links site needs its list and links", site.Site)
	}
	var keep *regexp.Regexp
	if site.Series != "" {
		var err error
		if keep, err = regexp.Compile(site.Series); err != nil {
			return nil, fmt.Errorf("%s: series: %w", site.Site, err)
		}
	}
	readPage := func(doc *goquery.Document, base string) []Entry {
		var entries []Entry
		doc.Find(site.Links).Each(func(_ int, a *goquery.Selection) {
			u := absURL(base, a.AttrOr("href", ""))
			if u == "" || (keep != nil && !keep.MatchString(u)) {
				return
			}
			title := squash(a.AttrOr("title", ""))
			if title == "" {
				title = squash(a.Text())
			}
			entries = append(entries, Entry{Title: title, URL: u})
		})

		return entries
	}
	if !strings.Contains(site.List, "{page}") {
		limit = 1
	}

	return walkPages(ctx, site.List, limit, readPage)
}
