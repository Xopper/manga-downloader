// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeRows(t *testing.T, dir, name string, rows []SourceRow) {
	t.Helper()
	b, _ := json.Marshal(rows)
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSourcesShards(t *testing.T) {
	dir := t.TempDir()
	writeRows(t, dir, "sources.json", []SourceRow{{Site: "old.test", Name: "Old", Lang: "en", SourceID: "9", Entries: 5}, {Site: "a.test", Name: "A old", Lang: "en", SourceID: "1", Entries: 1}})
	writeRows(t, dir, "sources-0.json", []SourceRow{{Site: "a.test", Name: "A", Lang: "en", SourceID: "1", Entries: 10, Complete: true}})
	writeRows(t, dir, "sources-1.json", []SourceRow{{Site: "b.test", Name: "B", Lang: "en", SourceID: "2", Entries: 3, NSFW: true}})
	rows, err := MergeSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]SourceRow{}
	for _, r := range rows {
		by[r.Site] = r
	}
	if by["a.test"].Entries != 10 || by["a.test"].Name != "A" {
		t.Errorf("shard row must win over the old one: %+v", by["a.test"])
	}
	if by["old.test"].Entries != 5 {
		t.Error("a site no shard touched keeps its row")
	}
	if !by["b.test"].NSFW {
		t.Error("b lost")
	}
	if _, err := os.Stat(filepath.Join(dir, "sources-0.json")); err == nil {
		t.Error("shard files must be removed once merged")
	}
	back, err := os.ReadFile(filepath.Join(dir, "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved []SourceRow
	json.Unmarshal(back, &saved)
	if len(saved) != len(rows) || saved[0].Site > saved[1].Site {
		t.Errorf("saved %+v", saved)
	}
}

func TestMergeSourcesKeepsTableSites(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "mangaread.org", Entries: []Entry{{Title: "A", URL: "https://www.mangaread.org/manga/a/"}}})
	writeRows(t, dir, "sources-0.json", []SourceRow{{Site: "x.test", Name: "X", Lang: "en", SourceID: "2", Entries: 3}})
	rows, err := MergeSources(dir)
	if err != nil {
		t.Fatal(err)
	}
	var mr, x SourceRow
	for _, r := range rows {
		switch r.Site {
		case "mangaread.org":
			mr = r
		case "x.test":
			x = r
		}
	}
	if !mr.Downloadable || mr.SourceID != "" || mr.Entries != 1 || mr.Lang != "en" {
		t.Errorf("a table site with a file but no source gets a row: %+v", mr)
	}
	if x.Downloadable {
		t.Errorf("x: %+v", x)
	}
}

func TestStatusIncludesSources(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "x.test", Language: "en", Crawled: time.Now(), Entries: []Entry{{Title: "A", URL: "https://x.test/a/"}}})
	writeRows(t, dir, "sources.json", []SourceRow{{Site: "x.test", Name: "X", Lang: "en", SourceID: "2", Entries: 1, NSFW: true}, {Site: "y.test", Name: "Y", Lang: "en", SourceID: "3", Failed: true}})
	s := NewStore(dir)
	if warns := s.Load(); len(warns) != 0 {
		t.Fatal(warns)
	}
	if len(s.Sources()) != 2 {
		t.Fatalf("sources %+v", s.Sources())
	}
	status := s.Status()
	if len(status) != len(Sites())+2 {
		t.Fatalf("got %d rows", len(status))
	}
	var x, y, mr SiteStatus
	for _, st := range status {
		switch st.Site {
		case "x.test":
			x = st
		case "y.test":
			y = st
		case "mangaread.org":
			mr = st
		}
	}
	if !x.HasCatalog || x.Downloadable || !x.NSFW || x.Source != "X" || x.Entries != 1 || x.Label != "X" {
		t.Errorf("x: %+v", x)
	}
	if y.HasCatalog || y.Downloadable || !y.Crawlable {
		t.Errorf("y: %+v", y)
	}
	if !mr.Downloadable || !mr.Crawlable {
		t.Errorf("table site: %+v", mr)
	}
	if !s.Downloadable("mangaread.org") || s.Downloadable("x.test") {
		t.Error("Downloadable is the site table")
	}
}
