// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"sync"
	"testing"

	"github.com/elboletaire/manga-downloader/http"
)

// fakeBodies serves fixture texts by URL (a POST's key is the URL, a space
// and its encoded form) and 404s anything else, recording every request
type fakeBodies struct {
	mu    sync.Mutex
	pages map[string]string
	codes map[string]int
	calls []bodyRequest
}

func (f *fakeBodies) fetch(_ context.Context, req bodyRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	key := req.URL
	if req.Form != nil {
		key += " " + req.Form.Encode()
	}
	if code, ok := f.codes[key]; ok {
		return "", &http.StatusError{Code: code}
	}
	page, ok := f.pages[key]
	if !ok {
		return "", &http.StatusError{Code: 404}
	}

	return page, nil
}

// withFakeBodies replaces the text fetcher for the test and disables pacing
func withFakeBodies(t *testing.T, pages map[string]string) *fakeBodies {
	t.Helper()
	f := &fakeBodies{pages: pages, codes: map[string]int{}}
	prevFetch, prevPause := fetchBody, pagePause
	fetchBody, pagePause = f.fetch, 0
	t.Cleanup(func() { fetchBody, pagePause = prevFetch, prevPause })

	return f
}
