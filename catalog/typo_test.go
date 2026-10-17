// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"slices"
	"testing"
)

func typoStore(t *testing.T) *Store {
	t.Helper()
	s := searchStore(t)
	s.BuildTypoIndex()

	return s
}

func TestDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		k    int
		want int
	}{
		{"naruto", "naruto", 1, 0},
		{"naurto", "naruto", 1, 1}, // two letters swapped
		{"bleech", "bleach", 1, 1},
		{"narto", "naruto", 1, 1},
		{"nruto", "naruto", 1, 1},
		{"narutoo", "naruto", 1, 1},
		{"nrauot", "naruto", 1, -1},
		{"jujutsu", "jujutsu", 1, 0},
		{"kaisne", "kaisen", 1, 1},
		{"berserkk", "berserk", 2, 1},
		{"shingeki", "shingiki", 2, 1},
	} {
		if got := distance([]byte(c.a), []byte(c.b), c.k); got != c.want {
			t.Errorf("distance(%q, %q, %d) = %d, want %d", c.a, c.b, c.k, got, c.want)
		}
	}
}

func TestPrefixDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"blea", "bleach", 0},
		{"blee", "bleach", 1},
		{"pieces", "piece", 1},
		{"peice", "pieces", 1},
		{"xyzw", "bleach", -1},
	} {
		if got := prefixDistance([]byte(c.a), []byte(c.b), 1); got != c.want {
			t.Errorf("prefixDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSearchForgivesATypo(t *testing.T) {
	s := typoStore(t)
	for query, want := range map[string]string{
		"one peice": "a.test:One Piece",
		"bleech":    "b.test:Bleach",
		"blech":     "b.test:Bleach",
		"bleahc":    "b.test:Bleach",
	} {
		got := titles(s.Search(query, nil, 0))
		if len(got) == 0 || got[0] != want {
			t.Errorf("Search(%q) = %v, want %s first", query, got, want)
		}
	}
}

func TestSearchFindsAWordWithoutItsSpace(t *testing.T) {
	got := titles(typoStore(t).Search("onepiece", nil, 0))
	if len(got) < 2 || got[0] != "a.test:One Piece" || got[1] != "b.test:One Piece" {
		t.Errorf("got %v", got)
	}
}

func TestExactHitsComeBeforeTypoHits(t *testing.T) {
	hits := typoStore(t).Search("pieces", nil, 0)
	got := titles(hits)
	if len(got) < 2 || got[0] != "a.test:Someone, Pieces" {
		t.Fatalf("got %v, want the exact hit first", got)
	}
	if !slices.Contains(got, "a.test:One Piece") {
		t.Errorf("got %v, want the typo hits after it", got)
	}
	for _, h := range hits[1:] {
		if h.Score >= hits[0].Score {
			t.Errorf("typo hit %s scores %d, exact one %d", h.Entry.Title, h.Score, hits[0].Score)
		}
	}
}

func TestShortWordsAndKanaGetNoTypos(t *testing.T) {
	s := typoStore(t)
	// 3 letters: no typo allowed, so "bne" isn't "one"
	if got := titles(s.Search("bne", nil, 0)); len(got) != 0 {
		t.Errorf("bne: got %v", got)
	}
	if got := titles(s.Search("ワンピーヌ", nil, 0)); len(got) != 0 {
		t.Errorf("kana: got %v", got)
	}
	if got := titles(s.Search("zzzzzz", nil, 0)); len(got) != 0 {
		t.Errorf("zzzzzz: got %v", got)
	}
}

func TestTypoSearchKeepsTheSiteFilter(t *testing.T) {
	got := titles(typoStore(t).Search("one peice", []string{"b.test"}, 0))
	if len(got) != 1 || got[0] != "b.test:One Piece" {
		t.Errorf("got %v", got)
	}
}

// TestTypoIndexFollowsTheStore: a search after a catalog changed starts a
// fresh index, which then finds the new entries
func TestTypoIndexFollowsTheStore(t *testing.T) {
	s := typoStore(t)
	if err := s.Write(&SiteCatalog{Site: "c.test", Entries: []Entry{{Title: "Jujutsu Kaisen", URL: "https://c.test/jjk"}}}); err != nil {
		t.Fatal(err)
	}
	s.BuildTypoIndex()
	if got := titles(s.Search("jujutsu kaisne", nil, 0)); len(got) != 1 || got[0] != "c.test:Jujutsu Kaisen" {
		t.Errorf("got %v", got)
	}
}

// TestATypoInTheRightTitleBeatsWeakExactHits: titles that only contain the
// letters typed don't push the title meant, a typo away, off the list
func TestATypoInTheRightTitleBeatsWeakExactHits(t *testing.T) {
	dir := t.TempDir()
	entries := []Entry{{Title: "Hunter x Hunter", URL: "https://a.test/hxh"}}
	for i := range 60 {
		entries = append(entries, Entry{Title: "Huntrix (Demon Hunters)", URL: "https://a.test/huntrix/" + string(rune('a'+i%26)) + string(rune('a'+i/26))})
	}
	writeCatalog(t, dir, SiteCatalog{Site: "a.test", Entries: entries})
	s := NewStore(dir)
	if warns := s.Load(); len(warns) != 0 {
		t.Fatal(warns)
	}
	s.BuildTypoIndex()
	got := titles(s.Search("hunter x huntr", nil, 0))
	if len(got) == 0 || got[0] != "a.test:Hunter x Hunter" {
		t.Errorf("got %v", got[:min(3, len(got))])
	}
}

func TestAThreeLetterWordAllowsASwap(t *testing.T) {
	s := typoStore(t)
	if got := titles(s.Search("one pi", nil, 0)); len(got) == 0 {
		t.Fatal("no exact hit")
	}
	if got := titles(s.Search("one ipece", nil, 0)); len(got) == 0 || got[0] != "a.test:One Piece" {
		t.Errorf("one ipece: got %v", got)
	}
	if got := titles(s.Search("noe piece", nil, 0)); len(got) == 0 || got[0] != "a.test:One Piece" {
		t.Errorf("noe piece: got %v", got)
	}
}
