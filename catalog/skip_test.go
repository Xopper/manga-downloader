// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkipped(t *testing.T) {
	for site, want := range map[string]bool{
		"pawchive.pw":      true,
		"www.XKCD.com":     true,
		"mangadex.org":     false,
		"allporncomic.com": false, // adult manga sites stay
	} {
		if got := Skipped(site); got != want {
			t.Errorf("Skipped(%q) = %v, want %v", site, got, want)
		}
	}
}

func TestTheTableHasNoSkippedSite(t *testing.T) {
	for _, s := range Sites() {
		if Skipped(s.Site) {
			t.Errorf("%s is in sites.json and in the skip list", s.Site)
		}
	}
}

// TestASkippedSiteIsNotLoaded: its catalog file and its sources.json row,
// left from before the skip, are neither searched nor listed nor indexed
func TestASkippedSiteIsNotLoaded(t *testing.T) {
	dir := t.TempDir()
	for site, body := range map[string]string{
		"xkcd.com":     `{"site":"xkcd.com","entries":[{"title":"Comic","url":"https://xkcd.com/1/"}]}`,
		"mangadex.org": `{"site":"mangadex.org","entries":[{"title":"Comic Girls","url":"https://mangadex.org/title/1"}]}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, site+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rows := `[{"site":"xkcd.com","name":"xkcd"},{"site":"mangadex.org","name":"MangaDex"}]`
	if err := os.WriteFile(filepath.Join(dir, sourcesFile), []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}

	s := NewStore(dir)
	if errs := s.Load(); len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, h := range s.Search("comic", nil, 10) {
		if h.Site == "xkcd.com" {
			t.Error("a skipped site's entry was found")
		}
	}
	for _, r := range s.Sources() {
		if r.Site == "xkcd.com" {
			t.Error("a skipped site's source row was kept")
		}
	}
	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Sites) != 1 || idx.Sites[0].Site != "mangadex.org" {
		t.Errorf("index lists %+v, want mangadex.org only", idx.Sites)
	}
}

func TestDropSkippedRemovesOnlyTheSkippedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"xkcd.com.json", "pawchive.pw.json.gz", "mangadex.org.json.gz", sourcesFile} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	NewStore(dir).dropSkipped()
	for name, want := range map[string]bool{"xkcd.com.json": false, "pawchive.pw.json.gz": false, "mangadex.org.json.gz": true, sourcesFile: true} {
		_, err := os.Stat(filepath.Join(dir, name))
		if got := err == nil; got != want {
			t.Errorf("%s on disk: %v, want %v", name, got, want)
		}
	}
}
