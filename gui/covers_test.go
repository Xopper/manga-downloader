// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// coverPNG is a w×h PNG, red on a transparent ground
func coverPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w/2; x++ {
			img.Set(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// coverSite is a fake series site: a page at /manga/x and whatever files
// the test puts in files, by path
type coverSite struct {
	page  string
	files map[string][]byte
	// hits counts the requests by path
	hits map[string]int
}

func newCoverSite(t *testing.T, page string, files map[string][]byte) (*httptest.Server, *coverSite) {
	t.Helper()
	site := &coverSite{page: page, files: files, hits: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site.hits[r.URL.Path]++
		if r.URL.Path == "/manga/x" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(site.page))
			return
		}
		if data, ok := site.files[r.URL.Path]; ok {
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, site
}

// testCovers is a store in a temp dir that treats no site as browser-gated
func testCovers(t *testing.T) *coverStore {
	t.Helper()
	s := newCoverStore(filepath.Join(t.TempDir(), "covers"))
	s.needsVisible = func(string) bool { return false }
	return s
}

// readJPEG decodes the cover file name of s
func readJPEG(t *testing.T, s *coverStore, name string) image.Image {
	t.Helper()
	f, err := os.Open(filepath.Join(s.dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("the cover isn't a JPEG: %v", err)
	}
	return img
}

func TestCoverFileFollowsTheURLKey(t *testing.T) {
	a := coverFile("https://www.example.com/manga/x/")
	b := coverFile("https://example.com/manga/x")
	if a != b {
		t.Errorf("two spellings of one series got %s and %s", a, b)
	}
	if !validCoverName(a) {
		t.Errorf("%s isn't a valid cover name", a)
	}
	if coverFile("https://example.com/manga/y") == a {
		t.Error("two series share a file")
	}
}

func TestFetchCoverTakesOgImageAndShrinksIt(t *testing.T) {
	srv, site := newCoverSite(t, `<html><head><meta property="og:image" content="/img/cover.png"></head><body></body></html>`,
		map[string][]byte{"/img/cover.png": coverPNG(t, 800, 1200)})
	s := testCovers(t)
	name, err := s.Fetch(srv.URL + "/manga/x")
	if err != nil {
		t.Fatal(err)
	}
	if name != coverFile(srv.URL+"/manga/x") {
		t.Errorf("name = %s", name)
	}
	img := readJPEG(t, s, name)
	if b := img.Bounds(); b.Dx() != coverWidth || b.Dy() != 600 {
		t.Errorf("cover is %v, want 400x600", b)
	}
	// the transparent half is white, not black
	if r, g, b, _ := img.At(coverWidth-1, 1).RGBA(); r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Errorf("the transparent margin came out %v", color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255})
	}
	if site.hits["/img/cover.png"] != 1 {
		t.Errorf("the cover was fetched %d times", site.hits["/img/cover.png"])
	}
	st, err := os.Stat(filepath.Join(s.dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0600 {
		t.Errorf("cover mode = %v", st.Mode().Perm())
	}
}

func TestFetchCoverFallsBackToTheCoverBox(t *testing.T) {
	page := `<html><head><meta name="twitter:image" content=""></head><body>
	<div class="site-logo"><img src="/logo.png"></div>
	<div class="summary_image"><a><img src="data:image/gif;base64,R0lGOD" data-src="//HOST/img/lazy.png"></a></div>
	</body></html>`
	srv, site := newCoverSite(t, "", map[string][]byte{"/img/lazy.png": coverPNG(t, 200, 300), "/logo.png": coverPNG(t, 50, 50)})
	site.page = strings.ReplaceAll(page, "HOST", strings.TrimPrefix(srv.URL, "http://"))
	s := testCovers(t)
	name, err := s.Fetch(srv.URL + "/manga/x")
	if err != nil {
		t.Fatal(err)
	}
	if b := readJPEG(t, s, name).Bounds(); b.Dx() != 200 || b.Dy() != 300 {
		t.Errorf("a small cover was resized to %v", b)
	}
	if site.hits["/logo.png"] != 0 {
		t.Error("the logo was taken for the cover")
	}
}

func TestFetchCoverWithNoneOnThePage(t *testing.T) {
	srv, _ := newCoverSite(t, `<html><body><img src="/x.png"></body></html>`, nil)
	s := testCovers(t)
	if _, err := s.Fetch(srv.URL + "/manga/x"); !errors.Is(err, errNoCover) {
		t.Errorf("err = %v", err)
	}
	if entries, _ := os.ReadDir(s.dir); len(entries) != 0 {
		t.Errorf("files were written: %v", entries)
	}
}

func TestFetchCoverRefusesABigFile(t *testing.T) {
	big := make([]byte, coverMaxBytes+1)
	copy(big, coverPNG(t, 10, 10))
	srv, _ := newCoverSite(t, `<html><head><meta property="og:image" content="/big.png"></head></html>`, map[string][]byte{"/big.png": big})
	s := testCovers(t)
	_, err := s.Fetch(srv.URL + "/manga/x")
	if err == nil || !strings.Contains(err.Error(), "bigger than 5 MB") {
		t.Errorf("err = %v", err)
	}
}

func TestFetchCoverReadsWebP(t *testing.T) {
	srv, _ := newCoverSite(t, `<html><head><meta property="og:image" content="/c.webp"></head></html>`, map[string][]byte{"/c.webp": tinyWebP})
	s := testCovers(t)
	name, err := s.Fetch(srv.URL + "/manga/x")
	if err != nil {
		t.Fatal(err)
	}
	if b := readJPEG(t, s, name).Bounds(); b.Dx() == 0 || b.Dy() == 0 {
		t.Errorf("cover is %v", b)
	}
}

func TestFetchCoverSkipsABrowserSite(t *testing.T) {
	srv, site := newCoverSite(t, `<html><head><meta property="og:image" content="/c.png"></head></html>`, map[string][]byte{"/c.png": coverPNG(t, 10, 10)})
	s := testCovers(t)
	s.needsVisible = func(string) bool { return true }
	if _, err := s.Fetch(srv.URL + "/manga/x"); !errors.Is(err, errCoverBrowser) {
		t.Errorf("err = %v", err)
	}
	if len(site.hits) != 0 {
		t.Errorf("the site was fetched: %v", site.hits)
	}
}

func TestCoverHandlerServesOnlyCoverNames(t *testing.T) {
	dir := t.TempDir()
	name := coverFile("https://example.com/manga/x")
	if err := os.WriteFile(filepath.Join(dir, name), []byte("jpegbytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("no"), 0600); err != nil {
		t.Fatal(err)
	}
	h := coverHandler(dir)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	if rec := get("/covers/" + name); rec.Code != 200 || rec.Body.String() != "jpegbytes" || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("a cover: %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	for _, path := range []string{
		"/covers/secret.txt",
		"/covers/../secret.txt",
		"/covers/../" + name,
		"/covers/" + strings.Repeat("0", 64) + ".jpg", // a valid name with no file
		"/covers/" + strings.ToUpper(name),
		"/" + name,
		"/covers/",
	} {
		if rec := get(path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/covers/"+name, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestFollowedDTOCarriesTheCoverPath(t *testing.T) {
	name := coverFile("https://example.com/manga/x")
	if got := followedDTO(Followed{URL: "https://example.com/manga/x", Cover: name}, "", "", nil).Cover; got != "/covers/"+name {
		t.Errorf("Cover = %q", got)
	}
	if got := followedDTO(Followed{URL: "https://example.com/manga/x"}, "", "", nil).Cover; got != "" {
		t.Errorf("Cover without one = %q", got)
	}
}

func TestLibraryKeepsTheCoverOnARefollow(t *testing.T) {
	l := NewLibrary(filepath.Join(t.TempDir(), "library.json"))
	url := "https://example.com/manga/x"
	if err := l.Follow(Followed{URL: url}); err != nil {
		t.Fatal(err)
	}
	if err := l.SetCover(url, "abc.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := l.Follow(Followed{URL: url, Title: "X"}); err != nil {
		t.Fatal(err)
	}
	if f, _ := l.Get(url); f.Cover != "abc.jpg" || f.Title != "X" {
		t.Errorf("entry = %+v", f)
	}
	again := NewLibrary(l.path)
	if err := again.Load(); err != nil {
		t.Fatal(err)
	}
	if f, _ := again.Get(url); f.Cover != "abc.jpg" {
		t.Errorf("reloaded entry = %+v", f)
	}
}

func TestFollowFetchesTheCoverAndUnfollowRemovesIt(t *testing.T) {
	srv, _ := newCoverSite(t, `<html><head><meta property="og:image" content="/c.png"></head></html>`, map[string][]byte{"/c.png": coverPNG(t, 20, 30)})
	a, _, _ := newTestApp(t, 3)
	s := testCovers(t)
	a.ctrl.SetCovers(s)
	url := srv.URL + "/manga/x"
	if err := a.Follow(FollowedDTO{URL: url, Title: "X"}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the cover", func() bool {
		f, _ := a.ctrl.Library().Get(url)
		return f.Cover != ""
	})
	f, _ := a.ctrl.Library().Get(url)
	path := filepath.Join(s.dir, f.Cover)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if got := a.State().Library.Followed[0].Cover; got != "/covers/"+f.Cover {
		t.Errorf("DTO cover = %q", got)
	}
	if err := a.Unfollow(url); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the cover file is still there: %v", err)
	}
}

func TestACoverThatFailsIsReportedOnce(t *testing.T) {
	srv, site := newCoverSite(t, `<html><body>nothing</body></html>`, nil)
	a, _, _ := newTestApp(t, 3)
	s := testCovers(t)
	a.ctrl.SetCovers(s)
	url := srv.URL + "/manga/x"
	if err := a.ctrl.Library().Follow(Followed{URL: url, Title: "X"}); err != nil {
		t.Fatal(err)
	}
	// a load or check of a followed series without a cover tries once per run
	a.ctrl.fetchCover(url, false)
	a.ctrl.fetchCover(url, false)
	if site.hits["/manga/x"] != 1 {
		t.Errorf("the page was fetched %d times", site.hits["/manga/x"])
	}
	n := 0
	for _, line := range a.ctrl.Log() {
		if strings.Contains(line.Text, "no cover") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d log lines about the cover: %+v", n, a.ctrl.Log())
	}
	// a fresh Follow tries again
	a.ctrl.fetchCover(url, true)
	if site.hits["/manga/x"] != 2 {
		t.Errorf("a re-follow didn't retry: %d", site.hits["/manga/x"])
	}
}

var tinyWebP = []byte{
	0x52, 0x49, 0x46, 0x46, 0xb2, 0x01, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x4c,
	0xa5, 0x01, 0x00, 0x00, 0x2f, 0x4a, 0xc0, 0x18, 0x00, 0x0f, 0x30, 0xff, 0xf3, 0x3f, 0xff, 0xf3,
	0x1f, 0x78, 0x90, 0x24, 0x6d, 0x7b, 0xda, 0x48, 0x6e, 0xe6, 0xf1, 0x0d, 0xc6, 0x7d, 0x84, 0x81,
	0x25, 0xe9, 0x30, 0x43, 0x3b, 0x66, 0xfc, 0x87, 0x19, 0x96, 0x0c, 0x27, 0x99, 0x62, 0x26, 0x9f,
	0x60, 0x4a, 0xed, 0xa1, 0x66, 0x06, 0xd9, 0xd5, 0x8a, 0xbe, 0xaa, 0xff, 0xff, 0x15, 0x3a, 0x41,
	0x44, 0xff, 0x19, 0xb8, 0x6d, 0xa4, 0xc8, 0xbb, 0xc7, 0x38, 0xf0, 0x0a, 0xc4, 0xa3, 0xaf, 0x81,
	0xdf, 0x31, 0x4a, 0x62, 0x59, 0xf7, 0xa6, 0xa0, 0xa5, 0x48, 0x22, 0x97, 0xd1, 0xb7, 0xa0, 0x15,
	0x30, 0x17, 0x14, 0xe2, 0xd7, 0x1d, 0x2c, 0x85, 0xf1, 0xc0, 0x8d, 0x71, 0x91, 0x06, 0xe0, 0xec,
	0xb0, 0xb8, 0x0e, 0x0a, 0x55, 0x57, 0xc9, 0x0a, 0x20, 0x2b, 0x53, 0xb1, 0x80, 0x80, 0x92, 0x3c,
	0xfa, 0x52, 0x4f, 0xfc, 0xe2, 0x8c, 0x4f, 0xf7, 0xc1, 0x02, 0x37, 0xaf, 0x83, 0x57, 0x18, 0x07,
	0xb6, 0x15, 0x90, 0x5b, 0x96, 0x81, 0xad, 0xa5, 0xc8, 0xf8, 0xb9, 0x23, 0x41, 0xc5, 0xcb, 0x96,
	0x13, 0xa5, 0x62, 0x07, 0x83, 0x44, 0x59, 0xa6, 0x49, 0xe2, 0x45, 0x55, 0xbd, 0xa1, 0xd1, 0xc0,
	0x28, 0xec, 0x28, 0xb1, 0x6b, 0x8e, 0x19, 0xdc, 0x48, 0xca, 0x7d, 0x8e, 0xbd, 0xa0, 0x83, 0xbe,
	0x18, 0x3f, 0xc1, 0xee, 0x93, 0xc1, 0xa7, 0x4f, 0x04, 0xf6, 0xea, 0x05, 0x5e, 0x7c, 0x32, 0xc2,
	0xe6, 0x30, 0x9f, 0x32, 0x66, 0x73, 0x96, 0x93, 0xc4, 0x91, 0xcf, 0x83, 0x7e, 0x42, 0x8c, 0x8f,
	0x2f, 0xe3, 0x27, 0x6a, 0x6c, 0xcc, 0xbd, 0xc1, 0x35, 0xac, 0x73, 0x44, 0xaf, 0xdd, 0x45, 0xf4,
	0x62, 0x99, 0x3d, 0x55, 0x1c, 0x4b, 0xdc, 0x3b, 0x3e, 0x18, 0x47, 0xdf, 0xab, 0x2e, 0x07, 0xda,
	0x8f, 0x79, 0x86, 0xff, 0xa0, 0xb9, 0x3a, 0x72, 0xe4, 0xe2, 0x27, 0x4c, 0x0e, 0x2b, 0x79, 0xb9,
	0x87, 0x57, 0x0a, 0x8d, 0x6e, 0x84, 0x55, 0x90, 0x98, 0x30, 0xae, 0xdd, 0xc5, 0xc2, 0x82, 0x05,
	0xd8, 0x0f, 0xf4, 0x79, 0x0a, 0xaf, 0xd8, 0x24, 0x00, 0xed, 0x8f, 0xf0, 0x62, 0x99, 0x19, 0x65,
	0x5d, 0x20, 0x06, 0xad, 0x41, 0xaf, 0xb5, 0x20, 0x3a, 0x6d, 0xea, 0xac, 0xa8, 0xad, 0x5c, 0x1d,
	0xcb, 0x4d, 0x71, 0x75, 0x6f, 0x09, 0x91, 0xf9, 0x3a, 0xc6, 0x31, 0x17, 0x99, 0x54, 0x10, 0xf8,
	0x74, 0x1d, 0x16, 0xbe, 0x8e, 0x2a, 0x12, 0x0d, 0xdf, 0x87, 0x57, 0x5a, 0xad, 0x3e, 0xd2, 0xaa,
	0xfa, 0x10, 0x94, 0x82, 0x79, 0xe5, 0x4b, 0x1f, 0xdf, 0xa0, 0xbc, 0x64, 0xcb, 0xca, 0xa3, 0x3a,
	0xe4, 0xf4, 0x38, 0xe2, 0x28, 0x73, 0x95, 0x35, 0xf1, 0x40, 0xa8, 0xca, 0x6c, 0x0b, 0xec, 0x85,
	0x78, 0x22, 0xaf, 0xb2, 0xe2, 0x97, 0xdc, 0x38, 0x2f, 0x66, 0xef, 0x33, 0x27, 0x26, 0x8d, 0x07,
	0x2a, 0x5d, 0xa3, 0x02, 0x3b, 0xa0, 0x65, 0x63, 0x6f, 0x22, 0xf8, 0x53, 0x8b, 0xcd, 0xb7, 0xc8,
	0xd6, 0xf1, 0x2a, 0xc4, 0x08, 0x68, 0xb6, 0x87, 0x00, 0x00,
}
