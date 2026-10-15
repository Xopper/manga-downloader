// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"sort"
	"strings"
)

// Hit is a search match
type Hit struct {
	Site  string
	Entry Entry
	// Score ranks the hit: exact title 400, title prefix 300, every query
	// word at the start of a title word 200, every word anywhere 100; a match
	// on another name scores 10 less
	Score int
}

// defaultLimit is how many hits Search returns when the caller doesn't say
const defaultLimit = 50

// Search finds the entries matching every word of query, in the given sites
// (none means all), best first: by score, then shorter title, then site.
// A query without a letter or digit matches nothing.
func (s *Store) Search(query string, sites []string, limit int) []Hit {
	words := strings.Fields(Fold(query))
	if len(words) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	folded := strings.Join(words, " ")
	var only map[string]bool
	if len(sites) > 0 {
		only = make(map[string]bool, len(sites))
		for _, site := range sites {
			only[strings.ToLower(site)] = true
		}
	}

	// a write lock: the index may need rebuilding after writes
	s.mu.Lock()
	if s.dirty {
		s.reindex()
		s.dirty = false
	}
	var hits []Hit
	for _, ix := range s.entries {
		if only != nil && !only[ix.site] {
			continue
		}
		score := match(ix.title, folded, words)
		for _, alt := range ix.alts {
			if alt := match(alt, folded, words) - 10; alt > score {
				score = alt
			}
		}
		if score > 0 {
			hits = append(hits, Hit{Site: ix.site, Entry: ix.entry, Score: score})
		}
	}
	s.mu.Unlock()

	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if len(a.Entry.Title) != len(b.Entry.Title) {
			return len(a.Entry.Title) < len(b.Entry.Title)
		}

		return a.Site < b.Site
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}

	return hits
}

// match scores a folded name against the folded query and its words; 0 is
// no match
func match(name, query string, words []string) int {
	switch {
	case name == query:
		return 400
	case strings.HasPrefix(name, query):
		return 300
	}
	nameWords := strings.Fields(name)
	prefixes := true
	for _, w := range words {
		if !strings.Contains(name, w) {
			return 0
		}
		if prefixes && !hasWordPrefix(nameWords, w) {
			prefixes = false
		}
	}
	if prefixes {
		return 200
	}

	return 100
}

// hasWordPrefix reports whether a word of the name starts with w
func hasWordPrefix(nameWords []string, w string) bool {
	for _, n := range nameWords {
		if strings.HasPrefix(n, w) {
			return true
		}
	}

	return false
}
