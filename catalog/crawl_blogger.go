// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// blogger is for manga blogs on Blogger, whose series are the blog's
// labels: the posts feed (List, by default /feeds/posts/summary?alt=json
// with no posts) names every label in its categories. The downloader reads a
// Blogger series as the blog's search for its title, so a label's URL is
// /search?q={label} (SeriesURL changes it, {1} being the escaped label).
// Labels that aren't series (genres, "Anime") are left out by Exclude.
type blogger struct{}

func init() {
	crawlers["blogger"] = blogger{}
}

// bloggerFeed is the part of a Blogger JSON feed the crawler reads
type bloggerFeed struct {
	Feed struct {
		Category []struct {
			Term string `json:"term"`
		} `json:"category"`
	} `json:"feed"`
}

func (blogger) Crawl(ctx context.Context, site SiteInfo, _ int) ([]Entry, error) {
	home := strings.TrimSuffix(site.Home, "/")
	list := site.List
	if list == "" {
		list = home + "/feeds/posts/summary?alt=json&max-results=0"
	}
	exclude, err := excludeRe(site)
	if err != nil {
		return nil, err
	}
	body, err := fetchText(ctx, list)
	if err != nil {
		return nil, err
	}
	var feed bloggerFeed
	if err := json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, fmt.Errorf("%s: %w", list, err)
	}
	template := site.SeriesURL
	if template == "" {
		template = home + "/search?q={1}"
	}
	var entries []Entry
	for _, c := range feed.Feed.Category {
		label := squash(c.Term)
		if label == "" || (exclude != nil && exclude.MatchString(label)) {
			continue
		}
		u := strings.ReplaceAll(template, "{1}", url.QueryEscape(label))
		entries = append(entries, Entry{Title: label, URL: u})
	}
	entries = dedupe(entries)
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s: no labels in its feed", site.Site)
	}

	return entries, nil
}
