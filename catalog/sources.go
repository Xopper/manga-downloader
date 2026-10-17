// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MergeSources folds every sources-<shard>.json in dir into sources.json:
// a shard's row replaces the old row of its site, sites no shard touched
// keep theirs, and a site of the table that has a catalog file but no
// source gets a row of its own (downloadable, no source id). The merged
// rows are written sorted by site and the shard files removed.
func MergeSources(dir string) ([]SourceRow, error) {
	by := map[string]SourceRow{}
	if old, err := readRows(filepath.Join(dir, sourcesFile)); err == nil {
		for _, r := range old {
			by[r.Site] = r
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	found, err := filepath.Glob(filepath.Join(dir, "sources-*.json"))
	if err != nil {
		return nil, err
	}
	var shards []string
	for _, name := range found {
		if shardFileRe.MatchString(filepath.Base(name)) {
			shards = append(shards, name)
		}
	}
	sort.Strings(shards)
	for _, name := range shards {
		rows, err := readRows(name)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			by[r.Site] = r
		}
	}
	// the table's sites with a file, so the app's picker and status see them
	// whether a source covers them or not
	store := NewStore(dir)
	store.Load()
	for _, site := range Sites() {
		if !store.has(site.Site) {
			continue
		}
		n := store.count(site.Site)
		if r, has := by[site.Site]; has {
			r.Downloadable = true
			if r.Entries < n {
				r.Entries = n
			}
			by[site.Site] = r
			continue
		}
		by[site.Site] = SourceRow{Site: site.Site, Name: site.Label, Lang: site.Lang, Entries: n, Complete: true, Downloadable: true}
	}
	rows := make([]SourceRow, 0, len(by))
	for _, r := range by {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Site < rows[j].Site })
	b, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(filepath.Join(dir, sourcesFile), b); err != nil {
		return nil, err
	}
	for _, name := range shards {
		os.Remove(name)
	}

	return rows, nil
}

// readRows reads a sources file
func readRows(path string) ([]SourceRow, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []SourceRow
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	kept := rows[:0]
	for _, r := range rows {
		if !Skipped(r.Site) {
			kept = append(kept, r)
		}
	}

	return kept, nil
}

// Sources returns the rows of sources.json the store loaded, nil without one
func (s *Store) Sources() []SourceRow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SourceRow, len(s.sources))
	copy(out, s.sources)

	return out
}

// Downloadable reports whether the downloader has a grabber for the site:
// it is in the site table
func (s *Store) Downloadable(site string) bool {
	_, ok := SiteByHost(site)

	return ok
}

// sourceOf finds a site's source row; the caller holds mu
func (s *Store) sourceOf(site string) (SourceRow, bool) {
	for _, r := range s.sources {
		if r.Site == site {
			return r, true
		}
	}

	return SourceRow{}, false
}

// hostLabel is a site's label for a source-only site: its source name
func hostLabel(r SourceRow) string {
	if strings.TrimSpace(r.Name) != "" {
		return r.Name
	}

	return r.Site
}
