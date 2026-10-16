// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"io"
	"sync"
)

// pageSize is how many entries a block's page holds. A hit inflates its
// page, so a page is small; the titles and links still deflate to a third
// by the page.
const pageSize = 256

// block is a site's entries packed for searching. The folded names sit in
// one byte arena the scan walks; the titles, links and other names sit in
// deflated pages, inflated only for a hit. Held this way, a million and a
// half entries take a fifth of what they took as Entry values with an
// index beside them. A block never changes once built: a write swaps in a
// new one, so a search reads it without the store's lock.
type block struct {
	site string
	n    int
	// names holds every entry's folded title, then its folded other names
	// (the empty ones left out), parted by a zero byte;
	// names[nameOff[i]:nameOff[i+1]] is entry i's
	names   []byte
	nameOff []uint32
	// titleLen is every title's length in bytes, what ranks hits of one
	// score, so a search ranks without inflating anything
	titleLen []uint16
	// pages holds the titles, links and other names, pageSize entries a
	// page; prefix is the part every link of the site starts with, kept
	// once
	pages  []page
	prefix string
	// latest is every entry's Latest, nil when they're all zero
	latest []float64
}

// page is pageSize entries' titles, links and other names, deflated
type page struct {
	data []byte
	// size is the inflated length
	size int
}

var deflaters = sync.Pool{New: func() any {
	w, _ := flate.NewWriter(io.Discard, flate.BestSpeed)
	return w
}}

var inflaters = sync.Pool{New: func() any { return flate.NewReader(bytes.NewReader(nil)) }}

// pack folds and packs a site's entries
func pack(site string, entries []Entry) *block {
	b := &block{site: site, n: len(entries), titleLen: make([]uint16, len(entries))}
	for i, e := range entries {
		if i == 0 {
			b.prefix = e.URL
		} else {
			b.prefix = commonPrefix(b.prefix, e.URL)
		}
	}
	names := make([]byte, 0, 32*len(entries))
	b.nameOff = make([]uint32, 1, len(entries)+1)
	w := deflaters.Get().(*flate.Writer)
	defer deflaters.Put(w)
	var fields []byte
	for i, e := range entries {
		names = append(names, Fold(e.Title)...)
		for _, alt := range e.AltNames {
			if f := Fold(alt); f != "" {
				names = append(names, 0)
				names = append(names, f...)
			}
		}
		b.nameOff = append(b.nameOff, uint32(len(names)))
		b.titleLen[i] = uint16(min(len(e.Title), 1<<16-1))
		if e.Latest != 0 {
			if b.latest == nil {
				b.latest = make([]float64, len(entries))
			}
			b.latest[i] = e.Latest
		}
		fields = appendField(fields, e.Title)
		fields = appendField(fields, e.URL[len(b.prefix):])
		fields = binary.AppendUvarint(fields, uint64(len(e.AltNames)))
		for _, alt := range e.AltNames {
			fields = appendField(fields, alt)
		}
		if (i+1)%pageSize == 0 || i == len(entries)-1 {
			b.pages = append(b.pages, deflate(w, fields))
			fields = fields[:0]
		}
	}
	b.names = bytes.Clone(names)

	return b
}

// commonPrefix is the longest prefix a and b share
func commonPrefix(a, b string) string {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}

	return a[:n]
}

// appendField adds a length-prefixed string
func appendField(p []byte, s string) []byte {
	p = binary.AppendUvarint(p, uint64(len(s)))

	return append(p, s...)
}

// nextField reads a length-prefixed field and returns what follows it
func nextField(p []byte) (field, rest []byte) {
	n, k := binary.Uvarint(p)
	p = p[k:]

	return p[:n], p[n:]
}

func deflate(w *flate.Writer, p []byte) page {
	var buf bytes.Buffer
	w.Reset(&buf)
	w.Write(p)
	w.Close()

	return page{data: bytes.Clone(buf.Bytes()), size: len(p)}
}

// inflate returns a page's content
func (b *block) inflate(p int) []byte {
	r := inflaters.Get().(io.ReadCloser)
	defer inflaters.Put(r)
	r.(flate.Resetter).Reset(bytes.NewReader(b.pages[p].data), nil)
	out := make([]byte, b.pages[p].size)
	io.ReadFull(r, out)

	return out
}

// entry rebuilds entry i from its inflated page
func (b *block) entry(i int, pageData []byte) Entry {
	p := pageData
	for k := i % pageSize; k > 0; k-- {
		p = skipRecord(p)
	}
	e, _ := b.record(i, p)

	return e
}

// record rebuilds entry i from its record at p and returns what follows
func (b *block) record(i int, p []byte) (Entry, []byte) {
	var e Entry
	title, p := nextField(p)
	e.Title = string(title)
	link, p := nextField(p)
	e.URL = b.prefix + string(link)
	alts, n := binary.Uvarint(p)
	p = p[n:]
	if alts > 0 {
		e.AltNames = make([]string, 0, alts)
		for ; alts > 0; alts-- {
			var alt []byte
			alt, p = nextField(p)
			e.AltNames = append(e.AltNames, string(alt))
		}
	}
	if b.latest != nil {
		e.Latest = b.latest[i]
	}

	return e, p
}

// skipRecord returns what follows the record at p
func skipRecord(p []byte) []byte {
	_, p = nextField(p)
	_, p = nextField(p)
	alts, n := binary.Uvarint(p)
	p = p[n:]
	for ; alts > 0; alts-- {
		_, p = nextField(p)
	}

	return p
}

// entries rebuilds every entry, in order: what Catalog hands out
func (b *block) entries() []Entry {
	out := make([]Entry, 0, b.n)
	for p := range b.pages {
		data := b.inflate(p)
		for i := p * pageSize; i < min((p+1)*pageSize, b.n); i++ {
			var e Entry
			e, data = b.record(i, data)
			out = append(out, e)
		}
	}

	return out
}
