// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// FlareSolverr (github.com/FlareSolverr/FlareSolverr) is an optional helper
// for sites behind a Cloudflare challenge: a server running its own Chrome,
// which opens a page, waits the challenge out and hands back the page, the
// clearance cookies and the user agent they're bound to. A request answered
// with the challenge asks it once per site, and goes again with them. When
// that passes, they're kept for the site (SetCookie, SetHostUserAgent) and
// its next requests, pictures included, pass on their own; when it doesn't,
// they're dropped (so the site's other requests aren't spoiled) and the
// site's pages are read through FlareSolverr from then on (proxied). It's
// off ("") until SetFlareSolverr names the server.
var flare = struct {
	sync.Mutex
	addr string
	// solved is when each host was last solved: a host isn't solved again
	// for a while, so a page it still refuses fails instead of looping
	solved map[string]time.Time
	// busy serialises the solves of one host: a chapter's pages ask at once
	busy map[string]*sync.Mutex
	// proxied are the hosts whose clearance doesn't carry over to the
	// downloader's own requests (Cloudflare also checks the TLS handshake):
	// their pages are read through FlareSolverr every time
	proxied map[string]bool
}{solved: map[string]time.Time{}, busy: map[string]*sync.Mutex{}, proxied: map[string]bool{}}

// flareAgain is how long a solved host isn't solved again
const flareAgain = 10 * time.Minute

// flareTimeout is how long FlareSolverr may take on one page
const flareTimeout = 60 * time.Second

// SetFlareSolverr sets the FlareSolverr server's address
// ("http://127.0.0.1:8191"), "" to turn it off
func SetFlareSolverr(addr string) {
	flare.Lock()
	defer flare.Unlock()
	flare.addr = strings.TrimRight(strings.TrimSpace(addr), "/")
}

// FlareSolverr returns the FlareSolverr server's address, "" when it's off
func FlareSolverr() string {
	flare.Lock()
	defer flare.Unlock()
	return flare.addr
}

// isChallenge reports whether a refused response is Cloudflare's challenge
// page (head is the start of its body)
func isChallenge(resp *http.Response, head []byte) bool {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusServiceUnavailable {
		return false
	}
	if resp.Header.Get("Cf-Mitigated") == "challenge" {
		return true
	}
	if !strings.EqualFold(resp.Header.Get("Server"), "cloudflare") {
		return false
	}
	return bytes.Contains(head, []byte("Just a moment")) || bytes.Contains(head, []byte("challenge-platform")) || bytes.Contains(head, []byte("cf_chl"))
}

// flareSolution is the part of FlareSolverr's answer the downloader uses
type flareSolution struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		Status    int    `json:"status"`
		Response  string `json:"response"`
		UserAgent string `json:"userAgent"`
		Cookies   []struct {
			Name   string `json:"name"`
			Value  string `json:"value"`
			Domain string `json:"domain"`
		} `json:"cookies"`
	} `json:"solution"`
}

// errFlareRecent is solve's answer for a host solved moments ago
var errFlareRecent = errors.New("FlareSolverr solved this site moments ago and it still refuses")

// flareCall asks FlareSolverr to open u
func flareCall(u string) (*flareSolution, error) {
	addr := FlareSolverr()
	if addr == "" {
		return nil, errors.New("FlareSolverr is off")
	}
	body, _ := json.Marshal(map[string]any{"cmd": "request.get", "url": u, "maxTimeout": int(flareTimeout / time.Millisecond)})
	client := &http.Client{Timeout: flareTimeout + 15*time.Second}
	resp, err := client.Post(addr+"/v1", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("FlareSolverr at %s: %w", addr, err)
	}
	defer resp.Body.Close()
	var sol flareSolution
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&sol); err != nil {
		return nil, fmt.Errorf("FlareSolverr's answer: %w", err)
	}
	if sol.Status != "ok" {
		return nil, fmt.Errorf("FlareSolverr: %s", sol.Message)
	}
	return &sol, nil
}

// solve has FlareSolverr earn host's clearance on u, and keeps its cookies
// and user agent for host (dropClearance takes them back). Solves of one
// host wait for each other, and a host isn't solved again for flareAgain.
func solve(u, host string) (*flareSolution, error) {
	flare.Lock()
	mu := flare.busy[host]
	if mu == nil {
		mu = &sync.Mutex{}
		flare.busy[host] = mu
	}
	flare.Unlock()

	mu.Lock()
	defer mu.Unlock()
	// a request that waited for another one's solve retries with its cookies
	flare.Lock()
	last := flare.solved[host]
	flare.Unlock()
	if time.Since(last) < flareAgain {
		return nil, errFlareRecent
	}
	sol, err := flareCall(u)
	if err != nil {
		return nil, err
	}
	for _, c := range sol.Solution.Cookies {
		domain := strings.TrimPrefix(c.Domain, ".")
		if domain == "" {
			domain = host
		}
		SetCookie(domain, c.Name, c.Value)
	}
	if sol.Solution.UserAgent != "" {
		SetHostUserAgent(host, sol.Solution.UserAgent)
	}
	flare.Lock()
	flare.solved[host] = time.Now()
	flare.Unlock()

	return sol, nil
}

// dropClearance forgets the user agent and the Cloudflare cookies solve kept
// for host, which the site refused from the downloader anyway, and has its
// pages read through FlareSolverr from now on
func dropClearance(host string) {
	session.Lock()
	for domain := range session.agents {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			delete(session.agents, domain)
		}
	}
	for domain, cookies := range session.cookies {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			delete(cookies, "cf_clearance")
		}
	}
	session.Unlock()
	flare.Lock()
	flare.proxied[host] = true
	flare.Unlock()
}

// isProxied reports whether host's pages are read through FlareSolverr
func isProxied(host string) bool {
	flare.Lock()
	defer flare.Unlock()
	return flare.addr != "" && flare.proxied[host]
}

// jsonViewRe is Chrome's page for a JSON answer: the text inside one <pre>
var jsonViewRe = regexp.MustCompile(`(?s)^<html><head>.*?</head><body><pre[^>]*>(.*)</pre>`)

// flarePage is what FlareSolverr read as the answer to a request: the page,
// or the JSON Chrome showed as one, with its content type; ok is false when
// it read no usable answer (a refusal, a picture: FlareSolverr only gives text)
func flarePage(sol *flareSolution) (body, contentType string, ok bool) {
	if sol == nil || sol.Solution.Status < 200 || sol.Solution.Status > 299 || sol.Solution.Response == "" {
		return "", "", false
	}
	page := sol.Solution.Response
	if m := jsonViewRe.FindStringSubmatch(page); m != nil {
		return html.UnescapeString(m[1]), "application/json", true
	}
	if strings.Contains(page, "<img") && strings.Count(page, "<") < 12 {
		// Chrome's own page around a picture: not something to hand on
		return "", "", false
	}
	return page, "text/html; charset=utf-8", true
}
