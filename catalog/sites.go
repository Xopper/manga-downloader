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
	// Family names the crawler: themesia, madara, single, or a platform with no
	// crawler yet (custom, nextjs, astro, nuxt, foolslide, blogger, api)
	Family string `json:"family"`
	// List is the family's list page; {page} is the page number
	List string `json:"list,omitempty"`
	Lang string `json:"lang"`
	// Crawl is false for sites with no crawler, API sites and browser-gated sites
	Crawl bool `json:"crawl"`
	// Browser marks the sites the README marks with *
	Browser bool `json:"browser,omitempty"`
	// Label is the README's parenthesised note (name, language, former domain)
	Label string `json:"label,omitempty"`
	// Note says why a site isn't crawled yet
	Note string `json:"note,omitempty"`
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
