// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

// IndexEntry describes one published catalog file
type IndexEntry struct {
	Site    string    `json:"site"`
	File    string    `json:"file"`
	Entries int       `json:"entries"`
	Crawled time.Time `json:"crawled"`
	SHA256  string    `json:"sha256"`
}

// Index lists the catalog files of a directory (or of the published branch),
// what Update compares to download only what changed
type Index struct {
	Generated time.Time    `json:"generated"`
	Sites     []IndexEntry `json:"sites"`
}

// entry returns the site's entry, nil when it has none
func (idx *Index) entry(site string) *IndexEntry {
	if idx == nil {
		return nil
	}
	for i := range idx.Sites {
		if idx.Sites[i].Site == site {
			return &idx.Sites[i]
		}
	}

	return nil
}

// BuildIndex hashes every <site>.json in dir
func BuildIndex(dir string) (*Index, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	idx := &Index{Generated: time.Now().UTC()}
	for _, name := range names {
		if !isCatalogFile(filepath.Base(name)) {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		var c SiteCatalog
		if err := json.Unmarshal(b, &c); err != nil || c.Site == "" {
			continue
		}
		sum := sha256.Sum256(b)
		idx.Sites = append(idx.Sites, IndexEntry{Site: c.Site, File: filepath.Base(name), Entries: len(c.Entries), Crawled: c.Crawled, SHA256: hex.EncodeToString(sum[:])})
	}

	return idx, nil
}

// WriteIndex saves the index into dir, atomically
func WriteIndex(dir string, idx *Index) error {
	b, err := json.MarshalIndent(idx, "", " ")
	if err != nil {
		return err
	}

	return writeFileAtomic(filepath.Join(dir, indexFile), b)
}

// ReadIndex loads the index of dir; os.ErrNotExist when there's none
func ReadIndex(dir string) (*Index, error) {
	b, err := os.ReadFile(filepath.Join(dir, indexFile))
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, fmt.Errorf("%s: %w", indexFile, err)
	}

	return &idx, nil
}

// defaultBaseURL is where the nightly job publishes the catalogs
const defaultBaseURL = "https://raw.githubusercontent.com/Xopper/manga-downloader/catalog-data/catalog/"

// BaseURL is the published catalogs' location, MANGA_DOWNLOADER_CATALOG_URL
// when set (tests, forks), always ending in a slash
func BaseURL() string {
	u := strings.TrimSpace(os.Getenv("MANGA_DOWNLOADER_CATALOG_URL"))
	if u == "" {
		u = defaultBaseURL
	}

	return strings.TrimSuffix(u, "/") + "/"
}

// siteNameRe is what a site name in a published index may look like: a bare
// domain. Anything else (a path, "..", a separator) is refused, since the
// name becomes a file name under Dir.
var siteNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// validEntry reports whether a published index entry is safe to act on: the
// site is a bare domain and the file is that site's own file
func validEntry(e IndexEntry) bool {
	return siteNameRe.MatchString(e.Site) && !strings.Contains(e.Site, "..") && e.File == e.Site+".json"
}

// Update fetches the published index at baseURL and downloads every site
// file whose hash differs from the local index's, keeping the local file
// when a download fails or doesn't match its hash. The local index is
// written afterwards with the entries that succeeded (the old ones stay for
// the rest, so the next update retries them). It returns the sites that
// changed, and the first error.
func (s *Store) Update(ctx context.Context, baseURL string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	text, err := http.GetText(http.RequestParams{URL: baseURL + indexFile, DisableRetry: true})
	if err != nil {
		return nil, fmt.Errorf("catalog index: %w", err)
	}
	var remote Index
	if err := json.Unmarshal([]byte(text), &remote); err != nil {
		return nil, fmt.Errorf("catalog index: %w", err)
	}
	oldIndex, _ := os.Stat(filepath.Join(s.Dir, indexFile))
	local, err := ReadIndex(s.Dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// unreadable: treat as none, it's rewritten below
		local = nil
	}

	var changed []string
	var firstErr error
	fail := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	merged := &Index{Generated: remote.Generated}
	cancelled := false
	for i, re := range remote.Sites {
		if !validEntry(re) {
			fail(fmt.Errorf("catalog index: refusing entry %q (%s)", re.Site, re.File))
			continue
		}
		if le := local.entry(re.Site); le != nil && le.SHA256 == re.SHA256 {
			if _, ok := s.Catalog(re.Site); ok {
				merged.Sites = append(merged.Sites, re)
				continue
			}
		}
		if err := ctx.Err(); err != nil {
			// the sites not reached keep their local entries, so the index
			// stays whole and the next update picks up where this one stopped
			fail(err)
			cancelled = true
			for _, rest := range remote.Sites[i:] {
				if le := local.entry(rest.Site); le != nil {
					merged.Sites = append(merged.Sites, *le)
				}
			}
			break
		}
		body, err := http.GetText(http.RequestParams{URL: baseURL + re.File, DisableRetry: true})
		if err != nil {
			fail(fmt.Errorf("%s: %w", re.File, err))
			if le := local.entry(re.Site); le != nil {
				merged.Sites = append(merged.Sites, *le)
			}
			continue
		}
		sum := sha256.Sum256([]byte(body))
		if hex.EncodeToString(sum[:]) != re.SHA256 {
			fail(fmt.Errorf("%s: downloaded file doesn't match its hash", re.File))
			if le := local.entry(re.Site); le != nil {
				merged.Sites = append(merged.Sites, *le)
			}
			continue
		}
		var c SiteCatalog
		if err := json.Unmarshal([]byte(body), &c); err != nil || c.Site != re.Site {
			fail(fmt.Errorf("%s: not the catalog of %s", re.File, re.Site))
			continue
		}
		if err := s.Write(&c); err != nil {
			fail(err)
			continue
		}
		merged.Sites = append(merged.Sites, re)
		changed = append(changed, re.Site)
	}
	if cancelled && len(changed) == 0 {
		// nothing moved: the old index (and its age) stands
		return nil, firstErr
	}
	if err := WriteIndex(s.Dir, merged); err != nil {
		fail(err)
	}
	if cancelled && oldIndex != nil {
		// the index isn't fresh: it keeps its old age so the next start
		// refreshes instead of waiting a day
		_ = os.Chtimes(filepath.Join(s.Dir, indexFile), oldIndex.ModTime(), oldIndex.ModTime())
	}

	return changed, firstErr
}

// Gzip turns every catalog file of dir into <site>.json.gz (the published
// form) and removes the plain file
func Gzip(dir string) error {
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	for _, name := range names {
		if !isCatalogFile(filepath.Base(name)) {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(b); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		if err := writeFileAtomic(name+".gz", buf.Bytes()); err != nil {
			return err
		}
		if err := os.Remove(name); err != nil {
			return err
		}
	}

	return nil
}
