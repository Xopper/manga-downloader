// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package catalog lists every series a site has, so a title can be found
// without knowing its URL, and searches those lists offline.
package catalog

import "time"

// Entry is one series a site lists
type Entry struct {
	Title string `json:"title"`
	// URL is the series page, what the downloader's Load/IdentifySite takes
	URL string `json:"url"`
	// AltNames are the other titles the site lists (romaji, native, …)
	AltNames []string `json:"alt,omitempty"`
}

// SiteCatalog is one site's list, the content of catalog/<site>.json
type SiteCatalog struct {
	Site     string    `json:"site"`
	Home     string    `json:"home"`
	Language string    `json:"lang"`
	Crawled  time.Time `json:"crawled"`
	// Source is the Tachiyomi source the list came through, when a Suwayomi
	// crawl built or extended it
	Source  *SourceInfo `json:"source,omitempty"`
	Entries []Entry     `json:"entries"`
}

// SourceInfo describes the Tachiyomi source a catalog came through
type SourceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
	NSFW bool   `json:"nsfw"`
	// Pages is how many listing pages the crawl read; Complete is false when
	// a listing was cut at the page cap
	Pages    int  `json:"pages"`
	Complete bool `json:"complete"`
}
