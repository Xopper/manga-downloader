// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

// SiteInfo is one row of the embedded site table (sites.json): every site the
// README lists, with the crawler family and list page its catalog comes from
type SiteInfo struct {
	// Site is the domain without www, the key of everything (file names, --site)
	Site string `json:"site"`
	Home string `json:"home"`
	// Aliases are former domains of the site, so a URL on one still matches
	Aliases []string `json:"aliases,omitempty"`
	// Family names the crawler: themesia, madara, single, gigaviewer,
	// sitemap, jsonlist, htmllist, blogger, bloggerposts, links, jsonapi,
	// inmanga, or a platform with no crawler yet
	// (custom, nextjs, astro, nuxt, foolslide, api)
	Family string `json:"family"`
	// List is the family's list page; {page} is the page number. For the
	// sitemap family it's the site's sitemap (or sitemap index); for the
	// gigaviewer family it can be several pages, parted by spaces.
	List string `json:"list,omitempty"`
	// Sitemaps picks, in a sitemap index, the child sitemaps that list series
	// (a regular expression on their URL; all of them when empty)
	Sitemaps string `json:"sitemaps,omitempty"`
	// Series picks the sitemap URLs that are series (a regular expression on
	// the URL); its first group, when it has one, is the series URL a chapter
	// URL belongs to. The links family keeps only the links it matches.
	Series string `json:"series,omitempty"`
	// SeriesURL builds the series URL from Series' first group when it isn't
	// a prefix of the matched URL ("https://www.utoon.us/en/manga/{1}.html").
	// For the jsonlist family it builds it from the item's fields instead
	// ("https://www.twmanga.com/comic/{comic_id}"), and for the blogger
	// family from the label ({1}, query-escaped).
	// For the jsonapi family it's built from an item's fields
	// ("https://luacomic.org/series/{series_slug}", "{link}").
	SeriesURL string `json:"seriesURL,omitempty"`
	// Items picks the series in a list page: for the jsonlist and jsonapi
	// families the dot path of the array of series in the JSON ("" is a
	// top-level array), for
	// the htmllist family a CSS selector of the series links (or elements
	// holding the link)
	Items string `json:"items,omitempty"`
	// ItemTitle is where an item's title is: jsonlist, its dot path in the
	// item (default "title"); htmllist, a selector inside the item (default
	// the item's own text, else its title attribute)
	ItemTitle string `json:"itemTitle,omitempty"`
	// ItemURL is where an item's series URL is: jsonlist, its dot path in the
	// item (default "url"); htmllist, the attribute holding it (default
	// href). Relative URLs are resolved against the list page.
	ItemURL string `json:"itemURL,omitempty"`
	// ItemAlt is the dot path of an item's other names (jsonlist), a string
	// or a list of strings
	ItemAlt string `json:"itemAlt,omitempty"`
	// Exclude drops the entries whose title it matches (a regular
	// expression): a blog's genre labels, a category that isn't a series
	Exclude string `json:"exclude,omitempty"`
	Lang    string `json:"lang"`
	// Crawl is false for sites with no crawler, API sites and browser-gated sites
	Crawl bool `json:"crawl"`
	// Browser marks the sites the README marks with *
	Browser bool `json:"browser,omitempty"`
	// Label is the README's parenthesised note (name, language, former domain)
	Label string `json:"label,omitempty"`
	// Note says why a site isn't crawled yet
	Note string `json:"note,omitempty"`

	// Links picks the series links on a list page (a CSS selector): the links
	// family's rows, a gigaviewer site's series links when they aren't all its
	// episode links
	Links string `json:"links,omitempty"`
	// TitleKey is, for the jsonapi family, an item's title (dotted keys,
	// "title", "title.rendered")
	TitleKey string `json:"titleKey,omitempty"`
}

//go:embed sites.json
var sitesJSON []byte

var (
	sitesOnce sync.Once
	sites     []SiteInfo
	byHost    map[string]int
)

func loadSites() {
	if err := json.Unmarshal(sitesJSON, &sites); err != nil {
		panic("catalog: sites.json: " + err.Error())
	}
	byHost = make(map[string]int, len(sites)*2)
	for i, s := range sites {
		byHost[s.Site] = i
		for _, a := range s.Aliases {
			byHost[a] = i
		}
	}
}

// Sites returns the site table, in file order (alphabetical by site)
func Sites() []SiteInfo {
	sitesOnce.Do(loadSites)
	out := make([]SiteInfo, len(sites))
	copy(out, sites)

	return out
}

// SiteByHost finds the site of a host name (www. and case ignored), by its
// current domain or a former one
func SiteByHost(host string) (SiteInfo, bool) {
	sitesOnce.Do(loadSites)
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
	i, ok := byHost[host]
	if !ok {
		return SiteInfo{}, false
	}

	return sites[i], true
}
