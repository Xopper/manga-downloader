// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// jsonapi is for the sites whose series come from a paged JSON API: a
// WordPress REST list (/wp-json/wp/v2/categories?per_page=100&page={page}),
// a HeanCMS /query, … The site's List is the API URL ({page} is the page
// number), Items where the array of series is in the answer, TitleKey an
// item's title and SeriesURL the series URL made of the item's fields
// ("{link}", "https://luacomic.org/series/{series_slug}"). An item missing
// a field is left out.
type jsonapi struct{}

func init() {
	crawlers["jsonapi"] = jsonapi{}
}

func (jsonapi) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	if site.List == "" || site.TitleKey == "" || site.SeriesURL == "" {
		return nil, fmt.Errorf("%s: a jsonapi site needs its list, titleKey and seriesURL", site.Site)
	}
	if !strings.Contains(site.List, "{page}") {
		limit = 1
	}
	request := func(page int) bodyRequest {
		return bodyRequest{URL: pageURL(site.List, page), Referer: strings.TrimSuffix(site.Home, "/") + "/"}
	}

	return walkBodies(ctx, limit, request, func(body, base string) ([]Entry, error) {
		return jsonapiPage(body, base, site)
	})
}

// jsonapiPage reads one API answer
func jsonapiPage(body, base string, site SiteInfo) ([]Entry, error) {
	var doc any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", base, err)
	}
	items, ok := jsonapiAt(doc, site.Items).([]any)
	if !ok {
		return nil, fmt.Errorf("%s: no %q array", base, site.Items)
	}
	var entries []Entry
	for _, item := range items {
		title := squash(html.UnescapeString(jsonapiJsonString(jsonapiAt(item, site.TitleKey))))
		link, ok := jsonapiFill(site.SeriesURL, item)
		if title == "" || !ok {
			continue
		}
		if u := absURL(base, link); u != "" {
			entries = append(entries, Entry{Title: title, URL: u})
		}
	}

	return entries, nil
}

// jsonapiAt follows a dotted path ("title.rendered") into decoded JSON; the
// value itself for an empty path, nil when a key is missing
func jsonapiAt(v any, path string) any {
	if path == "" {
		return v
	}
	for _, key := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[key]
	}

	return v
}

// jsonapiJsonString is a JSON string or number as text, "" for anything else
func jsonapiJsonString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}

	return ""
}

var fieldRef = regexp.MustCompile(`\{([^{}]+)\}`)

// jsonapiFill replaces each {path} of a template with the item's value there;
// false when one is missing or empty
func jsonapiFill(template string, item any) (string, bool) {
	ok := true
	out := fieldRef.ReplaceAllStringFunc(template, func(ref string) string {
		v := jsonapiJsonString(jsonapiAt(item, ref[1:len(ref)-1]))
		if v == "" {
			ok = false
		}

		return v
	})

	return out, ok
}
