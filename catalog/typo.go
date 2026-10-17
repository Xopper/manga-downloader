// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"bytes"
	"slices"
	"sort"
	"sync/atomic"
)

// The typo search: what Search adds when the exact match finds fewer good
// hits (every word at the start of a word, or better) than asked. Every word of every name is listed once (the vocabulary), with
// the entries it's in. A query word that isn't in the list, or is the last
// one and still being typed, is looked up there with typos allowed, the way
// Typesense counts them: 1 for a word of 4 to 7 letters, 2 for a longer one,
// a swap of two letters counting as one. A word typed without its space
// ("onepiece") is split into two listed words. A typo costs a hit 150 of
// its score: a title right but for one typo (250) ranks after the exact
// titles and title starts, before the hits that only contain the words.

const (
	// typoMinLen is the shortest word a typo is allowed in
	typoMinLen = 4
	// typoTwoLen is the shortest word two typos are allowed in
	typoTwoLen = 8
	// typoMaxCandidates bounds how many entries the typo search checks: a
	// query whose rarest word is in more entries than that is left to the
	// exact search
	typoMaxCandidates = 200000
	// typoMaxWordLen is the longest word the vocabulary looks typos up in
	typoMaxWordLen = 40
	// typoCost is what a typo takes off a hit's score
	typoCost = 150
	// strongScore is the score of a good exact hit: when fewer than asked
	// score that much, the typo search runs
	strongScore = 200
)

// typoIndex is the vocabulary of a set of blocks. It keeps the blocks it
// was built from, so a search keeps using it, unchanged, while the store
// loads new catalogs and a fresh one is built.
type typoIndex struct {
	blocks []*block
	// starts[b] is the global number of block b's first entry
	starts []int
	words  []string
	ids    map[string]uint32
	// postings[off[w]:off[w+1]] is the global numbers of the entries word w
	// is in, ascending
	postings []uint32
	off      []uint32
	// byLen[n] is the ids of the words of n bytes, for the typo lookup
	byLen [typoMaxWordLen + typoTwoLen][]uint32
}

// eachWord calls fn with every word of entry i's names (title and others)
func (b *block) eachWord(i int, fn func(w []byte)) {
	rec := b.names[b.nameOff[i]:b.nameOff[i+1]]
	start := 0
	for j := 0; j <= len(rec); j++ {
		if j == len(rec) || rec[j] == ' ' || rec[j] == 0 {
			if j > start {
				fn(rec[start:j])
			}
			start = j + 1
		}
	}
}

// buildTypoIndex lists the words of blocks
func buildTypoIndex(blocks []*block) *typoIndex {
	idx := &typoIndex{blocks: blocks, starts: make([]int, len(blocks)+1), ids: map[string]uint32{}}
	for i, b := range blocks {
		idx.starts[i+1] = idx.starts[i] + b.n
	}
	// first pass: the words, and how many entries each is in
	var counts []uint32
	var last []uint32 // the last entry counted for each word, so an entry counts once
	g := uint32(0)
	for _, b := range blocks {
		for i := 0; i < b.n; i++ {
			b.eachWord(i, func(w []byte) {
				id, ok := idx.ids[string(w)]
				if !ok {
					id = uint32(len(idx.words))
					idx.ids[string(w)] = id
					idx.words = append(idx.words, string(w))
					counts = append(counts, 0)
					last = append(last, ^uint32(0))
				}
				if last[id] != g {
					last[id] = g
					counts[id]++
				}
			})
			g++
		}
	}
	// second pass: the entries of each word
	idx.off = make([]uint32, len(idx.words)+1)
	for id, n := range counts {
		idx.off[id+1] = idx.off[id] + n
	}
	idx.postings = make([]uint32, idx.off[len(idx.words)])
	fill := slices.Clone(idx.off[:len(idx.words)])
	for id := range last {
		last[id] = ^uint32(0)
	}
	g = 0
	for _, b := range blocks {
		for i := 0; i < b.n; i++ {
			b.eachWord(i, func(w []byte) {
				id := idx.ids[string(w)]
				if last[id] != g {
					last[id] = g
					idx.postings[fill[id]] = g
					fill[id]++
				}
			})
			g++
		}
	}
	for id, w := range idx.words {
		if n := len(w); n < len(idx.byLen) && typoEligible([]byte(w)) {
			idx.byLen[n] = append(idx.byLen[n], uint32(id))
		}
	}

	return idx
}

// typoEligible reports whether a typo is looked for in w: a word of plain
// letters and digits (a typo in a kana or hanzi word isn't one) that isn't
// only digits
func typoEligible(w []byte) bool {
	digits := true
	for _, c := range w {
		if c >= 0x80 {
			return false
		}
		if c < '0' || c > '9' {
			digits = false
		}
	}

	return !digits
}

// maxTypos is how many typos a query word may have
func maxTypos(w []byte) int {
	switch {
	case len(w) < typoMinLen || len(w) > typoMaxWordLen || !typoEligible(w):
		return 0
	case len(w) < typoTwoLen:
		return 1
	}

	return 2
}

// variant is a listed word standing for a query word, d typos away
type variant struct {
	id uint32
	d  uint8
}

// variants lists the words that stand for w: itself when listed, and the
// ones within its typos. The last word of a query is being typed, so the
// listed words starting with it, or within its typos of it, stand for it too.
func (idx *typoIndex) variants(w []byte, last bool) []variant {
	var out []variant
	k := maxTypos(w)
	if last {
		// the listed words it starts, typos aside
		for id, word := range idx.words {
			if len(word) >= len(w) && word[:len(w)] == string(w) {
				out = append(out, variant{id: uint32(id)})
			}
		}
	} else if id, ok := idx.ids[string(w)]; ok {
		out = append(out, variant{id: id})
	}
	if k == 0 {
		if len(w) == 3 && typoEligible(w) {
			out = append(out, idx.swaps(w, last)...)
		}
		return out
	}
	seen := make(map[uint32]bool, len(out))
	for _, v := range out {
		seen[v.id] = true
	}
	lo, hi := max(1, len(w)-k), min(len(idx.byLen)-1, len(w)+k)
	if last {
		hi = len(idx.byLen) - 1
	}
	// the words of the lengths in reach, looked at on every core at once
	type task struct{ ids []uint32 }
	var tasks []task
	for n := lo; n <= hi; n++ {
		for ids := idx.byLen[n]; len(ids) > 0; {
			c := min(len(ids), 8192)
			tasks = append(tasks, task{ids[:c]})
			ids = ids[c:]
		}
	}
	found := make([][]variant, len(tasks))
	inParallel(len(tasks), func(t int) {
		for _, id := range tasks[t].ids {
			if seen[id] {
				continue
			}
			word := []byte(idx.words[id])
			d := -1
			if last {
				d = prefixDistance(w, word, k)
			} else {
				d = distance(w, word, k)
			}
			if d >= 0 {
				found[t] = append(found[t], variant{id: id, d: uint8(d)})
			}
		}
	})
	for _, f := range found {
		out = append(out, f...)
	}

	return out
}

// swaps is the one typo a word of three letters is allowed: two neighbours
// swapped ("mna" for "man"), anything more would match most short words.
// The last word of a query stands for the listed words starting with it.
func (idx *typoIndex) swaps(w []byte, last bool) []variant {
	var out []variant
	for i := 0; i+1 < len(w); i++ {
		sw := []byte(string(w))
		sw[i], sw[i+1] = sw[i+1], sw[i]
		if sw[i] == sw[i+1] {
			continue
		}
		if !last {
			if id, ok := idx.ids[string(sw)]; ok {
				out = append(out, variant{id: id, d: 1})
			}
			continue
		}
		for n := len(sw); n < len(idx.byLen); n++ {
			for _, id := range idx.byLen[n] {
				if word := idx.words[id]; word[:len(sw)] == string(sw) {
					out = append(out, variant{id: id, d: 1})
				}
			}
		}
	}

	return out
}

// distance is the edit distance between a and b (insertions, deletions,
// substitutions and swaps of two neighbours, one each), or -1 when it's
// over k
func distance(a, b []byte, k int) int {
	if d := len(a) - len(b); d > k || -d > k {
		return -1
	}
	last := osa(a, b, k)
	if last == nil || last[len(b)] > k {
		return -1
	}

	return last[len(b)]
}

// prefixDistance is the least distance between a and a start of b (b cut
// anywhere from len(a)-k letters on), or -1 when it's over k
func prefixDistance(a, b []byte, k int) int {
	if len(b) > len(a)+k {
		b = b[:len(a)+k]
	}
	if len(b) < len(a)-k {
		return -1
	}
	last := osa(a, b, k)
	if last == nil {
		return -1
	}
	best := -1
	for j := max(0, len(a)-k); j <= len(b); j++ {
		if v := last[j]; v <= k && (best < 0 || v < best) {
			best = v
		}
	}

	return best
}

// osa returns the last row of the optimal string alignment table of a
// against b, nil once a row is all over k (no cell below can be within k).
// The words are short (typoMaxWordLen), so the rows live on the stack.
func osa(a, b []byte, k int) []int {
	var rows [3][typoMaxWordLen + typoTwoLen + 1]int
	prev2, prev, cur := rows[0][:len(b)+1], rows[1][:len(b)+1], rows[2][:len(b)+1]
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		low := i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			low = min(low, v)
		}
		if low > k {
			return nil
		}
		prev2, prev, cur = prev, cur, prev2
	}
	out := make([]int, len(b)+1)
	copy(out, prev)

	return out
}

// typoWord is a query word and what stands for it
type typoWord struct {
	w        []byte
	variants map[uint32]uint8
	// size is how many entries its variants are in, with repeats
	size int
}

// typoQueries turns the query's words into the typo searches to run: the
// words as typed, and the words with one written without its space split
// in two (a split costs one typo). penalty is what each search adds.
func (idx *typoIndex) typoQueries(words [][]byte) (queries [][]typoWord, penalties []int) {
	build := func(ws [][]byte) []typoWord {
		out := make([]typoWord, len(ws))
		for i, w := range ws {
			tw := typoWord{w: w, variants: map[uint32]uint8{}}
			for _, v := range idx.variants(w, i == len(ws)-1) {
				tw.variants[v.id] = v.d
				tw.size += int(idx.off[v.id+1] - idx.off[v.id])
			}
			out[i] = tw
		}

		return out
	}
	queries = append(queries, build(words))
	penalties = append(penalties, 0)
	for i, w := range words {
		// a listed word is split all the same: "onepiece" is a word of some
		// titles, and still One Piece typed without its space
		if len(w) < 2*2 || !typoEligible(w) {
			continue
		}
		splits := 0
		for cut := 2; cut <= len(w)-2 && splits < 2; cut++ {
			_, left := idx.ids[string(w[:cut])]
			_, right := idx.ids[string(w[cut:])]
			if !left || !right {
				continue
			}
			ws := slices.Concat(words[:i], [][]byte{w[:cut], w[cut:]}, words[i+1:])
			queries = append(queries, build(ws))
			penalties = append(penalties, 1)
			splits++
		}
	}

	return queries, penalties
}

// typoHit is a typo search's match, before its entry is rebuilt
type typoHit struct {
	score    int
	titleLen int
	block, i int
}

// search runs the typo searches of words over the index's blocks, sites
// limited to only when it's set; skip says which entries the exact search
// already found
func (idx *typoIndex) search(words [][]byte, only map[string]bool, skip func(b *block, i int) bool) []typoHit {
	queries, penalties := idx.typoQueries(words)
	best := map[uint32]typoHit{}
	for q, query := range queries {
		// the rarest word drives: its entries are the candidates
		driver := -1
		for i, tw := range query {
			if tw.size == 0 {
				driver = -1
				break
			}
			if driver < 0 || tw.size < query[driver].size {
				driver = i
			}
		}
		if driver < 0 || query[driver].size > typoMaxCandidates {
			continue
		}
		var candidates []uint32
		for id := range query[driver].variants {
			candidates = append(candidates, idx.postings[idx.off[id]:idx.off[id+1]]...)
		}
		slices.Sort(candidates)
		candidates = slices.Compact(candidates)
		for _, g := range candidates {
			bi := sort.Search(len(idx.blocks), func(b int) bool { return idx.starts[b+1] > int(g) }) //nolint:gosec
			b, i := idx.blocks[bi], int(g)-idx.starts[bi]
			if only != nil && !only[b.site] {
				continue
			}
			score, typos := idx.scoreEntry(b, i, query)
			if score <= 0 || typos+penalties[q] == 0 || skip(b, i) {
				continue
			}
			// how well the corrected query matches, less 150 a typo
			score = max(1, score-typoCost*(typos+penalties[q]))
			if prev, ok := best[g]; !ok || score > prev.score {
				best[g] = typoHit{score: score, titleLen: int(b.titleLen[i]), block: bi, i: i}
			}
		}
	}
	out := make([]typoHit, 0, len(best))
	for _, h := range best {
		out = append(out, h)
	}

	return out
}

// scoreEntry matches entry i's names against the typo query: the best
// name's match score with every query word replaced by what stands for it
// there, and the typos that took. 0 is no match.
func (idx *typoIndex) scoreEntry(b *block, i int, query []typoWord) (score, typos int) {
	rec := b.names[b.nameOff[i]:b.nameOff[i+1]]
	alt := 0
	for len(rec) > 0 {
		name := rec
		if k := bytes.IndexByte(rec, 0); k >= 0 {
			name, rec = rec[:k], rec[k+1:]
		} else {
			rec = nil
		}
		if s, t := idx.scoreName(name, query); s > 0 {
			s -= alt
			if score == 0 || t < typos || (t == typos && s > score) {
				score, typos = s, t
			}
		}
		alt = 10
	}

	return score, typos
}

// scoreName is scoreEntry for one name
func (idx *typoIndex) scoreName(name []byte, query []typoWord) (score, typos int) {
	corrected := make([][]byte, len(query))
	for qi, tw := range query {
		if bytes.Contains(name, tw.w) {
			corrected[qi] = tw.w
			continue
		}
		bestD := -1
		start := 0
		for j := 0; j <= len(name); j++ {
			if j < len(name) && name[j] != ' ' {
				continue
			}
			if id, ok := idx.ids[string(name[start:j])]; ok {
				if d, ok := tw.variants[id]; ok && (bestD < 0 || int(d) < bestD) {
					bestD = int(d)
					corrected[qi] = name[start:j]
				}
			}
			start = j + 1
		}
		if bestD < 0 {
			return 0, 0
		}
		typos += bestD
	}

	return match(name, bytes.Join(corrected, []byte(" ")), corrected), typos
}

// typoState is the store's typo index and whether one is being built
type typoState struct {
	idx      atomic.Pointer[typoIndex]
	building atomic.Bool
}

// BuildTypoIndex lists the words of the catalogs loaded now, for the typo
// search, and returns once it's done (a second or two for two million
// entries). Search starts it in the background when the catalogs changed,
// using the last index meanwhile; calling it is for a search that can't wait.
func (s *Store) BuildTypoIndex() {
	s.mu.RLock()
	blocks := s.blocks
	s.mu.RUnlock()
	if cur := s.typo.idx.Load(); cur != nil && sameBlocks(cur.blocks, blocks) {
		return
	}
	s.typo.idx.Store(buildTypoIndex(blocks))
}

// typoIndex returns the index to search with, starting a fresh one in the
// background when the catalogs changed since it was built; nil while there's
// none yet
func (s *Store) typoIndex(blocks []*block) *typoIndex {
	cur := s.typo.idx.Load()
	if (cur == nil || !sameBlocks(cur.blocks, blocks)) && s.typo.building.CompareAndSwap(false, true) {
		go func() {
			defer s.typo.building.Store(false)
			s.BuildTypoIndex()
		}()
	}

	return cur
}

// sameBlocks reports whether two block lists are the same blocks
func sameBlocks(a, b []*block) bool {
	return slices.Equal(a, b)
}
