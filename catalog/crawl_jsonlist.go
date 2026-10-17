// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// jsonlist is for the sites whose series list is a JSON API: the site's
// List (paged with {page}, else one call) answers an array of series, which
// Items finds in the answer (a dot path, "" for a top-level array). Each
// item's title is at ItemTitle ("title" by default), its series URL at
// ItemURL ("url"), resolved against the list, or built from its fields by
// SeriesURL ("https://www.twmanga.com/comic/{comic_id}"), and its other
// names at ItemAlt. A WordPress REST list fits too: /wp-json/wp/v2/categories
// with "name" and "link" (animeallstar30.com).
type jsonlist struct{}

func init() {
	crawlers["jsonlist"] = jsonlist{}
}

func (jsonlist) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	if site.List == "" {
		return nil, fmt.Errorf("%s: a jsonlist site needs its list", site.Site)
	}
	exclude, err := excludeRe(site)
	if err != nil {
		return nil, err
	}
	titleKey := site.ItemTitle
	if titleKey == "" {
		titleKey = "title"
	}
	urlKey := site.ItemURL
	if urlKey == "" {
		urlKey = "url"
	}
	read := func(ctx context.Context, u string) ([]Entry, error) {
		body, err := fetchText(ctx, u)
		if err != nil {
			return nil, err
		}
		var doc any
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", u, err)
		}
		items, _ := jsonAt(doc, site.Items).([]any)
		var entries []Entry
		for _, item := range items {
			e := Entry{Title: squash(html.UnescapeString(jsonString(jsonAt(item, titleKey))))}
			if site.SeriesURL != "" {
				e.URL = fillFields(site.SeriesURL, item)
			} else {
				e.URL = absURL(u, jsonString(jsonAt(item, urlKey)))
			}
			if site.ItemAlt != "" {
				e.AltNames = jsonStrings(jsonAt(item, site.ItemAlt))
			}
			if exclude != nil && exclude.MatchString(e.Title) {
				continue
			}
			entries = append(entries, e)
		}

		return entries, nil
	}
	entries, err := walkList(ctx, site.List, limit, read)
	if err == nil && len(entries) == 0 {
		err = fmt.Errorf("%s: no series in its list", site.Site)
	}

	return entries, err
}

// walkList is walkPages for any reader: it reads the list's pages from 1
// until one is missing (404), lists nothing or adds nothing new, or limit
// pages were read; a list without {page} is read once
func walkList(ctx context.Context, list string, limit int, read func(ctx context.Context, u string) ([]Entry, error)) ([]Entry, error) {
	var entries []Entry
	for page := 1; limit <= 0 || page <= limit; page++ {
		if page > 1 {
			if err := pause(ctx); err != nil {
				return entries, err
			}
		}
		u := pageURL(list, page)
		found, err := read(ctx, u)
		if err != nil {
			if page > 1 && ctx.Err() == nil && isNotFound(err) {
				return entries, nil
			}

			return entries, err
		}
		before := len(entries)
		entries = dedupe(append(entries, found...))
		if len(entries) == before || !strings.Contains(list, "{page}") {
			return entries, nil
		}
	}

	return entries, nil
}

// excludeRe compiles the site's Exclude expression; nil when it has none
func excludeRe(site SiteInfo) (*regexp.Regexp, error) {
	if site.Exclude == "" {
		return nil, nil
	}
	re, err := regexp.Compile(site.Exclude)
	if err != nil {
		return nil, fmt.Errorf("%s: exclude: %w", site.Site, err)
	}

	return re, nil
}

// jsonAt follows a dot path ("title.rendered") into decoded JSON; nil when
// a step is missing, the value itself for an empty path
func jsonAt(v any, path string) any {
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

// jsonString is a JSON string or number as text, "" for anything else
func jsonString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}

	return ""
}

// jsonStrings is a JSON string, or the strings of a list or of an object
// (by language: {"en": "…", "ko": "…"}, in key order), without blanks or
// repeats
func jsonStrings(v any) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s = squash(html.UnescapeString(s)); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	switch t := v.(type) {
	case string:
		add(t)
	case []any:
		for _, x := range t {
			add(jsonString(x))
		}
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add(jsonString(t[k]))
		}
	}

	return out
}

// fieldRe is a {field} (or {a.b}) placeholder of a SeriesURL template
var fieldRe = regexp.MustCompile(`\{([^{}]+)\}`)

// fillFields puts an item's fields into a URL template; "" when a field is
// missing, so the entry is dropped rather than linking nowhere
func fillFields(template string, item any) string {
	missing := false
	out := fieldRe.ReplaceAllStringFunc(template, func(m string) string {
		v := jsonString(jsonAt(item, m[1:len(m)-1]))
		if v == "" {
			missing = true
		}

		return v
	})
	if missing {
		return ""
	}

	return out
}
