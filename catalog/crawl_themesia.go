// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// themesia crawls the MangaStream/themesia WordPress theme family. Its
// /manga/list-mode/ page lists every series at once (.soralist a.series);
// sites without it get the /manga/?page=N grid walked page by page
// (.listupd .bsx a[title]).
type themesia struct{}

func init() {
	crawlers["themesia"] = themesia{}
}

func (themesia) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if site.List != "" {
		doc, err := fetch(ctx, site.List)
		if err == nil {
			if entries := themesiaListMode(doc, site.List); len(entries) > 0 {
				return entries, nil
			}
		} else if ctx.Err() != nil {
			return nil, err
		}
	}

	return walkPages(ctx, strings.TrimSuffix(site.Home, "/")+"/manga/?page={page}", limit, themesiaGrid)
}

// themesiaListMode reads the one-page A–Z list
func themesiaListMode(doc *goquery.Document, base string) []Entry {
	var entries []Entry
	doc.Find(".soralist a.series").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		entries = append(entries, Entry{Title: strings.TrimSpace(a.Text()), URL: absURL(base, href)})
	})

	return dedupe(entries)
}

// themesiaGrid reads one page of the series grid
func themesiaGrid(doc *goquery.Document, base string) []Entry {
	var entries []Entry
	doc.Find(".listupd .bsx a").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		title, _ := a.Attr("title")
		if title == "" {
			title = a.Find(".tt").Text()
		}
		entries = append(entries, Entry{Title: strings.TrimSpace(title), URL: absURL(base, href)})
	})

	return entries
}

// pageReader turns one list page into its entries (not yet deduped)
type pageReader func(doc *goquery.Document, base string) []Entry

// walkPages fetches list pages from 1 until one is missing (404), lists
// nothing, or adds no series not seen before (a site that ignores the page
// number), or limit pages were read. It returns what it collected, with the
// error that ended the walk if a page after the first failed; a failure on
// page 1 is returned on its own.
func walkPages(ctx context.Context, list string, limit int, readPage pageReader) ([]Entry, error) {
	var entries []Entry
	for page := 1; limit <= 0 || page <= limit; page++ {
		if page > 1 {
			if err := pause(ctx); err != nil {
				return entries, err
			}
		}
		if err := ctx.Err(); err != nil {
			return entries, err
		}
		u := pageURL(list, page)
		doc, err := fetch(ctx, u)
		if err != nil {
			if page == 1 || ctx.Err() != nil {
				return entries, err
			}
			if isNotFound(err) {
				return entries, nil
			}

			return entries, err
		}
		found := readPage(doc, u)
		if len(found) == 0 {
			return entries, nil
		}
		before := len(entries)
		entries = dedupe(append(entries, found...))
		if len(entries) == before {
			return entries, nil
		}
	}

	return entries, nil
}
