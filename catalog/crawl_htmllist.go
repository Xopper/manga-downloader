// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"fmt"
	"regexp"

	"github.com/PuerkitoBio/goquery"
)

// htmllist is for the sites with a plain HTML list of their series that no
// theme family covers: the site's List (paged with {page}, else one page)
// and Items, a CSS selector of the series links. An item's URL is its
// ItemURL attribute (href by default; a <select>'s options carry it in
// value), its title the text of ItemTitle inside it, else its own text, else
// its title attribute. Items that aren't links themselves are searched for
// their first link. MyMangaReaderCMS sites list everything at
// /changeMangaList?type=text (ul.manga-list a); FMReader sites page through
// /manga-list.html?page={page}.
type htmllist struct{}

func init() {
	crawlers["htmllist"] = htmllist{}
}

func (htmllist) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	if site.List == "" || site.Items == "" {
		return nil, fmt.Errorf("%s: an htmllist site needs its list and items", site.Site)
	}
	exclude, err := excludeRe(site)
	if err != nil {
		return nil, err
	}
	read := func(ctx context.Context, u string) ([]Entry, error) {
		doc, err := fetch(ctx, u)
		if err != nil {
			return nil, err
		}

		return htmlListPage(doc, u, site, exclude), nil
	}
	entries, err := walkList(ctx, site.List, limit, read)
	if err == nil && len(entries) == 0 {
		err = fmt.Errorf("%s: no series in its list", site.Site)
	}

	return entries, err
}

// htmlListPage reads the site's items off one list page
func htmlListPage(doc *goquery.Document, base string, site SiteInfo, exclude *regexp.Regexp) []Entry {
	attr := site.ItemURL
	if attr == "" {
		attr = "href"
	}
	var entries []Entry
	doc.Find(site.Items).Each(func(_ int, item *goquery.Selection) {
		link, linkAttr := item, attr
		if _, ok := item.Attr(attr); !ok {
			link, linkAttr = item.Find("a[href]").First(), "href"
		}
		var title string
		if site.ItemTitle != "" {
			title = squash(item.Find(site.ItemTitle).First().Text())
		}
		if title == "" {
			title = squash(item.Text())
		}
		if title == "" {
			title = squash(item.AttrOr("title", ""))
		}
		if exclude != nil && exclude.MatchString(title) {
			return
		}
		entries = append(entries, Entry{Title: title, URL: absURL(base, link.AttrOr(linkAttr, ""))})
	})

	return entries
}
