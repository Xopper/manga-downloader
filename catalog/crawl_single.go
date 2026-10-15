// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"strings"
)

// single is for sites that host one series (chainsawman, heavenly-delusion,
// …): the catalog is the home page. The title is the README's label when
// there is one, else the page title up to its first separator; a site that
// doesn't answer still gets its entry, named after the domain.
type single struct{}

func init() {
	crawlers["single"] = single{}
}

func (single) Crawl(ctx context.Context, site SiteInfo, _ int) ([]Entry, error) {
	home := strings.TrimSuffix(site.Home, "/") + "/"
	entry := Entry{Title: site.Label, URL: home}
	if entry.Title != "" {
		return []Entry{entry}, nil
	}
	entry.Title = site.Site
	doc, err := fetch(ctx, home)
	if err != nil {
		return []Entry{entry}, err
	}
	if title := pageTitle(doc.Find("title").First().Text()); title != "" {
		entry.Title = title
	}

	return []Entry{entry}, nil
}

// pageTitle cuts a <title> at its first " - ", " | " or " – " and trims it
func pageTitle(title string) string {
	for _, sep := range []string{" - ", " | ", " – ", " — "} {
		if i := strings.Index(title, sep); i >= 0 {
			title = title[:i]
		}
	}

	return strings.TrimSpace(title)
}
