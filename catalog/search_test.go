// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFold(t *testing.T) {
	cases := map[string]string{
		"Café Ōtsuki!!":     "cafe otsuki",
		"  One   Piece ":    "one piece",
		"ｶﾌｪ":               "カフェ",
		"Dr. STONE":         "dr stone",
		"---":               "",
		"Naruto: Shippūden": "naruto shippuden",
		"東京喰種":              "東京喰種",
		"Ａｂｃ":               "abc",
	}
	for in, want := range cases {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

// writeCatalog writes a site's file into dir, as the store expects it
func writeCatalog(t *testing.T, dir string, c SiteCatalog) {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, c.Site+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func searchStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "a.test", Language: "en", Crawled: time.Now(), Entries: []Entry{
		{Title: "One Piece", URL: "https://a.test/manga/one-piece/"},
		{Title: "One Piece: Ace's Story", URL: "https://a.test/manga/one-piece-ace/"},
		{Title: "Piece of Cake", URL: "https://a.test/manga/piece-of-cake/"},
		{Title: "Someone, Pieces", URL: "https://a.test/manga/someone-pieces/"},
		{Title: "Wan Pisu", URL: "https://a.test/manga/wan-pisu/", AltNames: []string{"ワンピース", "One Piece (JP)"}},
	}})
	writeCatalog(t, dir, SiteCatalog{Site: "b.test", Language: "es", Crawled: time.Now(), Entries: []Entry{
		{Title: "One Piece", URL: "https://b.test/manga/one-piece/"},
		{Title: "Bleach", URL: "https://b.test/manga/bleach/"},
	}})
	s := NewStore(dir)
	if warns := s.Load(); len(warns) != 0 {
		t.Fatal(warns)
	}

	return s
}

func titles(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Site + ":" + h.Entry.Title
	}

	return out
}

func TestLoadReadsCatalogs(t *testing.T) {
	s := searchStore(t)
	if c, ok := s.Catalog("a.test"); !ok || len(c.Entries) != 5 {
		t.Fatalf("a.test: %+v %v", c, ok)
	}
	if _, ok := s.Catalog("nope"); ok {
		t.Error("unknown site found")
	}
}

func TestSearchOrdering(t *testing.T) {
	got := titles(searchStore(t).Search("one piece", nil, 0))
	want := []string{"a.test:One Piece", "b.test:One Piece", "a.test:One Piece: Ace's Story", "a.test:Wan Pisu", "a.test:Someone, Pieces"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestSearchWordsInAnyOrder(t *testing.T) {
	got := titles(searchStore(t).Search("cake piece", nil, 0))
	if len(got) != 1 || got[0] != "a.test:Piece of Cake" {
		t.Fatalf("got %v", got)
	}
}

func TestSearchSiteFilter(t *testing.T) {
	got := titles(searchStore(t).Search("one piece", []string{"b.test"}, 0))
	if len(got) != 1 || got[0] != "b.test:One Piece" {
		t.Fatalf("got %v", got)
	}
}

func TestSearchLimit(t *testing.T) {
	if got := searchStore(t).Search("piece", nil, 2); len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	s := searchStore(t)
	for _, q := range []string{"", "   ", "  -  ", "!!!"} {
		if got := s.Search(q, nil, 0); got != nil {
			t.Errorf("%q: got %v", q, titles(got))
		}
	}
}

func TestSearchAltNames(t *testing.T) {
	got := titles(searchStore(t).Search("ワンピース", nil, 0))
	if len(got) != 1 || got[0] != "a.test:Wan Pisu" {
		t.Fatalf("got %v", got)
	}
}

func TestSearchFoldsAccents(t *testing.T) {
	// every entry has "piece" in its title or another name
	got := titles(searchStore(t).Search("Pièce", nil, 0))
	if len(got) != 6 {
		t.Fatalf("got %v", got)
	}
}
