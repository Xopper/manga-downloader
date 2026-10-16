// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

// packEntries is a site's worth of varied entries: other names, a latest
// chapter, folded-to-nothing and non-Latin titles, more than a page
func packEntries() []Entry {
	entries := []Entry{
		{Title: "One Piece", URL: "https://a.test/manga/one-piece/", Latest: 1120},
		{Title: "Wan Pisu", URL: "https://a.test/manga/wan-pisu/", AltNames: []string{"ワンピース", "One Piece (JP)", "---"}},
		{Title: "---", URL: "https://a.test/manga/dashes/"},
		{Title: "東京喰種", URL: "https://a.test/manga/tokyo-ghoul/", AltNames: []string{"Tokyo Ghoul"}},
		{Title: "", URL: "https://a.test/"},
	}
	for i := 0; i < 3*pageSize; i++ {
		entries = append(entries, Entry{Title: fmt.Sprintf("Título %d", i), URL: fmt.Sprintf("https://a.test/manga/t-%d/", i)})
	}

	return entries
}

func TestPackRoundTrip(t *testing.T) {
	entries := packEntries()
	b := pack("a.test", entries)
	if b.n != len(entries) || len(b.pages) != 4 || b.prefix != "https://a.test/" {
		t.Fatalf("n %d pages %d prefix %q", b.n, len(b.pages), b.prefix)
	}
	for i, want := range entries {
		if got := b.entry(i, b.inflate(i/pageSize)); !reflect.DeepEqual(got, want) {
			t.Errorf("entry %d: got %+v, want %+v", i, got, want)
		}
	}
	if got := b.entries(); !reflect.DeepEqual(got, entries) {
		t.Error("entries() differs from the entries packed")
	}
	// the names arena: folded title, then the folded other names that
	// aren't empty
	names := func(i int) string { return string(b.names[b.nameOff[i]:b.nameOff[i+1]]) }
	if got := names(1); got != "wan pisu\x00ワンピース\x00one piece jp" {
		t.Errorf("names of 1: %q", got)
	}
	if got := names(2); got != "" {
		t.Errorf("names of 2: %q", got)
	}
	if b.latest == nil || b.latest[0] != 1120 || b.latest[1] != 0 {
		t.Errorf("latest: %v", b.latest)
	}
}

func TestPackNoLatestNoPrefix(t *testing.T) {
	b := pack("b.test", []Entry{{Title: "A", URL: "https://b.test/a/"}, {Title: "B", URL: "http://other.test/b/"}})
	if b.latest != nil || b.prefix != "http" {
		t.Errorf("latest %v prefix %q", b.latest, b.prefix)
	}
	if got := b.entries(); got[1].URL != "http://other.test/b/" || got[0].URL != "https://b.test/a/" {
		t.Errorf("got %+v", got)
	}
	if empty := pack("c.test", nil); empty.n != 0 || len(empty.pages) != 0 {
		t.Errorf("empty block: %+v", empty)
	}
}

func TestCatalogIsACopy(t *testing.T) {
	s := searchStore(t)
	c, _ := s.Catalog("a.test")
	c.Entries[0].Title = "changed"
	c.Home = "changed"
	again, _ := s.Catalog("a.test")
	if again.Entries[0].Title == "changed" || again.Home == "changed" {
		t.Error("the caller's changes reached the store")
	}
	if !s.has("A.TEST") || s.has("nope") || s.count("a.test") != 5 || s.count("nope") != 0 {
		t.Error("has/count")
	}
}

func TestMatchWordPrefixes(t *testing.T) {
	cases := []struct {
		name, query string
		want        int
	}{
		{"one piece", "one piece", 400},
		{"one piece ace", "one piece", 300},
		{"someone pieces", "one piece", 100},
		{"piece one", "one piece", 200},
		{"a one", "one", 200},
		{"bone", "one", 100},
		{"one", "one piece", 0},
		{"", "one", 0},
	}
	for _, c := range cases {
		words := [][]byte{}
		for _, w := range splitWords(c.query) {
			words = append(words, []byte(w))
		}
		if got := match([]byte(c.name), []byte(c.query), words); got != c.want {
			t.Errorf("match(%q, %q) = %d, want %d", c.name, c.query, got, c.want)
		}
	}
}

func splitWords(q string) []string {
	var out []string
	for _, w := range []byte(q) {
		if w == ' ' {
			out = append(out, "")
		} else if len(out) == 0 {
			out = append(out, string(w))
		} else {
			out[len(out)-1] += string(w)
		}
	}

	return out
}

func TestBestKeepsTheRankingOfAFullSort(t *testing.T) {
	// a one-letter query matches most entries; the heap must pick the same
	// top of the list as sorting them all would
	var keys []key
	for i := 0; i < 5000; i++ {
		keys = append(keys, key{score: 100 * (1 + i%4), titleLen: 5 + i%37, block: i % 7, i: i})
	}
	full := append([]key(nil), keys...)
	sort.SliceStable(full, func(i, j int) bool { return full[i].before(full[j]) })
	for _, limit := range []int{1, 50, 4999, 5000, 6000} {
		got := best(append([]key(nil), keys...), limit)
		want := full[:min(limit, len(full))]
		if !reflect.DeepEqual(got, want) {
			t.Errorf("limit %d: got %v…, want %v…", limit, got[:3], want[:3])
		}
	}
}

func TestSearchBroadQuery(t *testing.T) {
	s := searchStore(t)
	// "e" is in nearly every name: the limit keeps the shortest titles of
	// the best score (Wan Pisu matches only through another name, 90)
	got := titles(s.Search("e", nil, 2))
	if len(got) != 2 || got[0] != "b.test:Bleach" || got[1] != "a.test:One Piece" {
		t.Fatalf("got %v", got)
	}
}
