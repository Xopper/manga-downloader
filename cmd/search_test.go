// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/catalog"
)

// searchTestStore is a loaded store with two sites in a temp dir
func searchTestStore(t *testing.T) *catalog.Store {
	t.Helper()
	dir := t.TempDir()
	write := func(c catalog.SiteCatalog) {
		b, _ := json.Marshal(c)
		if err := os.WriteFile(filepath.Join(dir, c.Site+".json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(catalog.SiteCatalog{Site: "a.test", Crawled: time.Now(), Entries: []catalog.Entry{
		{Title: "One Piece", URL: "https://a.test/manga/one-piece/"},
		{Title: "One Piece: a very long subtitle that goes on and on past sixty characters for sure", URL: "https://a.test/manga/one-piece-long/"},
	}})
	write(catalog.SiteCatalog{Site: "b.test", Crawled: time.Now(), Entries: []catalog.Entry{
		{Title: "One Piece", URL: "https://b.test/manga/one-piece/"},
	}})
	s := catalog.NewStore(dir)
	if warns := s.Load(); len(warns) != 0 {
		t.Fatal(warns)
	}

	return s
}

func TestRunSearchTable(t *testing.T) {
	var out bytes.Buffer
	code := runSearch(&out, searchTestStore(t), "one piece", "", 0, false)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "SITE") {
		t.Fatalf("got %q", out.String())
	}
	if !strings.HasPrefix(lines[1], "a.test") || !strings.Contains(lines[1], "https://a.test/manga/one-piece/") {
		t.Errorf("first row: %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "b.test") {
		t.Errorf("second row: %q", lines[2])
	}
	if !strings.Contains(lines[3], "...") || strings.Contains(lines[3], "for sure") {
		t.Errorf("long title not truncated: %q", lines[3])
	}
}

func TestRunSearchJSON(t *testing.T) {
	var out bytes.Buffer
	if code := runSearch(&out, searchTestStore(t), "one piece", "b.test", 0, true); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var hits []searchHit
	if err := json.Unmarshal(out.Bytes(), &hits); err != nil {
		t.Fatal(err, out.String())
	}
	if len(hits) != 1 || hits[0].Site != "b.test" || hits[0].Title != "One Piece" || hits[0].URL != "https://b.test/manga/one-piece/" {
		t.Fatalf("got %+v", hits)
	}
}

func TestRunSearchNoResultsExit1(t *testing.T) {
	var out bytes.Buffer
	if code := runSearch(&out, searchTestStore(t), "bleach", "", 0, false); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "No results") {
		t.Errorf("got %q", out.String())
	}
	out.Reset()
	if code := runSearch(&out, searchTestStore(t), "bleach", "", 0, true); code != 1 || strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("json: exit %d, %q", code, out.String())
	}
}

func TestRunSearchLimit(t *testing.T) {
	var out bytes.Buffer
	runSearch(&out, searchTestStore(t), "one piece", "", 1, false)
	if n := strings.Count(out.String(), "\n"); n != 2 {
		t.Errorf("want a header and one row, got %q", out.String())
	}
}

func TestOpenStoreKeepsStdoutForResults(t *testing.T) {
	// the first-run download message and the load warnings go to stderr, so
	// `search --json | jq` works on a fresh machine
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	dir, err := catalog.DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600)
	t.Setenv("MANGA_DOWNLOADER_CATALOG_URL", "http://127.0.0.1:1/")
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	_, err = openStore(true)
	w.Close()
	os.Stdout = stdout
	var got bytes.Buffer
	got.ReadFrom(r)
	if err == nil {
		t.Fatal("an unreachable publish must be an error")
	}
	if got.Len() != 0 {
		t.Errorf("stdout got %q", got.String())
	}
}
