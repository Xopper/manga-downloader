// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

// SuwayomiOptions drive CrawlSuwayomi
type SuwayomiOptions struct {
	// Server is the Suwayomi-Server's URL (http://127.0.0.1:4567)
	Server string
	// Langs are the source languages to crawl; "all" (multi-language
	// sources) is always included
	Langs []string
	// Out is the catalog dir the files are merged into
	Out string
	// MaxPages caps every listing walk (popular, latest, search); 300 when 0
	MaxPages int
	// Parallel is how many sources are walked at once; 4 when 0
	Parallel int
	// Shard/Shards keep every source whose index (in id order) is Shard
	// modulo Shards; 0/0 keeps all
	Shard, Shards int
	// Sources keeps only these sources (by name or id); empty keeps all
	Sources []string
	// Install installs the extensions of the languages that aren't yet
	Install bool
	// Timeout stops starting new sources once it has elapsed; 0 is none
	Timeout time.Duration
	// Pause is the wait between pages; 200ms when 0, set to a negative value
	// for none
	Pause time.Duration
}

// SourceRow is one crawled source in sources.json
type SourceRow struct {
	Site     string `json:"site"`
	Name     string `json:"name"`
	Lang     string `json:"lang"`
	NSFW     bool   `json:"nsfw"`
	SourceID string `json:"sourceId"`
	Entries  int    `json:"entries"`
	Complete bool   `json:"complete"`
	Failed   bool   `json:"failed,omitempty"`
	// Downloadable is whether the site is in the site table: the downloader
	// has a grabber for it
	Downloadable bool `json:"downloadable"`
}

// SuwayomiReport is what a crawl did
type SuwayomiReport struct {
	Sources []SourceRow
	// Written lists the sites whose file was written
	Written []string
	// Failed counts the sources that gave nothing, or were skipped by the
	// timeout
	Failed int
}

// suwayomi is the slice of the server's REST API the crawl uses
type suwayomi struct {
	base string
}

type suwayomiExtension struct {
	PkgName   string `json:"pkgName"`
	Lang      string `json:"lang"`
	Installed bool   `json:"installed"`
}

type suwayomiSource struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Lang           string `json:"lang"`
	BaseURL        string `json:"baseUrl"`
	SupportsLatest bool   `json:"supportsLatest"`
	NSFW           bool   `json:"isNsfw"`
}

type suwayomiPageResp struct {
	MangaList []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	} `json:"mangaList"`
	HasNextPage bool `json:"hasNextPage"`
}

func (s suwayomi) get(ctx context.Context, path string, into any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text, err := http.GetText(http.RequestParams{URL: s.base + path, DisableRetry: true, Accept: "application/json"})
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(text), into)
}

func (s suwayomi) extensions(ctx context.Context) ([]suwayomiExtension, error) {
	var out []suwayomiExtension
	err := s.get(ctx, "/api/v1/extension/list", &out)

	return out, err
}

func (s suwayomi) install(ctx context.Context, pkg string) error {
	var ignored any
	err := s.get(ctx, "/api/v1/extension/install/"+url.PathEscape(pkg), &ignored)
	if err != nil && strings.Contains(err.Error(), "invalid character") {
		// the install answers plain text; that's fine
		return nil
	}

	return err
}

func (s suwayomi) sources(ctx context.Context) ([]suwayomiSource, error) {
	var out []suwayomiSource
	err := s.get(ctx, "/api/v1/source/list", &out)

	return out, err
}

func (s suwayomi) page(ctx context.Context, id, listing string, n int) (*suwayomiPageResp, error) {
	var path string
	switch listing {
	case "search":
		path = fmt.Sprintf("/api/v1/source/%s/search?searchTerm=&pageNum=%d", id, n)
	default:
		path = fmt.Sprintf("/api/v1/source/%s/%s/%d", id, listing, n)
	}
	var out suwayomiPageResp
	if err := s.get(ctx, path, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// wantLang reports whether a source or extension language is crawled
func wantLang(langs []string, lang string) bool {
	if lang == "all" {
		return true
	}
	for _, l := range langs {
		if l == lang {
			return true
		}
	}

	return false
}

// siteOfBase is the catalog site key of a source's base URL: its host
// without www. "" when there's no usable host.
func siteOfBase(base string) string {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if !siteNameRe.MatchString(host) || strings.Contains(host, "..") {
		return ""
	}

	return host
}

// CrawlSuwayomi lists every series of the server's sources in the
// languages wanted and merges them into the catalog files of Out, one per
// site, writing a line per source to w and the sources' rows to
// sources.json (sources-<shard>.json with shards). The error is only for a
// bad option or a server that doesn't answer; a source failing is a row.
func CrawlSuwayomi(ctx context.Context, w io.Writer, o SuwayomiOptions) (*SuwayomiReport, error) {
	if o.Server == "" {
		return nil, fmt.Errorf("no server URL")
	}
	if o.Shards < 0 || o.Shard < 0 || (o.Shards > 0 && o.Shard >= o.Shards) {
		return nil, fmt.Errorf("bad shard %d/%d", o.Shard, o.Shards)
	}
	if o.MaxPages <= 0 {
		o.MaxPages = 300
	}
	if o.Parallel <= 0 {
		o.Parallel = 4
	}
	switch {
	case o.Pause == 0:
		o.Pause = 200 * time.Millisecond
	case o.Pause < 0:
		o.Pause = 0
	}
	langs := o.Langs
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	api := suwayomi{base: strings.TrimSuffix(o.Server, "/")}
	started := time.Now()

	if o.Install {
		exts, err := api.extensions(ctx)
		if err != nil {
			return nil, fmt.Errorf("extension list: %w", err)
		}
		for _, e := range exts {
			if e.Installed || !wantLang(langs, e.Lang) {
				continue
			}
			if err := api.install(ctx, e.PkgName); err != nil {
				fmt.Fprintf(w, "FAIL install %s: %s\n", e.PkgName, err)
			}
		}
	}

	all, err := api.sources(ctx)
	if err != nil {
		return nil, fmt.Errorf("source list: %w", err)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	var sources []suwayomiSource
	for i, s := range all {
		if s.ID == "0" || !wantLang(langs, s.Lang) {
			continue
		}
		if len(o.Sources) > 0 && !nameOrID(o.Sources, s) {
			continue
		}
		if o.Shards > 0 && i%o.Shards != o.Shard {
			continue
		}
		if siteOfBase(s.BaseURL) == "" {
			fmt.Fprintf(w, "%s  FAIL: no usable base URL (%q)\n", s.Name, s.BaseURL)
			continue
		}
		sources = append(sources, s)
	}

	store := NewStore(o.Out)
	for _, warn := range store.Load() {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}

	rep := &SuwayomiReport{}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, o.Parallel)
		// writes serialises the merge of one site, so two sources on the
		// same host don't lose each other's entries
		writes sync.Mutex
	)
	for i, src := range sources {
		site := siteOfBase(src.BaseURL)
		row := SourceRow{Site: site, Name: src.Name, Lang: src.Lang, NSFW: src.NSFW, SourceID: src.ID, Complete: true}
		if _, known := SiteByHost(site); known {
			row.Downloadable = true
		}
		// the first source always runs; the timeout only stops later ones
		if i > 0 && o.Timeout > 0 && time.Since(started) > o.Timeout {
			row.Failed = true
			mu.Lock()
			rep.Sources = append(rep.Sources, row)
			rep.Failed++
			mu.Unlock()
			fmt.Fprintf(w, "%s  skipped: timeout\n", site)
			continue
		}
		wg.Add(1)
		go func(src suwayomiSource, site string, row SourceRow) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			entries, pages, complete := walkSource(ctx, api, src, o)
			row.Complete = complete
			if len(entries) == 0 {
				row.Failed = true
				mu.Lock()
				rep.Sources = append(rep.Sources, row)
				rep.Failed++
				mu.Unlock()
				fmt.Fprintf(w, "%s  FAIL: no entries (%s)\n", site, src.Name)
				return
			}
			writes.Lock()
			n, err := mergeSite(store, site, src, entries, pages, complete)
			writes.Unlock()
			if err != nil {
				row.Failed = true
				mu.Lock()
				rep.Sources = append(rep.Sources, row)
				rep.Failed++
				mu.Unlock()
				fmt.Fprintf(w, "%s  FAIL: %s\n", site, err)
				return
			}
			row.Entries = n
			mu.Lock()
			rep.Sources = append(rep.Sources, row)
			rep.Written = append(rep.Written, site)
			mu.Unlock()
			note := ""
			if !complete {
				note = " (cut at the page cap)"
			}
			fmt.Fprintf(w, "%s  %d entries from %s, %d pages%s\n", site, n, src.Name, pages, note)
		}(src, site, row)
	}
	wg.Wait()

	sort.Slice(rep.Sources, func(i, j int) bool { return rep.Sources[i].SourceID < rep.Sources[j].SourceID })
	sort.Strings(rep.Written)
	name := "sources.json"
	if o.Shards > 0 {
		name = fmt.Sprintf("sources-%d.json", o.Shard)
	}
	b, _ := json.MarshalIndent(rep.Sources, "", " ")
	if err := writeFileAtomic(filepath.Join(o.Out, name), b); err != nil {
		return rep, err
	}

	return rep, nil
}

// nameOrID reports whether a source is one of the names or ids asked for
func nameOrID(want []string, s suwayomiSource) bool {
	for _, w := range want {
		if strings.EqualFold(w, s.Name) || w == s.ID {
			return true
		}
	}

	return false
}

// walkSource unions the source's popular, latest and empty-search listings.
// It returns the entries, how many pages were read, and whether no listing
// was cut at the page cap.
func walkSource(ctx context.Context, api suwayomi, src suwayomiSource, o SuwayomiOptions) (entries []Entry, pages int, complete bool) {
	complete = true
	listings := []string{"popular"}
	if src.SupportsLatest {
		listings = append(listings, "latest")
	}
	listings = append(listings, "search")
	seen := map[string]bool{}
	for _, listing := range listings {
		errs := 0
		for n := 1; ; n++ {
			if n > o.MaxPages {
				complete = false
				break
			}
			if n > 1 && o.Pause > 0 {
				select {
				case <-ctx.Done():
					return entries, pages, false
				case <-time.After(o.Pause):
				}
			}
			resp, err := api.page(ctx, src.ID, listing, n)
			if err != nil {
				if ctx.Err() != nil {
					return entries, pages, false
				}
				errs++
				if errs >= 3 {
					complete = false
					break
				}
				n--
				continue
			}
			errs = 0
			pages++
			added := 0
			for _, m := range resp.MangaList {
				u := joinBase(src.BaseURL, m.URL)
				key := urlKey(u)
				if key == "" || strings.TrimSpace(m.Title) == "" || seen[key] {
					continue
				}
				seen[key] = true
				entries = append(entries, Entry{Title: strings.TrimSpace(m.Title), URL: u})
				added++
			}
			if !resp.HasNextPage || added == 0 {
				break
			}
		}
	}

	return entries, pages, complete
}

// joinBase builds a series URL the way Tachiyomi's HttpSource does: the
// source's base URL plus the manga's path, by plain concatenation (a base
// with a path keeps it); an absolute path is kept as is
func joinBase(base, path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return strings.TrimSuffix(strings.TrimSpace(base), "/") + path
}

// mergeSite unions the entries into the site's file (the existing ones
// first) and returns the file's new size
func mergeSite(store *Store, site string, src suwayomiSource, entries []Entry, pages int, complete bool) (int, error) {
	c := &SiteCatalog{Site: site, Home: strings.TrimSuffix(strings.TrimSpace(src.BaseURL), "/"), Language: src.Lang}
	if old, ok := store.Catalog(site); ok {
		cp := *old
		c = &cp
		if c.Home == "" {
			c.Home = strings.TrimSuffix(strings.TrimSpace(src.BaseURL), "/")
		}
		if c.Language == "" {
			c.Language = src.Lang
		}
	}
	c.Entries = dedupe(append(append([]Entry{}, c.Entries...), entries...))
	sort.SliceStable(c.Entries, func(i, j int) bool { return Fold(c.Entries[i].Title) < Fold(c.Entries[j].Title) })
	c.Crawled = time.Now().UTC()
	c.Source = &SourceInfo{ID: src.ID, Name: src.Name, Lang: src.Lang, NSFW: src.NSFW, Pages: pages, Complete: complete}
	if err := store.Write(c); err != nil {
		return 0, err
	}

	return len(c.Entries), nil
}
