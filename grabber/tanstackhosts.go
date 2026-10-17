// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "regexp"

// TanstackHosts serves the sites that run the tanstackComic platform (see
// tanstackcomic.go) under their own domain with nothing to rewrite:
// magustoon.org.
type TanstackHosts struct {
	tanstackComic
}

func NewTanstackHosts(g *Grabber) *TanstackHosts {
	return &TanstackHosts{tanstackComic{Grabber: g}}
}

var tanstackHostsRe = regexp.MustCompile(`^https?://(?:www\.)?magustoon\.org/`)

// Test returns true for a URL on one of those domains
func (t *TanstackHosts) Test() (bool, error) {
	return tanstackHostsRe.MatchString(t.URL), nil
}
