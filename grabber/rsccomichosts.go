// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "regexp"

// RSCComicHosts serves the sites that run the rscComic platform (see
// rsccomic.go) under their own domain with nothing to rewrite: divascans.org
// and valirscans.org.
type RSCComicHosts struct {
	rscComic
}

func NewRSCComicHosts(g *Grabber) *RSCComicHosts {
	return &RSCComicHosts{rscComic{Grabber: g}}
}

var rscComicHostsRe = regexp.MustCompile(`^https?://(?:www\.)?(?:divascans\.org|valirscans\.org)/`)

// Test returns true for a URL on one of those domains
func (r *RSCComicHosts) Test() (bool, error) {
	return rscComicHostsRe.MatchString(r.URL), nil
}
