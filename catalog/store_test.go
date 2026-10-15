// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreWriteLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	c := &SiteCatalog{Site: "a.test", Home: "https://a.test", Language: "en", Crawled: time.Now().UTC().Truncate(time.Second),
		Entries: []Entry{{Title: "One Piece", URL: "https://a.test/manga/one-piece/"}}}
	if err := s.Write(c); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "a.test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
	// in memory at once
	if hits := s.Search("one piece", nil, 0); len(hits) != 1 {
		t.Fatalf("written catalog not searchable: %v", hits)
	}
	// and from disk
	again := NewStore(dir)
	if warns := again.Load(); len(warns) != 0 {
		t.Fatal(warns)
	}
	got, ok := again.Catalog("a.test")
	if !ok || len(got.Entries) != 1 || !got.Crawled.Equal(c.Crawled) || got.Home != c.Home {
		t.Fatalf("got %+v", got)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp*")); len(leftovers) != 0 {
		t.Errorf("temp files left: %v", leftovers)
	}
}

func TestLoadSkipsBadFile(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "a.test", Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}})
	if err := os.WriteFile(filepath.Join(dir, "b.test.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, indexFile), []byte(`{"sites":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(dir)
	warns := s.Load()
	if len(warns) != 1 {
		t.Fatalf("want one warning, got %v", warns)
	}
	if _, ok := s.Catalog("a.test"); !ok {
		t.Error("good file not loaded")
	}
}

func TestLoadMissingDirIsEmpty(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "none"))
	if warns := s.Load(); len(warns) != 0 {
		t.Fatal(warns)
	}
	if s.Search("x", nil, 0) != nil {
		t.Error("hits from nowhere")
	}
}

func TestStatusListsEverySite(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "kingofshojo.com", Crawled: time.Now().Add(-2 * time.Hour), Entries: []Entry{{Title: "A", URL: "https://kingofshojo.com/manga/a/"}}})
	s := NewStore(dir)
	s.Load()
	status := s.Status()
	if len(status) != len(Sites()) {
		t.Fatalf("got %d rows, want one per site", len(status))
	}
	var kos, mangadex SiteStatus
	for _, st := range status {
		switch st.Site {
		case "kingofshojo.com":
			kos = st
		case "mangadex.org":
			mangadex = st
		}
	}
	if !kos.HasCatalog || kos.Entries != 1 || kos.Crawled.IsZero() || kos.FileAge <= 0 || kos.FileAge > time.Minute {
		t.Errorf("kingofshojo: %+v", kos)
	}
	if mangadex.HasCatalog || mangadex.Entries != 0 || mangadex.Label != "MangaDex" || mangadex.Crawlable {
		t.Errorf("mangadex: %+v", mangadex)
	}
	if kos.Language != "id" {
		t.Errorf("language comes from the site table: %+v", kos)
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "catalog" || filepath.Base(filepath.Dir(dir)) != "manga-downloader" {
		t.Errorf("got %s", dir)
	}
}

func TestSourcesLikeHostNamesAreCatalogs(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Write(&SiteCatalog{Site: "sources.example.com", Entries: []Entry{{Title: "A", URL: "https://sources.example.com/a/"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(&SiteCatalog{Site: "sources-xyz.com", Entries: []Entry{{Title: "B", URL: "https://sources-xyz.com/b/"}}}); err != nil {
		t.Fatal(err)
	}
	again := NewStore(dir)
	again.Load()
	for _, site := range []string{"sources.example.com", "sources-xyz.com"} {
		if _, ok := again.Catalog(site); !ok {
			t.Errorf("%s not loaded", site)
		}
	}
	idx, _ := BuildIndex(dir)
	if len(idx.Sites) != 2 {
		t.Errorf("index lists %d sites", len(idx.Sites))
	}
	if _, err := MergeSources(dir); err != nil {
		t.Errorf("merge took a host for a shard file: %v", err)
	}
}
