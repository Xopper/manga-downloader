// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/elboletaire/manga-downloader/catalog"
)

// catalogMaxAge is how old the local index may be before a start refreshes it
const catalogMaxAge = 24 * time.Hour

// CatalogInfo sums the catalogs up for the window
type CatalogInfo struct {
	// Sites and Entries are how many catalogs the store holds and their
	// entries together
	Sites   int
	Entries int
	// UpdatedAt is when the local index was last written, zero without one
	UpdatedAt time.Time
	Updating  bool
	// Error is the last refresh's failure, "" once one succeeds
	Error string
	// Generation moves on every refresh that changed something, so the window
	// can tell a new catalog from a push with the same counts
	Generation uint64
}

// SetCatalog gives the controller its catalog store (loaded by the caller).
// Without one, Search finds nothing and RefreshCatalog does nothing.
func (c *Controller) SetCatalog(s *catalog.Store) {
	c.mu.Lock()
	c.catalog = s
	c.catalogGen++
	c.mu.Unlock()
	c.emit()
}

// RefreshCatalog downloads the catalogs that changed upstream. Unless force
// is set, it does nothing while the local index is younger than
// catalogMaxAge. It's meant to run in the background; a failure is kept in
// CatalogInfo and logged to stderr, never the window's log.
func (c *Controller) RefreshCatalog(ctx context.Context, force bool) {
	c.mu.Lock()
	store := c.catalog
	if store == nil || c.catalogUpdating {
		c.mu.Unlock()
		return
	}
	if !force && !catalogStale(store.Dir) {
		c.mu.Unlock()
		return
	}
	c.catalogUpdating = true
	url := c.catalogURL
	c.mu.Unlock()
	c.emit()

	changed, err := store.Update(ctx, url)

	c.mu.Lock()
	c.catalogUpdating = false
	if err != nil {
		c.catalogErr = err.Error()
		if ctx.Err() == nil {
			log.Printf("couldn't update the catalog: %v", err)
		}
	} else {
		c.catalogErr = ""
	}
	if len(changed) > 0 || err == nil {
		c.catalogGen++
	}
	c.mu.Unlock()
	c.emit()
}

// catalogStale reports whether dir's index is missing or older than
// catalogMaxAge
func catalogStale(dir string) bool {
	info, err := os.Stat(dir + string(os.PathSeparator) + "index.json")
	if err != nil {
		return true
	}

	return time.Since(info.ModTime()) > catalogMaxAge
}

// Search finds series in the catalogs; site "" means every site
func (c *Controller) Search(query, site string) []catalog.Hit {
	c.mu.Lock()
	store := c.catalog
	c.mu.Unlock()
	if store == nil {
		return nil
	}
	var sites []string
	if site != "" {
		sites = []string{site}
	}

	return store.Search(query, sites, 0)
}

// Sites is every site of the table with its catalog's standing
func (c *Controller) Sites() []catalog.SiteStatus {
	c.mu.Lock()
	store := c.catalog
	c.mu.Unlock()
	if store == nil {
		store = catalog.NewStore("")
	}

	return store.Status()
}

// CatalogInfo sums the store up
func (c *Controller) CatalogInfo() CatalogInfo {
	c.mu.Lock()
	store := c.catalog
	info := CatalogInfo{Updating: c.catalogUpdating, Error: c.catalogErr, Generation: c.catalogGen}
	c.mu.Unlock()
	if store == nil {
		return info
	}
	for _, st := range store.Status() {
		if st.HasCatalog {
			info.Sites++
			info.Entries += st.Entries
		}
	}
	if fi, err := os.Stat(store.Dir + string(os.PathSeparator) + "index.json"); err == nil {
		info.UpdatedAt = fi.ModTime()
	}

	return info
}

// HitDTO is a search result
type HitDTO struct {
	Site     string `json:"site"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Language string `json:"language"`
}

// SiteDTO is a site of the picker
type SiteDTO struct {
	Site     string `json:"site"`
	Label    string `json:"label"`
	Language string `json:"language"`
	// HasCatalog is whether the site can be searched; Crawlable whether a
	// catalog exists for it at all (false: the site has no crawler yet)
	HasCatalog bool `json:"hasCatalog"`
	Crawlable  bool `json:"crawlable"`
	Entries    int  `json:"entries"`
}

// CatalogDTO sums the catalogs up: State.catalog
type CatalogDTO struct {
	Sites   int `json:"sites"`
	Entries int `json:"entries"`
	// UpdatedAt is RFC 3339, "" without a local index yet
	UpdatedAt  string `json:"updatedAt"`
	Updating   bool   `json:"updating"`
	Error      string `json:"error"`
	Generation uint64 `json:"generation"`
}

func catalogDTO(info CatalogInfo) CatalogDTO {
	dto := CatalogDTO{Sites: info.Sites, Entries: info.Entries, Updating: info.Updating, Error: info.Error, Generation: info.Generation}
	if !info.UpdatedAt.IsZero() {
		dto.UpdatedAt = info.UpdatedAt.UTC().Format(time.RFC3339)
	}

	return dto
}

// Search finds series in the catalogs, best match first; site "" is all
func (a *App) Search(query, site string) []HitDTO {
	hits := a.ctrl.Search(query, site)
	out := make([]HitDTO, 0, len(hits))
	for _, h := range hits {
		lang := ""
		if s, ok := catalog.SiteByHost(h.Site); ok {
			lang = s.Lang
		}
		out = append(out, HitDTO{Site: h.Site, Title: h.Entry.Title, URL: h.Entry.URL, Language: lang})
	}

	return out
}

// Sites lists every site for the picker
func (a *App) Sites() []SiteDTO {
	status := a.ctrl.Sites()
	out := make([]SiteDTO, 0, len(status))
	for _, st := range status {
		out = append(out, SiteDTO{Site: st.Site, Label: st.Label, Language: st.Language, HasCatalog: st.HasCatalog, Crawlable: st.Crawlable, Entries: st.Entries})
	}

	return out
}

// CatalogStatus sums the catalogs up
func (a *App) CatalogStatus() CatalogDTO {
	return catalogDTO(a.ctrl.CatalogInfo())
}

// UpdateCatalog refreshes the catalogs now, in the background; the result
// reaches the window through State.catalog
func (a *App) UpdateCatalog() error {
	if a.ctrl.CatalogInfo().Updating {
		return errors.New("the catalog is already updating")
	}
	go a.ctrl.RefreshCatalog(a.updateCtx, true)

	return nil
}

// openCatalog loads the catalog store from the config dir; a missing dir is
// an empty store. Warnings (a file that doesn't parse) go to stderr.
func openCatalog() *catalog.Store {
	dir, err := catalog.DefaultDir()
	if err != nil {
		log.Printf("no catalog dir: %v", err)
		return catalog.NewStore("")
	}
	store := catalog.NewStore(dir)
	for _, warn := range store.Load() {
		log.Printf("catalog: %v", warn)
	}

	return store
}
