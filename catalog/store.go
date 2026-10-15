// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store is the catalogs on disk (one <site>.json per site under Dir) held in
// memory for searching
type Store struct {
	Dir string

	mu       sync.RWMutex
	catalogs map[string]*SiteCatalog
	// sources is sources.json: the Tachiyomi sources the catalogs came
	// through, nil without one
	sources []SourceRow
	// entries is every catalog's entries with their folded names, in site
	// order, what Search scans
	entries []indexed
}

// indexed is an entry ready to match
type indexed struct {
	site  string
	entry Entry
	title string
	alts  []string
}

// NewStore makes a store over dir; nothing is read until Load
func NewStore(dir string) *Store {
	return &Store{Dir: dir, catalogs: map[string]*SiteCatalog{}}
}

// Load reads every <site>.json in Dir (index.json excluded). A missing dir
// is an empty store; a file that doesn't parse is skipped and reported.
func (s *Store) Load() []error {
	names, err := catalogFiles(s.Dir)
	if err != nil {
		return []error{err}
	}
	catalogs := make(map[string]*SiteCatalog, len(names))
	var warnings []error
	for _, name := range names {
		b, err := os.ReadFile(name)
		if err != nil {
			warnings = append(warnings, err)
			continue
		}
		if b, err = maybeGunzip(filepath.Base(name), b); err != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", filepath.Base(name), err))
			continue
		}
		var c SiteCatalog
		if err := json.Unmarshal(b, &c); err != nil || c.Site == "" {
			warnings = append(warnings, fmt.Errorf("%s: not a catalog file: %v", filepath.Base(name), err))
			continue
		}
		catalogs[c.Site] = &c
	}

	sources, err := readRows(filepath.Join(s.Dir, sourcesFile))
	if err != nil && !os.IsNotExist(err) {
		warnings = append(warnings, fmt.Errorf("%s: %w", sourcesFile, err))
	}

	s.mu.Lock()
	s.catalogs = catalogs
	s.sources = sources
	s.reindex()
	s.mu.Unlock()

	return warnings
}

// reindex rebuilds entries from catalogs; the caller holds mu
func (s *Store) reindex() {
	sites := make([]string, 0, len(s.catalogs))
	for site := range s.catalogs {
		sites = append(sites, site)
	}
	sort.Strings(sites)
	s.entries = s.entries[:0]
	for _, site := range sites {
		for _, e := range s.catalogs[site].Entries {
			ix := indexed{site: site, entry: e, title: Fold(e.Title)}
			for _, alt := range e.AltNames {
				if f := Fold(alt); f != "" {
					ix.alts = append(ix.alts, f)
				}
			}
			s.entries = append(s.entries, ix)
		}
	}
}

// Catalog returns a site's loaded catalog
func (s *Store) Catalog(site string) (*SiteCatalog, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.catalogs[strings.ToLower(site)]

	return c, ok
}

// indexFile is the published listing of the catalog files
const indexFile = "index.json"

// sourcesFile lists the Tachiyomi sources the catalogs came through
const sourcesFile = "sources.json"

// shardFileRe names a sharded crawl's rows file (sources-3.json)
var shardFileRe = regexp.MustCompile(`^sources-\d+\.json$`)

// isCatalogFile tells a site's file from the index and the sources files
// that sit next to them (a host named sources.example.com is a site)
func isCatalogFile(name string) bool {
	return name != indexFile && name != sourcesFile && !shardFileRe.MatchString(name)
}

// DefaultDir is where the app keeps its catalogs: next to gui.json
func DefaultDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not find a user config dir for the catalog: %w", err)
	}

	return filepath.Join(dir, "manga-downloader", "catalog"), nil
}

// Write saves a site's catalog to Dir (atomically, 0600) and loads it
func (s *Store) Write(c *SiteCatalog) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(s.Dir, c.Site+".json"), b); err != nil {
		return err
	}
	cp := *c
	s.mu.Lock()
	s.catalogs[c.Site] = &cp
	s.reindex()
	s.mu.Unlock()

	return nil
}

// writeFileAtomic writes b to path through a temp file next to it, so a
// reader never sees a half-written file
func writeFileAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}

	return nil
}

// SiteStatus is one site's standing in the store, for `catalog status` and
// the app's site picker
type SiteStatus struct {
	Site     string
	Label    string
	Language string
	// Crawlable is the site table's crawl flag: whether a catalog can exist
	Crawlable bool
	// HasCatalog is whether this store holds one
	HasCatalog bool
	Entries    int
	// Crawled is when the catalog was built; FileAge how long ago the local
	// file was written (an update's download, or the crawl)
	Crawled time.Time
	FileAge time.Duration
	// Downloadable is whether the downloader has a grabber for the site (it
	// is in the site table); a site known only through a Tachiyomi source is
	// search only
	Downloadable bool
	// NSFW and Source come from the site's Tachiyomi source, when it has one
	NSFW   bool
	Source string
}

// Status describes every site: the table's, in table order, then the
// sites known only through a Tachiyomi source (sources.json), by site
func (s *Store) Status() []SiteStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sites := Sites()
	out := make([]SiteStatus, 0, len(sites)+len(s.sources))
	now := time.Now()
	fill := func(st *SiteStatus) {
		if c, ok := s.catalogs[st.Site]; ok {
			st.HasCatalog = true
			st.Entries = len(c.Entries)
			st.Crawled = c.Crawled
			if info, err := os.Stat(filepath.Join(s.Dir, st.Site+".json")); err == nil {
				st.FileAge = now.Sub(info.ModTime())
			}
		}
		if r, ok := s.sourceOf(st.Site); ok {
			st.NSFW = r.NSFW
			st.Source = r.Name
			st.Crawlable = st.Crawlable || r.SourceID != ""
		}
	}
	inTable := map[string]bool{}
	for _, site := range sites {
		inTable[site.Site] = true
		st := SiteStatus{Site: site.Site, Label: site.Label, Language: site.Lang, Crawlable: site.Crawl, Downloadable: true}
		fill(&st)
		out = append(out, st)
	}
	for _, r := range s.sources {
		if inTable[r.Site] {
			continue
		}
		st := SiteStatus{Site: r.Site, Label: hostLabel(r), Language: r.Lang}
		fill(&st)
		out = append(out, st)
	}

	return out
}
