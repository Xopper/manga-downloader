// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	// the page formats a cover can come in (AVIF comes registered through
	// core's packer)
	_ "image/gif"
	_ "image/png"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	mdhttp "github.com/elboletaire/manga-downloader/http"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// A followed series' cover: the image its page announces (og:image, the
// Twitter card, else the first picture in a "cover"/"thumb"/"summary_image"
// box), fetched once in the background, shrunk to coverWidth and kept as a
// JPEG in the covers dir under a name derived from the series URL. The
// window loads it as /covers/<name> through coverHandler. Everything here is
// toolkit-free.

const (
	// coverWidth is the width covers are shrunk to; a card shows ~220px
	coverWidth = 400
	// coverMaxBytes caps the download; a site that answers with something
	// bigger gets no cover
	coverMaxBytes = 5 << 20
	// coverMaxPixels caps what's decoded, so a small file can't claim a
	// huge canvas
	coverMaxPixels = 40 << 20
	// coverTimeout bounds each of the two requests
	coverTimeout = 15 * time.Second
	coverQuality = 85
)

var (
	// errCoverBrowser is a site whose pages need the browser window: the
	// cover is skipped rather than fetched through it
	errCoverBrowser = errors.New("the site needs the browser window")
	errNoCover      = errors.New("no cover on the page")
	// coverNameRe is the only shape a cover file name has: the sha256 of
	// the series' urlKey. Nothing else is served or removed.
	coverNameRe = regexp.MustCompile(`^[0-9a-f]{64}\.jpg$`)
)

// coverFile is the file name of url's cover
func coverFile(rawURL string) string {
	sum := sha256.Sum256([]byte(urlKey(rawURL)))
	return hex.EncodeToString(sum[:]) + ".jpg"
}

// validCoverName reports whether name is a cover file name (and so can't
// leave the covers dir)
func validCoverName(name string) bool {
	return coverNameRe.MatchString(name)
}

// coverURLIn finds the cover's URL in a series page, resolved against base:
// og:image, else twitter:image, else the first <img> inside an element
// whose class mentions cover, thumb or summary_image. "" when none.
func coverURLIn(page io.Reader, base *url.URL) string {
	doc, err := goquery.NewDocumentFromReader(page)
	if err != nil {
		return ""
	}
	candidates := []string{
		doc.Find(`meta[property="og:image"]`).First().AttrOr("content", ""),
		doc.Find(`meta[name="twitter:image"], meta[property="twitter:image"]`).First().AttrOr("content", ""),
	}
	img := doc.Find(`[class*="cover"] img, [class*="thumb"] img, [class*="summary_image"] img`).First()
	// a lazy-loaded picture keeps its real source in a data attribute and
	// a placeholder in src
	for _, attr := range []string{"src", "data-src", "data-lazy-src", "data-cfsrc"} {
		if v := img.AttrOr(attr, ""); v != "" && !strings.HasPrefix(strings.TrimSpace(v), "data:") {
			candidates = append(candidates, v)
			break
		}
	}
	for _, raw := range candidates {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := base.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		return u.String()
	}
	return ""
}

// getter fetches a URL the way the grabbers do; tests swap it
type getter func(mdhttp.RequestParams) (io.ReadCloser, error)

// fetchCoverImage fetches pageURL, finds its cover and downloads it, decoded.
// A site that needs the browser window is skipped with errCoverBrowser.
func fetchCoverImage(pageURL string, get getter, needsVisible func(string) bool) (image.Image, error) {
	if needsVisible(pageURL) {
		return nil, errCoverBrowser
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}
	page, err := get(mdhttp.RequestParams{URL: pageURL, DisableRetry: true, Timeout: coverTimeout})
	if err != nil {
		return nil, err
	}
	html, err := io.ReadAll(io.LimitReader(page, coverMaxBytes))
	page.Close()
	if err != nil {
		return nil, err
	}
	imgURL := coverURLIn(bytes.NewReader(html), base)
	if imgURL == "" {
		return nil, errNoCover
	}
	body, err := get(mdhttp.RequestParams{URL: imgURL, Referer: pageURL, Accept: mdhttp.ImageAccept, DisableRetry: true, Timeout: coverTimeout})
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(body, coverMaxBytes+1))
	body.Close()
	if err != nil {
		return nil, err
	}
	if len(data) > coverMaxBytes {
		return nil, fmt.Errorf("the cover is bigger than %d MB", coverMaxBytes>>20)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("the cover can't be read: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > coverMaxPixels {
		return nil, fmt.Errorf("the cover is %dx%d", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("the cover can't be read: %w", err)
	}
	return img, nil
}

// shrinkCover scales img down to coverWidth wide (never up), on white, so
// a transparent margin doesn't turn black in the JPEG
func shrinkCover(img image.Image) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > coverWidth {
		h = h * coverWidth / w
		w = coverWidth
	}
	if h < 1 {
		h = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
	return dst
}

// coverStore keeps the cover files of a dir and remembers, for this run,
// which series are being fetched and which failed (so a failure is reported
// once, not on every load)
type coverStore struct {
	dir          string
	get          getter
	needsVisible func(string) bool

	mu     sync.Mutex
	busy   map[string]bool
	failed map[string]bool
}

// newCoverStore returns the store of dir, created on the first write
func newCoverStore(dir string) *coverStore {
	return &coverStore{
		dir:          dir,
		get:          func(p mdhttp.RequestParams) (io.ReadCloser, error) { return mdhttp.Get(p) },
		needsVisible: browser.NeedsVisible,
		busy:         map[string]bool{},
		failed:       map[string]bool{},
	}
}

// Path is the file of a cover name, "" for a name that isn't one
func (s *coverStore) Path(name string) string {
	if !validCoverName(name) {
		return ""
	}
	return filepath.Join(s.dir, name)
}

// Fetch downloads the cover of pageURL and writes it to the dir, returning
// the file name
func (s *coverStore) Fetch(pageURL string) (string, error) {
	img, err := fetchCoverImage(pageURL, s.get, s.needsVisible)
	if err != nil {
		return "", err
	}
	name := coverFile(pageURL)
	if err := s.write(name, shrinkCover(img)); err != nil {
		return "", err
	}
	return name, nil
}

// write encodes img as JPEG at name, atomically
func (s *coverStore) write(name string, img image.Image) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(s.dir, name)
	tmp, err := os.CreateTemp(s.dir, name+".*.tmp")
	if err != nil {
		return err
	}
	if err := jpeg.Encode(tmp, img, &jpeg.Options{Quality: coverQuality}); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0600); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Remove deletes the cover file name; a missing file or a name that isn't
// a cover's is a no-op
func (s *coverStore) Remove(name string) error {
	path := s.Path(name)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// begin claims the fetch of pageURL for this goroutine: false when one is
// running, or when it failed earlier this run and retry is false
func (s *coverStore) begin(pageURL string, retry bool) bool {
	key := urlKey(pageURL)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[key] || (s.failed[key] && !retry) {
		return false
	}
	delete(s.failed, key)
	s.busy[key] = true
	return true
}

// end releases the claim, remembering a failure
func (s *coverStore) end(pageURL string, failed bool) {
	key := urlKey(pageURL)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.busy, key)
	if failed {
		s.failed[key] = true
	}
}

// coverHandler serves GET /covers/<name> from dir, read-only: only a cover
// file name is looked up, so nothing outside dir can be reached
func coverHandler(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutPrefix(r.URL.Path, "/covers/")
		if !ok || !validCoverName(name) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		// the name is stable per series, but the file can be refetched
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, name, st.ModTime(), f)
	})
}

// The controller's side: a cover is fetched when a series is followed and
// when a followed series without one is loaded or checked, in the
// background, and never twice at once.

// SetCovers gives the controller the store the covers are kept in; without
// one no cover is fetched
func (c *Controller) SetCovers(s *coverStore) {
	c.mu.Lock()
	c.covers = s
	c.mu.Unlock()
}

// Covers returns the store, nil without one
func (c *Controller) Covers() *coverStore {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.covers
}

// ensureCover fetches url's cover in the background when the series is
// followed without one. retry gives a series whose fetch failed earlier
// this run another go (a fresh Follow); a load or check doesn't.
func (c *Controller) ensureCover(url string, retry bool) {
	if c.Covers() == nil {
		return
	}
	go c.fetchCover(url, retry)
}

// fetchCover is ensureCover's body, on the caller's goroutine. It returns
// once the cover is on disk and in the library, or the fetch was skipped or
// failed (reported in the log once per series and run; a site that needs
// the browser window is skipped silently).
func (c *Controller) fetchCover(url string, retry bool) {
	s := c.Covers()
	lib := c.Library()
	entry, ok := lib.Get(url)
	if s == nil || !ok || entry.Cover != "" {
		return
	}
	if !s.begin(url, retry) {
		return
	}
	name, err := s.Fetch(url)
	s.end(url, err != nil)
	if err != nil {
		if !errors.Is(err, errCoverBrowser) {
			c.addLog(LogInfo, fmt.Sprintf("%s: no cover (%s)", nonEmpty(entry.Title, url), err))
		}
		return
	}
	if err := lib.SetCover(url, name); err != nil {
		c.addLog(LogWarn, "could not save the library: "+err.Error())
	}
	if _, still := lib.Get(url); !still {
		// unfollowed while the cover was on its way
		_ = s.Remove(name)
	}
	c.emit()
}

// FetchMissingCovers fetches, one after the other, the cover of every
// followed series that has none, until ctx ends. It's for startup.
func (c *Controller) FetchMissingCovers(ctx context.Context) {
	if c.Covers() == nil {
		return
	}
	for _, f := range c.Library().List() {
		if ctx.Err() != nil {
			return
		}
		if f.Cover == "" {
			c.fetchCover(f.URL, false)
		}
	}
}
