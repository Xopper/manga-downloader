// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSuwayomi serves the four endpoints the crawl uses from canned data
type fakeSuwayomi struct {
	mu         sync.Mutex
	extensions []map[string]any
	sources    []map[string]any
	// pages maps "/api/v1/source/<id>/popular/2" (or latest, or
	// "search?searchTerm=&pageNum=2") to its answer
	pages map[string]suwayomiPage
	// fails maps a path to how many 500s it answers before serving
	fails map[string]int
	calls []string
	srv   *httptest.Server
}

type suwayomiPage struct {
	Titles  []string
	HasNext bool
}

func (f *fakeSuwayomi) handle(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	f.mu.Lock()
	f.calls = append(f.calls, path)
	left := f.fails[path]
	if left > 0 {
		f.fails[path] = left - 1
	}
	f.mu.Unlock()
	if left > 0 {
		http.Error(w, "boom", 500)
		return
	}
	switch {
	case path == "/api/v1/extension/list":
		json.NewEncoder(w).Encode(f.extensions)
	case strings.HasPrefix(path, "/api/v1/extension/install/"):
		w.Write([]byte("ok"))
	case path == "/api/v1/source/list":
		json.NewEncoder(w).Encode(f.sources)
	default:
		page, ok := f.pages[path]
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"mangaList": []any{}, "hasNextPage": false})
			return
		}
		list := make([]map[string]any, 0, len(page.Titles))
		for _, t := range page.Titles {
			list = append(list, map[string]any{"url": "/manga/" + slugOf(t), "title": t})
		}
		json.NewEncoder(w).Encode(map[string]any{"mangaList": list, "hasNextPage": page.HasNext})
	}
}

func slugOf(title string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(title), " ", "-"))
}

func (f *fakeSuwayomi) called(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}

	return n
}

func newFakeSuwayomi(t *testing.T) *fakeSuwayomi {
	t.Helper()
	f := &fakeSuwayomi{pages: map[string]suwayomiPage{}, fails: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)

	return f
}

func source(id, name, lang, base string, latest bool) map[string]any {
	return map[string]any{"id": id, "name": name, "lang": lang, "baseUrl": base, "supportsLatest": latest, "isNsfw": false}
}

func titlesN(prefix string, from, n int) []string {
	out := make([]string, 0, n)
	for i := from; i < from+n; i++ {
		out = append(out, fmt.Sprintf("%s %03d", prefix, i))
	}

	return out
}

func crawlOpts(f *fakeSuwayomi, out string) SuwayomiOptions {
	return SuwayomiOptions{Server: f.srv.URL, Langs: []string{"en"}, Out: out, MaxPages: 10, Parallel: 2, Pause: 0}
}

func readCatalog(t *testing.T, dir, site string) *SiteCatalog {
	t.Helper()
	s := NewStore(dir)
	if warns := s.Load(); len(warns) != 0 {
		t.Fatal(warns)
	}
	c, ok := s.Catalog(site)
	if !ok {
		t.Fatalf("%s not written", site)
	}

	return c
}

func TestSuwayomiUnionOfListings(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Alpha", "en", "https://alpha.test", true)}
	// popular pages 1-2 (20 + 20), latest repeats itself (10, then the same
	// 10 again with hasNext), the empty search adds 5 more
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titlesN("Pop", 1, 20), HasNext: true}
	f.pages["/api/v1/source/1/popular/2"] = suwayomiPage{Titles: titlesN("Pop", 21, 20), HasNext: false}
	f.pages["/api/v1/source/1/latest/1"] = suwayomiPage{Titles: titlesN("Late", 1, 10), HasNext: true}
	f.pages["/api/v1/source/1/latest/2"] = suwayomiPage{Titles: titlesN("Late", 1, 10), HasNext: true}
	f.pages["/api/v1/source/1/latest/3"] = suwayomiPage{Titles: titlesN("Late", 1, 10), HasNext: true}
	f.pages["/api/v1/source/1/search?searchTerm=&pageNum=1"] = suwayomiPage{Titles: titlesN("Srch", 1, 5), HasNext: false}
	out := t.TempDir()
	var log bytes.Buffer
	rep, err := CrawlSuwayomi(context.Background(), &log, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, out, "alpha.test")
	if len(c.Entries) != 55 {
		t.Fatalf("got %d entries", len(c.Entries))
	}
	if c.Entries[0].URL != "https://alpha.test/manga/late-001" || c.Entries[0].Title != "Late 001" {
		t.Errorf("entries must be sorted by folded title: %+v", c.Entries[0])
	}
	if c.Source == nil || c.Source.ID != "1" || c.Source.Name != "Alpha" || !c.Source.Complete || c.Language != "en" {
		t.Errorf("source: %+v lang %q", c.Source, c.Language)
	}
	if n := f.called("/api/v1/source/1/latest/"); n != 2 {
		t.Errorf("latest should stop after the first repeated page, fetched %d", n)
	}
	if len(rep.Sources) != 1 || rep.Sources[0].Entries != 55 || rep.Sources[0].Failed || rep.Failed != 0 || len(rep.Written) != 1 {
		t.Errorf("report: %+v", rep)
	}
	if !strings.Contains(log.String(), "alpha.test") || !strings.Contains(log.String(), "55") {
		t.Errorf("log: %s", log.String())
	}
}

func TestSuwayomiBaseURLWithPath(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("7", "Pathy", "en", "https://www.pathy.test/en", false)}
	f.pages["/api/v1/source/7/popular/1"] = suwayomiPage{Titles: []string{"One"}, HasNext: false}
	out := t.TempDir()
	if _, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out)); err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, out, "pathy.test")
	if c.Entries[0].URL != "https://www.pathy.test/en/manga/one" || c.Home != "https://www.pathy.test/en" {
		t.Errorf("got %+v home %s", c.Entries[0], c.Home)
	}
}

func TestSuwayomiMergesSameHost(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{
		source("1", "Alpha", "en", "https://alpha.test", false),
		source("2", "Alpha Mirror", "en", "https://www.alpha.test", false),
	}
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: []string{"B", "C"}, HasNext: false}
	f.pages["/api/v1/source/2/popular/1"] = suwayomiPage{Titles: []string{"C", "D"}, HasNext: false}
	out := t.TempDir()
	existing := NewStore(out)
	if err := existing.Write(&SiteCatalog{Site: "alpha.test", Home: "https://alpha.test", Language: "en", Entries: []Entry{
		{Title: "A", URL: "https://alpha.test/manga/a"}, {Title: "B", URL: "https://alpha.test/manga/b"}, {Title: "Z", URL: "https://alpha.test/manga/z"},
	}}); err != nil {
		t.Fatal(err)
	}
	o := crawlOpts(f, out)
	o.Parallel = 1
	if _, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o); err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, out, "alpha.test")
	var titles []string
	for _, e := range c.Entries {
		titles = append(titles, e.Title)
	}
	if strings.Join(titles, "") != "ABCDZ" {
		t.Fatalf("got %v", titles)
	}
	if c.Home != "https://alpha.test" {
		t.Errorf("home changed: %s", c.Home)
	}
}

func TestSuwayomiShard(t *testing.T) {
	f := newFakeSuwayomi(t)
	for i := 1; i <= 4; i++ {
		f.sources = append(f.sources, source(fmt.Sprint(i), fmt.Sprintf("S%d", i), "en", fmt.Sprintf("https://s%d.test", i), false))
		f.pages[fmt.Sprintf("/api/v1/source/%d/popular/1", i)] = suwayomiPage{Titles: []string{"One"}, HasNext: false}
	}
	out := t.TempDir()
	o := crawlOpts(f, out)
	o.Shard, o.Shards = 3, 4
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Sources) != 1 || rep.Sources[0].Site != "s4.test" {
		t.Fatalf("got %+v", rep.Sources)
	}
	if _, err := os.Stat(filepath.Join(out, "sources-3.json")); err != nil {
		t.Error("shard rows file missing")
	}
}

func TestSuwayomiErrorsInARowStopTheWalk(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Alpha", "en", "https://alpha.test", false)}
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titlesN("Pop", 1, 5), HasNext: true}
	f.pages["/api/v1/source/1/popular/2"] = suwayomiPage{Titles: titlesN("Pop", 6, 5), HasNext: true}
	f.fails["/api/v1/source/1/popular/2"] = 3
	out := t.TempDir()
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, out, "alpha.test")
	if len(c.Entries) != 5 || rep.Sources[0].Failed || rep.Sources[0].Complete {
		t.Fatalf("got %d entries, row %+v", len(c.Entries), rep.Sources[0])
	}
	if n := f.called("/api/v1/source/1/popular/2"); n != 3 {
		t.Errorf("page 2 tried %d times", n)
	}
}

func TestSuwayomiEmptySourceFails(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Empty", "en", "https://empty.test", true)}
	out := t.TempDir()
	var log bytes.Buffer
	rep, err := CrawlSuwayomi(context.Background(), &log, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "empty.test.json")); err == nil {
		t.Error("an empty crawl wrote a file")
	}
	if rep.Failed != 1 || !rep.Sources[0].Failed || !strings.Contains(log.String(), "FAIL") {
		t.Errorf("report %+v log %s", rep, log.String())
	}
}

func TestSuwayomiInstallsMissingExtensions(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.extensions = []map[string]any{
		{"pkgName": "pkg.en.a", "lang": "en", "installed": false},
		{"pkgName": "pkg.all.b", "lang": "all", "installed": false},
		{"pkgName": "pkg.fr.c", "lang": "fr", "installed": false},
		{"pkgName": "pkg.en.d", "lang": "en", "installed": true},
	}
	out := t.TempDir()
	o := crawlOpts(f, out)
	o.Install = true
	if _, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o); err != nil {
		t.Fatal(err)
	}
	if f.called("/api/v1/extension/install/pkg.en.a") != 1 || f.called("/api/v1/extension/install/pkg.all.b") != 1 {
		t.Errorf("missing installs: %v", f.calls)
	}
	if f.called("/api/v1/extension/install/pkg.fr.c") != 0 || f.called("/api/v1/extension/install/pkg.en.d") != 0 {
		t.Errorf("installed what it shouldn't: %v", f.calls)
	}
	o.Install = false
	f.calls = nil
	CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o)
	if f.called("/api/v1/extension/") != 0 {
		t.Error("--no-install still touched extensions")
	}
}

func TestSuwayomiSkipsBadHostAndLocalSource(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{
		source("0", "Local source", "localsourcelang", "", true),
		source("1", "Bad", "en", "https://../x", false),
		source("2", "NoBase", "en", "", false),
		source("3", "Other", "fr", "https://fr.test", false),
	}
	out := t.TempDir()
	var log bytes.Buffer
	rep, err := CrawlSuwayomi(context.Background(), &log, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Sources) != 0 {
		t.Errorf("rows %+v", rep.Sources)
	}
	if f.called("/api/v1/source/1/") != 0 || f.called("/api/v1/source/0/") != 0 || f.called("/api/v1/source/3/") != 0 {
		t.Errorf("walked a skipped source: %v", f.calls)
	}
}

func TestSuwayomiTimeoutStopsNewSources(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{
		source("1", "A", "en", "https://a.test", false),
		source("2", "B", "en", "https://b.test", false),
	}
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: []string{"One"}, HasNext: false}
	f.pages["/api/v1/source/2/popular/1"] = suwayomiPage{Titles: []string{"Two"}, HasNext: false}
	out := t.TempDir()
	o := crawlOpts(f, out)
	o.Parallel = 1
	o.Timeout = time.Nanosecond
	var log bytes.Buffer
	rep, err := CrawlSuwayomi(context.Background(), &log, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Sources) != 2 {
		t.Fatalf("rows %+v", rep.Sources)
	}
	if rep.Sources[0].Failed || rep.Sources[0].Entries != 1 {
		t.Errorf("the first source must still be crawled: %+v", rep.Sources[0])
	}
	if !rep.Sources[1].Failed || !strings.Contains(log.String(), "timeout") || rep.Failed != 1 {
		t.Errorf("the second must be skipped as a timeout: %+v, %s", rep.Sources[1], log.String())
	}
}

func TestSuwayomiSourcesFile(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{
		source("1", "MangaRead", "en", "https://www.mangaread.org", false),
		source("2", "X", "en", "https://x.test", false),
	}
	f.sources[1]["isNsfw"] = true
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: []string{"One"}, HasNext: false}
	f.pages["/api/v1/source/2/popular/1"] = suwayomiPage{Titles: []string{"Two"}, HasNext: false}
	out := t.TempDir()
	if _, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []SourceRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	bysite := map[string]SourceRow{}
	for _, r := range rows {
		bysite[r.Site] = r
	}
	if r := bysite["mangaread.org"]; !r.Downloadable || r.NSFW || r.SourceID != "1" || r.Entries != 1 || r.Name != "MangaRead" {
		t.Errorf("mangaread: %+v", r)
	}
	if r := bysite["x.test"]; r.Downloadable || !r.NSFW || r.SourceID != "2" || r.Lang != "en" {
		t.Errorf("x.test: %+v", r)
	}
}
