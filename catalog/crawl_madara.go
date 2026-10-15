// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// madara crawls the Madara WordPress theme family: the /manga/page/N/
// archive, one grid page at a time, until a page is missing (404) or empty.
// Rows are .post-title a (inside .page-item-detail).
type madara struct{}

func init() {
	crawlers["madara"] = madara{}
}

func (madara) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list := site.List
	if list == "" {
		list = strings.TrimSuffix(site.Home, "/") + "/manga/page/{page}/"
	}

	return walkPages(ctx, list, limit, madaraPage)
}

// madaraPage reads one archive page
func madaraPage(doc *goquery.Document, base string) []Entry {
	var entries []Entry
	doc.Find(".post-title a").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		entries = append(entries, Entry{Title: strings.TrimSpace(a.Text()), URL: absURL(base, href)})
	})

	return entries
}
