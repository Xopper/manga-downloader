// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Store is the catalogs on disk (one <site>.json per site under Dir) held in
// memory for searching
type Store struct {
	Dir string

	mu       sync.RWMutex
	catalogs map[string]*SiteCatalog
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
	names, err := filepath.Glob(filepath.Join(s.Dir, "*.json"))
	if err != nil {
		return []error{err}
	}
	sort.Strings(names)
	catalogs := make(map[string]*SiteCatalog, len(names))
	var warnings []error
	for _, name := range names {
		if filepath.Base(name) == indexFile {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			warnings = append(warnings, err)
			continue
		}
		var c SiteCatalog
		if err := json.Unmarshal(b, &c); err != nil || c.Site == "" {
			warnings = append(warnings, fmt.Errorf("%s: not a catalog file: %v", filepath.Base(name), err))
			continue
		}
		catalogs[c.Site] = &c
	}

	s.mu.Lock()
	s.catalogs = catalogs
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
