// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// a site that answers Cloudflare's challenge until it gets the clearance
// cookie with FlareSolverr's user agent
func challengeSite(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var refused atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("cf_clearance")
		if err != nil || c.Value != "ok" || r.UserAgent() != "SolverChrome/1" {
			refused.Add(1)
			w.Header().Set("Server", "cloudflare")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "<html><title>Just a moment...</title></html>")
			return
		}
		_, _ = io.WriteString(w, "<html><h1>the series</h1></html>")
	}))
	t.Cleanup(srv.Close)
	return srv, &refused
}

// a FlareSolverr that hands out the clearance cookie for the site's host
func fakeSolver(t *testing.T, calls *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct{ Cmd, URL string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		u, _ := url.Parse(req.URL)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status":    200,
				"response":  "<html><h1>the series, as Chrome read it</h1></html>",
				"userAgent": "SolverChrome/1",
				"cookies":   []map[string]string{{"name": "cf_clearance", "value": "ok", "domain": u.Hostname()}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func resetFlare(t *testing.T) {
	t.Cleanup(func() {
		SetFlareSolverr("")
		flare.Lock()
		flare.solved = map[string]time.Time{}
		flare.Unlock()
		session.Lock()
		session.cookies = map[string]map[string]string{}
		session.agents = map[string]string{}
		session.Unlock()
	})
}

func TestFlareSolverrEarnsTheClearanceOnce(t *testing.T) {
	resetFlare(t)
	site, refused := challengeSite(t)
	var calls atomic.Int32
	SetFlareSolverr(fakeSolver(t, &calls).URL + "/")

	for i := 0; i < 2; i++ {
		text, err := GetText(RequestParams{URL: site.URL + "/series", DisableRetry: true})
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if !strings.Contains(text, "<h1>the series</h1>") {
			t.Errorf("request %d got %q, want the site's own page", i, text)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("FlareSolverr was asked %d times, want once", calls.Load())
	}
	if refused.Load() != 1 {
		t.Errorf("the site refused %d requests, want only the first", refused.Load())
	}
}

func TestFlareSolverrOffLeavesTheChallengeAnError(t *testing.T) {
	resetFlare(t)
	site, _ := challengeSite(t)
	_, err := GetText(RequestParams{URL: site.URL + "/series", DisableRetry: true})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Fatalf("err = %v, want the 403", err)
	}
}

func TestIsChallenge(t *testing.T) {
	cases := []struct {
		status int
		header map[string]string
		body   string
		want   bool
	}{
		{403, map[string]string{"Server": "cloudflare"}, "<title>Just a moment...</title>", true},
		{503, map[string]string{"Cf-Mitigated": "challenge"}, "", true},
		{403, map[string]string{"Server": "nginx"}, "Just a moment", false},
		{404, map[string]string{"Server": "cloudflare"}, "Just a moment", false},
		{403, map[string]string{"Server": "cloudflare"}, "Access denied", false},
	}
	for _, c := range cases {
		resp := &http.Response{StatusCode: c.status, Header: http.Header{}}
		for k, v := range c.header {
			resp.Header.Set(k, v)
		}
		if got := isChallenge(resp, []byte(c.body)); got != c.want {
			t.Errorf("isChallenge(%d, %v, %q) = %v, want %v", c.status, c.header, c.body, got, c.want)
		}
	}
}

// a site whose clearance doesn't carry over to the downloader (Cloudflare also
// checks the TLS handshake) is read through FlareSolverr, and its user agent
// and cookies don't spoil the site's other requests
func TestFlareSolverrReadsASiteThatStillRefuses(t *testing.T) {
	resetFlare(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "<html><title>Just a moment...</title></html>")
	}))
	t.Cleanup(site.Close)
	var calls atomic.Int32
	SetFlareSolverr(fakeSolver(t, &calls).URL)
	t.Cleanup(func() {
		flare.Lock()
		flare.proxied = map[string]bool{}
		flare.Unlock()
	})

	for i := 0; i < 2; i++ {
		text, err := GetText(RequestParams{URL: site.URL + "/series", DisableRetry: true})
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if !strings.Contains(text, "as Chrome read it") {
			t.Errorf("request %d got %q, want FlareSolverr's page", i, text)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("FlareSolverr was asked %d times, want once per page", calls.Load())
	}
	u, _ := url.Parse(site.URL)
	if ua := sessionUserAgentFor(u.Hostname(), "own"); ua != "own" {
		t.Errorf("the site's user agent stayed %q", ua)
	}
	// a picture isn't something FlareSolverr can hand on: it's asked for directly
	if _, err := GetText(RequestParams{URL: site.URL + "/1.jpg", Accept: ImageAccept, DisableRetry: true}); err == nil {
		t.Error("a refused picture came back as FlareSolverr's page")
	}
}

func TestFlarePageUnwrapsJSON(t *testing.T) {
	sol := &flareSolution{}
	sol.Solution.Status = 200
	sol.Solution.Response = `<html><head><meta name="color-scheme" content="light dark"></head><body><pre style="word-wrap: break-word; white-space: pre-wrap;">{"a":"&lt;b&gt;"}</pre></body></html>`
	body, ct, ok := flarePage(sol)
	if !ok || ct != "application/json" || body != `{"a":"<b>"}` {
		t.Errorf("flarePage = %q, %q, %v", body, ct, ok)
	}
}
