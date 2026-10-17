// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"errors"
	"net/url"

	"github.com/elboletaire/manga-downloader/http"
)

// bodyRequest is one page a text crawler asks for: a GET, or a form POST
// when Form isn't nil. Referer defaults to the URL's origin; an API on its
// own host (api.luacomic.org) may want the site's.
type bodyRequest struct {
	URL     string
	Referer string
	Form    url.Values
}

// bodyFetcher is how the crawlers that don't read an HTML document (JSON
// feeds and APIs, POSTed result pages) get a page as text. Tests replace it
// with fixtures.
type bodyFetcher func(ctx context.Context, req bodyRequest) (string, error)

var fetchBody bodyFetcher = httpFetchBody

// httpFetchBody is httpFetch without the parsing, POSTing the form if any
func httpFetchBody(ctx context.Context, req bodyRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	params := http.RequestParams{URL: req.URL, Referer: req.Referer, DisableRetry: true}
	if params.Referer == "" {
		params.Referer = homeOf(req.URL)
	}
	if req.Form != nil {
		params.Form = req.Form

		return http.PostText(params)
	}

	return http.GetText(params)
}

// isPastEnd reports whether a fetch failed the way a paged API says there's
// no such page: a 404, or WordPress REST's 400 for a page past the last
func isPastEnd(err error) bool {
	var se *http.StatusError
	if errors.As(err, &se) {
		return se.Code == 404 || se.Code == 400
	}

	return false
}

// bodyReader turns one page's text into its entries (not yet deduped)
type bodyReader func(body, base string) ([]Entry, error)

// walkBodies is walkPages for pages read as text: request(page) is page N's
// request. It stops when a page is missing, lists nothing or adds no series
// not seen before, or after limit pages; a failure on page 1 is returned on
// its own.
func walkBodies(ctx context.Context, limit int, request func(page int) bodyRequest, readPage bodyReader) ([]Entry, error) {
	var entries []Entry
	for page := 1; limit <= 0 || page <= limit; page++ {
		if page > 1 {
			if err := pause(ctx); err != nil {
				return entries, err
			}
		}
		req := request(page)
		body, err := fetchBody(ctx, req)
		if err != nil {
			if page > 1 && ctx.Err() == nil && isPastEnd(err) {
				return entries, nil
			}

			return entries, err
		}
		found, err := readPage(body, req.URL)
		if err != nil {
			return entries, err
		}
		if len(found) == 0 {
			return entries, nil
		}
		before := len(entries)
		entries = dedupe(append(entries, found...))
		if len(entries) == before {
			return entries, nil
		}
	}

	return entries, nil
}
