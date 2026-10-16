// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"bytes"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// PageMemory is how long a fetched page is remembered, so the same page asked
// for again within that time is answered from memory instead of fetched
// again. Identifying a site and then fetching its title and chapter list asks
// for the series page several times in a row; with the memory on, the site
// is asked once. Zero (the default, and what the tests run with) remembers
// nothing. ResetPageMemory forgets everything, which a load does at its start
// so a reload fetches.
var PageMemory time.Duration

// pageMemoMaxEntry is the largest page remembered; a bigger one streams
// through as it did
const pageMemoMaxEntry = 4 << 20

// pageMemoMaxTotal bounds what's held in all
const pageMemoMaxTotal = 48 << 20

// pageMemoKey is what has to match for a remembered page to stand in: only
// plain GETs of documents (the default Accept, no form, no body) are
// remembered, so a page image or a POST never is
type pageMemoKey struct {
	url, referer, headers string
}

type pageMemoEntry struct {
	body []byte
	at   time.Time
}

var pageMemo = struct {
	sync.Mutex
	pages map[pageMemoKey]pageMemoEntry
	total int
}{pages: map[pageMemoKey]pageMemoEntry{}}

// ResetPageMemory forgets every remembered page
func ResetPageMemory() {
	pageMemo.Lock()
	pageMemo.pages = map[pageMemoKey]pageMemoEntry{}
	pageMemo.total = 0
	pageMemo.Unlock()
}

// memoKey returns the key a request is remembered under, and whether it's
// one that is
func memoKey(method string, rp RequestParams) (pageMemoKey, bool) {
	if PageMemory <= 0 || method != "GET" || rp.Form != nil || rp.Body != "" || rp.Accept != "" {
		return pageMemoKey{}, false
	}
	var headers []string
	for k, v := range rp.Headers {
		headers = append(headers, k+":"+v)
	}
	sort.Strings(headers)
	return pageMemoKey{url: rp.URL, referer: rp.Referer, headers: strings.Join(headers, "\n")}, true
}

// rememberedPage returns the page remembered under key, if it's still fresh
func rememberedPage(key pageMemoKey) (io.ReadCloser, bool) {
	pageMemo.Lock()
	defer pageMemo.Unlock()
	e, ok := pageMemo.pages[key]
	if !ok {
		return nil, false
	}
	if time.Since(e.at) > PageMemory {
		delete(pageMemo.pages, key)
		pageMemo.total -= len(e.body)
		return nil, false
	}
	return io.NopCloser(bytes.NewReader(e.body)), true
}

// rememberPage reads body so it can be remembered under key, and returns a
// reader of the same bytes. A page past pageMemoMaxEntry isn't remembered:
// what was read is handed back in front of the rest.
func rememberPage(key pageMemoKey, body io.ReadCloser) io.ReadCloser {
	buf, err := io.ReadAll(io.LimitReader(body, pageMemoMaxEntry+1))
	if err != nil {
		body.Close()
		return io.NopCloser(bytes.NewReader(buf))
	}
	if len(buf) > pageMemoMaxEntry {
		return &joinedBody{Reader: io.MultiReader(bytes.NewReader(buf), body), rest: body}
	}
	body.Close()

	pageMemo.Lock()
	if old, ok := pageMemo.pages[key]; ok {
		pageMemo.total -= len(old.body)
	}
	pageMemo.pages[key] = pageMemoEntry{body: buf, at: time.Now()}
	pageMemo.total += len(buf)
	for pageMemo.total > pageMemoMaxTotal {
		// the oldest goes first
		var oldest pageMemoKey
		var oldestAt time.Time
		for k, e := range pageMemo.pages {
			if oldestAt.IsZero() || e.at.Before(oldestAt) {
				oldest, oldestAt = k, e.at
			}
		}
		pageMemo.total -= len(pageMemo.pages[oldest].body)
		delete(pageMemo.pages, oldest)
	}
	pageMemo.Unlock()

	return io.NopCloser(bytes.NewReader(buf))
}

// joinedBody is a body too big to remember: what was read, then the rest
type joinedBody struct {
	io.Reader
	rest io.Closer
}

func (j *joinedBody) Close() error { return j.rest.Close() }
