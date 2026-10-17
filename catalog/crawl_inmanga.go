// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// inmanga is inmanga.com's own crawler: its library is only reachable as
// the result of the consult form, POSTed to List
// (/manga/getMangasConsultResult) inmangaPerPage series at a time, an HTML
// fragment of a.manga-result cards (the series link, its title in an h4).
type inmanga struct{}

func init() {
	crawlers["inmanga"] = inmanga{}
}

const inmangaPerPage = 200

func (inmanga) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	list := site.List
	if list == "" {
		list = strings.TrimSuffix(site.Home, "/") + "/manga/getMangasConsultResult"
	}
	request := func(page int) bodyRequest {
		return bodyRequest{URL: list, Form: url.Values{
			"filter[generes][]":       {"-1"},
			"filter[queryString]":     {""},
			"filter[skip]":            {strconv.Itoa((page - 1) * inmangaPerPage)},
			"filter[take]":            {strconv.Itoa(inmangaPerPage)},
			"filter[sortby]":          {"1"},
			"filter[broadcastStatus]": {"0"},
			"filter[onlyFavorites]":   {"false"},
			"d":                       {""},
		}}
	}

	return walkBodies(ctx, limit, request, inmangaPage)
}

// inmangaPage reads one page of consult results
func inmangaPage(body, base string) ([]Entry, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	var entries []Entry
	doc.Find("a.manga-result").Each(func(_ int, a *goquery.Selection) {
		entries = append(entries, Entry{
			Title: squash(a.Find("h4").First().Text()),
			URL:   absURL(base, a.AttrOr("href", "")),
		})
	})

	return entries, nil
}
