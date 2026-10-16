// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store is the catalogs on disk (one <site>.json per site under Dir) held in
// memory for searching
type Store struct {
	Dir string

	mu    sync.RWMutex
	sites map[string]*siteData
	// blocks is every site's block by site: what Search scans
	blocks []*block
	// sources is sources.json: the Tachiyomi sources the catalogs came
	// through, nil without one
	sources []SourceRow
}

// siteData is a loaded catalog: its header, and its entries packed. The
// Entry values themselves aren't kept (a million and a half of them were
// most of the app's memory); Catalog rebuilds them from the block.
type siteData struct {
	meta  SiteCatalog
	block *block
}

// NewStore makes a store over dir; nothing is read until Load
func NewStore(dir string) *Store {
	return &Store{Dir: dir, sites: map[string]*siteData{}}
}

// Load reads every <site>.json in Dir (index.json excluded). A missing dir
// is an empty store; a file that doesn't parse is skipped and reported.
func (s *Store) Load() []error {
	names, err := catalogFiles(s.Dir)
	if err != nil {
		return []error{err}
	}
	// the files are read, parsed and packed on every core at once: a few
	// hundred of them hold over a million entries, and one core took
	// seconds. Each file's entries live only until its block is built.
	type parsed struct {
		data *siteData
		err  error
	}
	results := make([]parsed, len(names))
	inParallel(len(names), func(i int) {
		name := names[i]
		b, err := os.ReadFile(name)
		if err != nil {
			results[i].err = err
			return
		}
		if b, err = maybeGunzip(filepath.Base(name), b); err != nil {
			results[i].err = fmt.Errorf("%s: %w", filepath.Base(name), err)
			return
		}
		var c SiteCatalog
		if err := json.Unmarshal(b, &c); err != nil || c.Site == "" {
			results[i].err = fmt.Errorf("%s: not a catalog file: %v", filepath.Base(name), err)
			return
		}
		results[i].data = newSiteData(&c)
	})
	sites := make(map[string]*siteData, len(names))
	var warnings []error
	for _, r := range results {
		if r.err != nil {
			warnings = append(warnings, r.err)
			continue
		}
		sites[r.data.meta.Site] = r.data
	}

	sources, err := readRows(filepath.Join(s.Dir, sourcesFile))
	if err != nil && !os.IsNotExist(err) {
		warnings = append(warnings, fmt.Errorf("%s: %w", sourcesFile, err))
	}

	s.mu.Lock()
	s.sites = sites
	s.sources = sources
	s.sortBlocks()
	s.mu.Unlock()

	return warnings
}

// newSiteData packs a catalog: its header kept as is, its entries folded
// into a block
func newSiteData(c *SiteCatalog) *siteData {
	meta := *c
	meta.Entries = nil

	return &siteData{meta: meta, block: pack(c.Site, c.Entries)}
}

// sortBlocks lists the sites' blocks by site; the caller holds mu
func (s *Store) sortBlocks() {
	s.blocks = make([]*block, 0, len(s.sites))
	for _, d := range s.sites {
		s.blocks = append(s.blocks, d.block)
	}
	sort.Slice(s.blocks, func(i, j int) bool { return s.blocks[i].site < s.blocks[j].site })
}

// inParallel runs fn(i) for every i below n, as many at a time as there are
// cores, and returns once they're all done
func inParallel(n int, fn func(i int)) {
	workers := min(runtime.NumCPU(), n)
	if workers <= 1 {
		for i := 0; i < n; i++ {
			fn(i)
		}
		return
	}
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		next <- i
	}
	close(next)
	wg.Wait()
}

// Catalog returns a site's loaded catalog, its entries rebuilt from the
// packed block (a copy: the caller may change it)
func (s *Store) Catalog(site string) (*SiteCatalog, bool) {
	s.mu.RLock()
	d, ok := s.sites[strings.ToLower(site)]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	c := d.meta
	c.Entries = d.block.entries()

	return &c, true
}

// has reports whether the store holds a site's catalog, without rebuilding
// its entries
func (s *Store) has(site string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.sites[strings.ToLower(site)]

	return ok
}

// count is how many entries a site's catalog has, 0 without one
func (s *Store) count(site string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if d, ok := s.sites[strings.ToLower(site)]; ok {
		return d.block.n
	}

	return 0
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

// Write saves a site's catalog to Dir (atomically, 0600) and loads it. The
// entries are packed before the store is locked, so a search during an
// update waits only for the swap.
func (s *Store) Write(c *SiteCatalog) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(s.Dir, c.Site+".json"), b); err != nil {
		return err
	}
	d := newSiteData(c)
	s.mu.Lock()
	s.sites[c.Site] = d
	s.sortBlocks()
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
		if d, ok := s.sites[st.Site]; ok {
			st.HasCatalog = true
			st.Entries = d.block.n
			st.Crawled = d.meta.Crawled
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
