// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"bytes"
	"container/heap"
	"runtime"
	"sort"
	"strings"
)

// Hit is a search match
type Hit struct {
	Site  string
	Entry Entry
	// Score ranks the hit: exact title 400, title prefix 300, every query
	// word at the start of a title word 200, every word anywhere 100; a match
	// on another name scores 10 less. A typo hit (typo.go) scores what the
	// corrected query would, less 150 a typo (a word typed without its space
	// counts as one), and 1 at least.
	Score int
}

// DefaultLimit is how many hits Search returns when the caller doesn't say
const DefaultLimit = 50

// key is a match before its entry is rebuilt: enough to rank it
type key struct {
	score    int
	titleLen int
	// block is the index in the store's site-ordered blocks, i the entry's
	// in it: together the entries' order
	block, i int
	// typo is set on a typo hit, whose block is the typo index's
	typo bool
}

// before is the ranking: by score, then shorter title, then site, then
// the entries' own order
func (k key) before(o key) bool {
	if k.score != o.score {
		return k.score > o.score
	}
	if k.titleLen != o.titleLen {
		return k.titleLen < o.titleLen
	}
	if k.block != o.block {
		return k.block < o.block
	}

	return k.i < o.i
}

// Search finds the entries matching every word of query, in the given sites
// (none means all), best first: by score, then shorter title, then site.
// When fewer than limit hits are good ones (every word at a word's start,
// or better), the entries matching with typos or a missing space join in,
// ranked by their own score. A query without a
// letter or digit matches nothing.
func (s *Store) Search(query string, sites []string, limit int) []Hit {
	words := bytes.Fields([]byte(Fold(query)))
	if len(words) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	folded := bytes.Join(words, []byte(" "))
	var only map[string]bool
	if len(sites) > 0 {
		only = make(map[string]bool, len(sites))
		for _, site := range sites {
			only[strings.ToLower(site)] = true
		}
	}

	// the blocks never change once built, so the scan runs without the lock
	s.mu.RLock()
	blocks := s.blocks
	s.mu.RUnlock()
	starts := make([]int, len(blocks)+1)
	for i, b := range blocks {
		starts[i+1] = starts[i] + b.n
	}
	total := starts[len(blocks)]

	// the entries are scanned on every core at once, a slice of them each,
	// and the matches put back in entry order
	chunks := min(runtime.NumCPU(), 8)
	size := (total + chunks - 1) / chunks
	found := make([][]key, chunks)
	inParallel(chunks, func(c int) {
		from, to := c*size, min((c+1)*size, total)
		for bi, b := range blocks {
			lo, hi := max(from, starts[bi]), min(to, starts[bi+1])
			if lo >= hi || (only != nil && !only[b.site]) {
				continue
			}
			for i := lo - starts[bi]; i < hi-starts[bi]; i++ {
				if score := b.score(i, folded, words); score > 0 {
					found[c] = append(found[c], key{score: score, titleLen: int(b.titleLen[i]), block: bi, i: i})
				}
			}
		}
	})
	var keys []key
	for _, f := range found {
		keys = append(keys, f...)
	}
	strong := 0
	for _, k := range keys {
		if k.score >= strongScore {
			strong++
		}
	}
	var idx *typoIndex
	if strong < limit {
		idx = s.typoIndex(blocks)
	}
	if idx == nil {
		return hitsOf(blocks, nil, best(keys, limit))
	}
	exact := make(map[*block]map[int]bool, len(keys))
	for _, k := range keys {
		if exact[blocks[k.block]] == nil {
			exact[blocks[k.block]] = map[int]bool{}
		}
		exact[blocks[k.block]][k.i] = true
	}
	for _, h := range idx.search(words, only, func(b *block, i int) bool { return exact[b][i] }) {
		keys = append(keys, key{score: h.score, titleLen: h.titleLen, block: h.block, i: h.i, typo: true})
	}
	// the index can be a step behind the store (a catalog just updated), so
	// an entry can come both ways: it's listed once all the same
	var hits []Hit
	seen := map[[2]string]bool{}
	for _, h := range hitsOf(blocks, idx.blocks, best(keys, limit+limit/2)) {
		if id := [2]string{h.Site, h.Entry.URL}; !seen[id] && len(hits) < limit {
			seen[id] = true
			hits = append(hits, h)
		}
	}

	return hits
}

// hitsOf rebuilds the entries of keys, from blocks or, for a typo hit,
// typoBlocks; a page is inflated once for every hit on it
func hitsOf(blocks, typoBlocks []*block, keys []key) []Hit {
	if len(keys) == 0 {
		return nil
	}
	pages := map[*block]map[int][]byte{}
	hits := make([]Hit, 0, len(keys))
	for _, k := range keys {
		b := blocks[k.block]
		if k.typo {
			b = typoBlocks[k.block]
		}
		if pages[b] == nil {
			pages[b] = map[int][]byte{}
		}
		data, ok := pages[b][k.i/pageSize]
		if !ok {
			data = b.inflate(k.i / pageSize)
			pages[b][k.i/pageSize] = data
		}
		hits = append(hits, Hit{Site: b.site, Entry: b.entry(k.i, data), Score: k.score})
	}

	return hits
}

// score matches entry i's folded names against the query; 0 is no match
func (b *block) score(i int, query []byte, words [][]byte) int {
	rec := b.names[b.nameOff[i]:b.nameOff[i+1]]
	name, alts := rec, []byte(nil)
	if k := bytes.IndexByte(rec, 0); k >= 0 {
		name, alts = rec[:k], rec[k+1:]
	}
	score := match(name, query, words)
	for len(alts) > 0 {
		alt := alts
		if k := bytes.IndexByte(alts, 0); k >= 0 {
			alt, alts = alts[:k], alts[k+1:]
		} else {
			alts = nil
		}
		if alt := match(alt, query, words) - 10; alt > score {
			score = alt
		}
	}

	return score
}

// best keeps the limit best keys, in ranking order. A one-letter query
// matches most of the store, so the rest is dropped without a full sort.
func best(keys []key, limit int) []key {
	if len(keys) > limit {
		h := &worstFirst{}
		for _, k := range keys {
			if len(*h) < limit {
				heap.Push(h, k)
			} else if k.before((*h)[0]) {
				(*h)[0] = k
				heap.Fix(h, 0)
			}
		}
		keys = *h
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].before(keys[j]) })

	return keys
}

// worstFirst is a heap with the worst-ranked key on top
type worstFirst []key

func (h worstFirst) Len() int           { return len(h) }
func (h worstFirst) Less(i, j int) bool { return h[j].before(h[i]) }
func (h worstFirst) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *worstFirst) Push(x any)        { *h = append(*h, x.(key)) }
func (h *worstFirst) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]

	return x
}

// match scores a folded name against the folded query and its words; 0 is
// no match
func match(name, query []byte, words [][]byte) int {
	switch {
	case bytes.Equal(name, query):
		return 400
	case bytes.HasPrefix(name, query):
		return 300
	}
	for _, w := range words {
		if !bytes.Contains(name, w) {
			return 0
		}
	}
	for _, w := range words {
		if !hasWordPrefix(name, w) {
			return 100
		}
	}

	return 200
}

// hasWordPrefix reports whether a word of the folded name (its words are
// parted by single spaces) starts with w
func hasWordPrefix(name, w []byte) bool {
	for i := 0; i < len(name); i++ {
		if (i == 0 || name[i-1] == ' ') && bytes.HasPrefix(name[i:], w) {
			return true
		}
	}

	return false
}
