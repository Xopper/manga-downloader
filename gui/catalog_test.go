// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/catalog"
	"github.com/elboletaire/manga-downloader/grabber"
)

// catalogDir writes a published-looking catalog dir: files plus index
func catalogDir(t *testing.T, catalogs ...catalog.SiteCatalog) string {
	t.Helper()
	dir := t.TempDir()
	for _, c := range catalogs {
		b, _ := json.Marshal(c)
		if err := os.WriteFile(filepath.Join(dir, c.Site+".json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	idx, err := catalog.BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.WriteIndex(dir, idx); err != nil {
		t.Fatal(err)
	}

	return dir
}

// serveCatalog publishes dir over HTTP, counting requests
func serveCatalog(t *testing.T, dir string) (base string, hits *atomic.Int32) {
	t.Helper()
	hits = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/catalog/", hits
}

var onePiece = catalog.SiteCatalog{Site: "mangaread.org", Language: "en", Crawled: time.Unix(1000, 0).UTC(), Entries: []catalog.Entry{
	{Title: "One Piece", URL: "https://www.mangaread.org/manga/one-piece/"},
	{Title: "Bleach", URL: "https://www.mangaread.org/manga/bleach/"},
}}

func TestSearchUsesStore(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	store := catalog.NewStore(catalogDir(t, onePiece))
	store.Load()
	c.SetCatalog(store)
	hits := c.Search("one piece", "")
	if len(hits) != 1 || hits[0].Entry.URL != "https://www.mangaread.org/manga/one-piece/" {
		t.Fatalf("got %+v", hits)
	}
	if hits := c.Search("one piece", "nope.test"); len(hits) != 0 {
		t.Errorf("site filter ignored: %+v", hits)
	}
	if c.Search("", "") != nil {
		t.Error("empty query matched")
	}
	sites := c.Sites()
	if len(sites) != len(catalog.Sites()) {
		t.Errorf("Sites: %d", len(sites))
	}
	info := c.CatalogInfo()
	if info.Sites != 1 || info.Entries != 2 || info.Updating || info.Error != "" {
		t.Errorf("info: %+v", info)
	}
}

func TestRefreshSkipsFreshIndex(t *testing.T) {
	// the local store already has a fresh index: no request goes out
	local := catalogDir(t, onePiece)
	base, hits := serveCatalog(t, local)
	c := NewController(&grabber.Settings{}, nil)
	store := catalog.NewStore(local)
	store.Load()
	c.SetCatalog(store)
	c.catalogURL = base
	c.RefreshCatalog(context.Background(), false)
	if hits.Load() != 0 {
		t.Errorf("%d requests for a fresh index", hits.Load())
	}
	// an old index is refreshed
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(filepath.Join(local, "index.json"), old, old)
	c.RefreshCatalog(context.Background(), false)
	if hits.Load() != 1 {
		t.Errorf("%d requests for a stale index (want the index only: nothing changed)", hits.Load())
	}
}

func TestRefreshForceDownloads(t *testing.T) {
	var notified atomic.Int32
	c := NewController(&grabber.Settings{}, func() { notified.Add(1) })
	store := catalog.NewStore(t.TempDir())
	store.Load()
	c.SetCatalog(store)
	base, hits := serveCatalog(t, catalogDir(t, onePiece))
	c.catalogURL = base
	before := c.CatalogInfo().Generation
	c.RefreshCatalog(context.Background(), true)
	if hits.Load() != 2 {
		t.Errorf("%d requests, want index + one file", hits.Load())
	}
	info := c.CatalogInfo()
	if info.Sites != 1 || info.Entries != 2 || info.Generation == before || info.UpdatedAt.IsZero() || info.Updating || info.Error != "" {
		t.Errorf("info: %+v", info)
	}
	if notified.Load() == 0 {
		t.Error("the window wasn't told")
	}
	if len(c.Search("bleach", "")) != 1 {
		t.Error("downloaded catalog not searchable")
	}
}

func TestRefreshErrorKeptInInfo(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	store := catalog.NewStore(t.TempDir())
	c.SetCatalog(store)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }))
	t.Cleanup(srv.Close)
	c.catalogURL = srv.URL + "/"
	c.RefreshCatalog(context.Background(), true)
	info := c.CatalogInfo()
	if info.Error == "" || info.Updating {
		t.Errorf("info: %+v", info)
	}
	// a later success clears it
	base, _ := serveCatalog(t, catalogDir(t, onePiece))
	c.catalogURL = base
	c.RefreshCatalog(context.Background(), true)
	if info := c.CatalogInfo(); info.Error != "" || info.Sites != 1 {
		t.Errorf("info after success: %+v", info)
	}
}

func TestRefreshWithoutStoreIsNoop(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	c.RefreshCatalog(context.Background(), true)
	if c.Search("x", "") != nil || c.CatalogInfo().Sites != 0 {
		t.Error("a controller without a store must answer empty")
	}
}

func TestStateCarriesCatalog(t *testing.T) {
	app, _, _ := newTestApp(t, 1)
	store := catalog.NewStore(catalogDir(t, onePiece))
	store.Load()
	app.ctrl.SetCatalog(store)
	st := app.State()
	if st.Catalog.Sites != 1 || st.Catalog.Entries != 2 {
		t.Errorf("state.catalog: %+v", st.Catalog)
	}
	hits := app.Search("one piece", "")
	if len(hits) != 1 || hits[0].Site != "mangaread.org" || hits[0].Title != "One Piece" || hits[0].URL != "https://www.mangaread.org/manga/one-piece/" || hits[0].Language != "en" {
		t.Errorf("hits: %+v", hits)
	}
	var mangaread SiteDTO
	for _, s := range app.Sites() {
		if s.Site == "mangaread.org" {
			mangaread = s
		}
	}
	if !mangaread.HasCatalog || mangaread.Entries != 2 || mangaread.Language != "en" {
		t.Errorf("site: %+v", mangaread)
	}
	if cs := app.CatalogStatus(); cs.Sites != 1 || cs.Entries != 2 {
		t.Errorf("status: %+v", cs)
	}
}

func TestOpenCatalogWithoutConfigDirIsNil(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if store := openCatalog(); store != nil {
		t.Fatalf("a store with no dir would write into the working directory: %+v", store.Dir)
	}
	c := NewController(&grabber.Settings{}, nil)
	c.SetCatalog(nil)
	c.RefreshCatalog(context.Background(), true)
	if c.Search("x", "") != nil || len(c.Sites()) != len(catalog.Sites()) {
		t.Error("a nil store must answer empty")
	}
}

func TestHitsCarryDownloadable(t *testing.T) {
	app, _, _ := newTestApp(t, 1)
	dir := catalogDir(t, onePiece, catalog.SiteCatalog{Site: "x.test", Language: "en", Crawled: time.Unix(1000, 0).UTC(), Entries: []catalog.Entry{
		{Title: "One Piece", URL: "https://x.test/manga/one-piece/"},
	}})
	rows, _ := json.Marshal([]catalog.SourceRow{{Site: "x.test", Name: "X Source", Lang: "en", SourceID: "2", Entries: 1, NSFW: true}})
	if err := os.WriteFile(filepath.Join(dir, "sources.json"), rows, 0o600); err != nil {
		t.Fatal(err)
	}
	store := catalog.NewStore(dir)
	store.Load()
	app.ctrl.SetCatalog(store)
	hits := app.Search("one piece", "")
	if len(hits) != 2 {
		t.Fatalf("hits %+v", hits)
	}
	for _, h := range hits {
		switch h.Site {
		case "mangaread.org":
			if !h.Downloadable || h.NSFW {
				t.Errorf("mangaread hit: %+v", h)
			}
		case "x.test":
			if h.Downloadable || !h.NSFW || h.Language != "en" {
				t.Errorf("x hit: %+v", h)
			}
		}
	}
	var x SiteDTO
	for _, s := range app.Sites() {
		if s.Site == "x.test" {
			x = s
		}
	}
	if x.Downloadable || !x.NSFW || !x.HasCatalog || x.Label != "X Source" {
		t.Errorf("x site: %+v", x)
	}
	if cs := app.CatalogStatus(); cs.Sites != 2 || cs.SearchOnly != 1 {
		t.Errorf("status: %+v", cs)
	}
}
