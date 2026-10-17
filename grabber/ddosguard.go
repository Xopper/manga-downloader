// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"sync"

	"github.com/elboletaire/manga-downloader/http"
)

// ddosGuardCheckURL is DDoS-Guard's check script, which names the per-visitor
// path a browser then loads from the protected site
var ddosGuardCheckURL = "https://check.ddos-guard.net/check.js"

var (
	ddosGuardPathRe = regexp.MustCompile(`'(/\.well-known/ddos-guard/[^']+)'`)
	ddosGuardMu     sync.Mutex
	ddosGuardDone   = map[string]bool{}
)

// ddosGuardPass gets past DDoS-Guard's passive check for a site, which
// answers 403 until the visitor holds its __ddg2_ cookie: that cookie is set
// by the site's /.well-known/ddos-guard/id/{id} path, the one check.js names.
// The http package keeps the cookie for the site and its subdomains (the
// image hosts), so this runs once per host. A failure is left for the next
// request to report.
func ddosGuardPass(base string) {
	u, err := url.Parse(base)
	if err != nil {
		return
	}
	ddosGuardMu.Lock()
	defer ddosGuardMu.Unlock()
	if ddosGuardDone[u.Host] {
		return
	}

	script, err := http.GetText(http.RequestParams{URL: ddosGuardCheckURL, Accept: "*/*"})
	if err != nil {
		return
	}
	m := ddosGuardPathRe.FindStringSubmatch(script)
	if m == nil {
		return
	}
	if _, err = http.GetText(http.RequestParams{URL: u.Scheme + "://" + u.Host + m[1], Accept: "image/*,*/*"}); err != nil {
		return
	}
	ddosGuardDone[u.Host] = true
}
