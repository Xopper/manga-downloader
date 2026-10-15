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
	// slow maps a path to how long it sleeps before answering
	slow map[string]time.Duration
	// realURL maps a title to the real URL its details answer; a title
	// missing here answers no realUrl (the join is right for it)
	realURL map[string]string
	ids     map[string]int
	titles  map[int]string
	// bases remembers which source listed a manga, so its details can
	// answer the joined link as the real one by default
	bases map[int]string
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
	wait := f.slow[path]
	f.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
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
	case strings.HasPrefix(path, "/api/v1/manga/"):
		var id int
		fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/api/v1/manga/"), "%d", &id)
		f.mu.Lock()
		title := f.titles[id]
		real := f.realURL[title]
		base := f.bases[id]
		f.mu.Unlock()
		if title == "" {
			http.Error(w, "no such manga", 500)
			return
		}
		m := map[string]any{"id": id, "url": "/manga/" + slugOf(title), "title": title}
		if real == "" {
			// the join is right for this source
			real = strings.TrimSuffix(base, "/") + "/manga/" + slugOf(title)
		}
		m["realUrl"] = real
		json.NewEncoder(w).Encode(m)
	default:
		page, ok := f.pages[path]
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"mangaList": []any{}, "hasNextPage": false})
			return
		}
		var base string
		if parts := strings.Split(path, "/"); len(parts) > 4 {
			f.mu.Lock()
			for _, src := range f.sources {
				if src["id"] == parts[4] {
					base, _ = src["baseUrl"].(string)
				}
			}
			f.mu.Unlock()
		}
		list := make([]map[string]any, 0, len(page.Titles))
		for _, t := range page.Titles {
			id := f.idOf(t)
			f.mu.Lock()
			f.bases[id] = base
			f.mu.Unlock()
			list = append(list, map[string]any{"id": id, "url": "/manga/" + slugOf(t), "title": t})
		}
		json.NewEncoder(w).Encode(map[string]any{"mangaList": list, "hasNextPage": page.HasNext})
	}
}

// idOf gives a title a stable manga id, as Suwayomi's database would
func (f *fakeSuwayomi) idOf(title string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.ids[title]; ok {
		return id
	}
	id := len(f.ids) + 1
	f.ids[title] = id
	f.titles[id] = title

	return id
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
	f := &fakeSuwayomi{pages: map[string]suwayomiPage{}, fails: map[string]int{}, realURL: map[string]string{}, ids: map[string]int{}, titles: map[int]string{}, bases: map[int]string{}}
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
		{Title: "A", URL: "https://alpha.test/manga/a"}, {Title: "B", URL: "https://alpha.test/manga/b", Latest: 42}, {Title: "Z", URL: "https://alpha.test/manga/z"},
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
	if c.Entries[1].Latest != 42 {
		t.Errorf("a source that lists no chapter number must keep the crawler's: %+v", c.Entries[1])
	}
	if c.Home != "https://alpha.test" {
		t.Errorf("home changed: %s", c.Home)
	}
}

func TestSuwayomiShardWritesItsRowsFile(t *testing.T) {
	// a shard crawls a subset (by host hash) and names its rows file after
	// itself, even when nothing fell into it
	f := newFakeSuwayomi(t)
	for i := 1; i <= 4; i++ {
		f.sources = append(f.sources, source(fmt.Sprint(i), fmt.Sprintf("S%d", i), "en", fmt.Sprintf("https://s%d.test", i), false))
		f.pages[fmt.Sprintf("/api/v1/source/%d/popular/1", i)] = suwayomiPage{Titles: []string{"One"}, HasNext: false}
	}
	total := 0
	for shard := 0; shard < 4; shard++ {
		out := t.TempDir()
		o := crawlOpts(f, out)
		o.Shard, o.Shards = shard, 4
		rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o)
		if err != nil {
			t.Fatal(err)
		}
		total += len(rep.Sources)
		if _, err := os.Stat(filepath.Join(out, fmt.Sprintf("sources-%d.json", shard))); err != nil {
			t.Errorf("shard %d rows file missing", shard)
		}
		if len(rep.Sources) == 4 {
			t.Errorf("shard %d took every source", shard)
		}
	}
	if total != 4 {
		t.Errorf("the shards crawled %d sources in all, want 4", total)
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

func TestSuwayomiTimeoutSkipsSourcesWaitingForASlot(t *testing.T) {
	// the first source takes 300ms; with one slot and a 100ms timeout the
	// two behind it must be skipped, not started once the slot frees
	f := newFakeSuwayomi(t)
	for i := 1; i <= 3; i++ {
		f.sources = append(f.sources, source(fmt.Sprint(i), fmt.Sprintf("S%d", i), "en", fmt.Sprintf("https://s%d.test", i), false))
		f.pages[fmt.Sprintf("/api/v1/source/%d/popular/1", i)] = suwayomiPage{Titles: []string{"One"}, HasNext: false}
	}
	f.slow = map[string]time.Duration{"/api/v1/source/1/popular/1": 300 * time.Millisecond}
	out := t.TempDir()
	o := crawlOpts(f, out)
	o.Parallel = 1
	o.Timeout = 100 * time.Millisecond
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Written) != 1 || rep.Failed != 2 {
		t.Fatalf("written %v failed %d: %+v", rep.Written, rep.Failed, rep.Sources)
	}
}

func TestSuwayomiHungRequestEnds(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Hang", "en", "https://hang.test", false)}
	f.slow = map[string]time.Duration{"/api/v1/source/1/popular/1": 5 * time.Second}
	out := t.TempDir()
	o := crawlOpts(f, out)
	o.RequestTimeout = 150 * time.Millisecond
	done := make(chan *SuwayomiReport, 1)
	go func() {
		rep, _ := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o)
		done <- rep
	}()
	select {
	case rep := <-done:
		if rep.Failed != 1 {
			t.Errorf("a hung source must fail: %+v", rep.Sources)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a hung request never ended the crawl")
	}
}

func TestSuwayomiShardsKeepAHostTogether(t *testing.T) {
	// two sources on one host must land in the same shard (their files are
	// merged inside one process), and every source in exactly one shard
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{
		source("1", "Alpha", "en", "https://alpha.test", false),
		source("2", "Alpha Mirror", "en", "https://www.alpha.test", false),
		source("3", "Beta", "en", "https://beta.test", false),
		source("4", "Gamma", "en", "https://gamma.test", false),
		source("5", "Delta", "en", "https://delta.test", false),
	}
	for i := 1; i <= 5; i++ {
		f.pages[fmt.Sprintf("/api/v1/source/%d/popular/1", i)] = suwayomiPage{Titles: []string{"One"}, HasNext: false}
	}
	seen := map[string]int{}
	shardOf := map[string]int{}
	for shard := 0; shard < 3; shard++ {
		o := crawlOpts(f, t.TempDir())
		o.Shard, o.Shards = shard, 3
		rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rep.Sources {
			seen[r.SourceID]++
			shardOf[r.SourceID] = shard
		}
	}
	for i := 1; i <= 5; i++ {
		if seen[fmt.Sprint(i)] != 1 {
			t.Errorf("source %d crawled %d times", i, seen[fmt.Sprint(i)])
		}
	}
	if shardOf["1"] != shardOf["2"] {
		t.Errorf("one host split over shards %d and %d", shardOf["1"], shardOf["2"])
	}
}

func TestSuwayomiUnchangedSetLeavesTheFileAlone(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Alpha", "en", "https://alpha.test", false)}
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: []string{"A", "B"}, HasNext: false}
	out := t.TempDir()
	if _, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out)); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(out, "alpha.test.json"))
	time.Sleep(10 * time.Millisecond)
	if _, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out)); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(out, "alpha.test.json"))
	if !bytes.Equal(first, second) {
		t.Fatalf("the same set of titles rewrote the file:\n%s\n%s", first, second)
	}
	// a new title does change it
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: []string{"A", "B", "C"}, HasNext: false}
	CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out))
	third, _ := os.ReadFile(filepath.Join(out, "alpha.test.json"))
	if bytes.Equal(second, third) {
		t.Fatal("a new title must change the file")
	}
}

// realFor marks a source's titles as needing a fetched link: their real URL
// is the site's own form, not base + path
func realFor(f *fakeSuwayomi, base string, titles ...string) {
	for _, t := range titles {
		f.realURL[t] = base + "/series/" + slugOf(t) + "-x1"
	}
}

func TestSuwayomiJoinedLinksAreCheckedOnce(t *testing.T) {
	// a source whose join is right: one details fetch to check it, none after
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Alpha", "en", "https://alpha.test", false)}
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titlesN("Pop", 1, 20), HasNext: false}
	for _, title := range titlesN("Pop", 1, 20) {
		f.realURL[title] = "https://alpha.test/manga/" + slugOf(title)
	}
	out := t.TempDir()
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	if n := f.called("/api/v1/manga/"); n != 1 {
		t.Errorf("details fetched %d times, want the one check", n)
	}
	c := readCatalog(t, out, "alpha.test")
	if len(c.Entries) != 20 || c.Entries[0].URL != "https://alpha.test/manga/pop-001" {
		t.Errorf("got %d entries, first %+v", len(c.Entries), c.Entries[0])
	}
	if rep.Sources[0].Links != "joined" {
		t.Errorf("row: %+v", rep.Sources[0])
	}
}

func TestSuwayomiFetchesRealLinksWhenTheJoinIsWrong(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Lily", "en", "https://lily.test", false)}
	titles := titlesN("Lily", 1, 6)
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titles, HasNext: false}
	realFor(f, "https://lily.test", titles...)
	out := t.TempDir()
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, out, "lily.test")
	if len(c.Entries) != 6 {
		t.Fatalf("got %d entries", len(c.Entries))
	}
	for _, e := range c.Entries {
		if !strings.HasSuffix(e.URL, "-x1") || !strings.Contains(e.URL, "/series/") {
			t.Errorf("entry kept the joined link: %+v", e)
		}
	}
	if rep.Sources[0].Links != "fetched" || rep.Sources[0].Entries != 6 {
		t.Errorf("row: %+v", rep.Sources[0])
	}
}

func TestSuwayomiFetchBudgetDropsUnverifiedLinks(t *testing.T) {
	// only MaxFetch titles get their real link per run; the rest are left
	// out rather than stored with a wrong link, and a later run adds more
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Lily", "en", "https://lily.test", false)}
	titles := titlesN("Lily", 1, 10)
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titles, HasNext: false}
	realFor(f, "https://lily.test", titles...)
	out := t.TempDir()
	o := crawlOpts(f, out)
	o.MaxFetch = 4
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o)
	if err != nil {
		t.Fatal(err)
	}
	// the checked title plus four fetched links
	c := readCatalog(t, out, "lily.test")
	if len(c.Entries) != 5 || rep.Sources[0].Complete {
		t.Fatalf("got %d entries, row %+v", len(c.Entries), rep.Sources[0])
	}
	// the next run skips the titles already linked and adds four more
	f.calls = nil
	if _, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, o); err != nil {
		t.Fatal(err)
	}
	c = readCatalog(t, out, "lily.test")
	if len(c.Entries) != 9 {
		t.Fatalf("second run: %d entries", len(c.Entries))
	}
	if n := f.called("/api/v1/manga/"); n > 5 {
		t.Errorf("second run fetched %d details; the known titles must be skipped", n)
	}
}

func TestSuwayomiLinkCheckFailureKeepsJoin(t *testing.T) {
	// details that can't be fetched at all: the join stays, flagged unchecked
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Alpha", "en", "https://alpha.test", false)}
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titlesN("Pop", 1, 3), HasNext: false}
	f.fails["/api/v1/manga/1?onlineFetch=true"] = 9
	out := t.TempDir()
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, out, "alpha.test")
	if len(c.Entries) != 3 || rep.Sources[0].Links != "unchecked" {
		t.Errorf("got %d entries, row %+v", len(c.Entries), rep.Sources[0])
	}
}

func TestInferLinkTemplate(t *testing.T) {
	cases := []struct{ url, real, url2, want string }{
		{"https://mangadex.org/manga/06611f84-ff49", "https://mangadex.org/title/06611f84-ff49", "https://mangadex.org/manga/aaaa-bbbb", "https://mangadex.org/title/aaaa-bbbb"},
		{"https://comick.live/calamity-summoner", "https://comick.live/comic/calamity-summoner", "https://comick.live/other-one", "https://comick.live/comic/other-one"},
		{"https://e621.net/51343", "https://e621.net/pools/51343", "https://e621.net/999", "https://e621.net/pools/999"},
		{"https://x.test/manga/slug/", "https://x.test/manga/slug/", "https://x.test/manga/two/", "https://x.test/manga/two/"},
		// a title slug after the id is dropped: the short form is the canonical page
		{"https://mangadex.org/manga/06611f84-ff49", "https://mangadex.org/title/06611f84-ff49/the-high-school", "https://mangadex.org/manga/aaaa-bbbb", "https://mangadex.org/title/aaaa-bbbb"},
	}
	for _, c := range cases {
		tpl, ok := inferLinkTemplate(c.url, c.real)
		if !ok {
			t.Errorf("%s -> %s: no template", c.url, c.real)
			continue
		}
		if got := tpl.apply(c.url2); got != c.want {
			t.Errorf("%s: got %s want %s", c.url2, got, c.want)
		}
	}
	// a slug that isn't in the real link (an id turned into a name) has no template
	if _, ok := inferLinkTemplate("https://lilymanga.net/6955", "https://lilymanga.net/gl/my-food/"); ok {
		t.Error("id -> slug must not give a template")
	}
	if _, ok := inferLinkTemplate("https://asurascans.com/series/overgeared", "https://asurascans.com/comics/overgeared-1bb35e56"); ok {
		t.Error("a hash suffix is not a template (it differs per title)")
	}
}

func TestSuwayomiTemplateWithTitleSlugConfirmedByPrefix(t *testing.T) {
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Dex", "en", "https://dex.test", false)}
	titles := titlesN("Dex", 1, 5)
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titles, HasNext: false}
	for _, title := range titles {
		f.realURL[title] = "https://dex.test/title/" + slugOf(title) + "/some-title-words"
	}
	out := t.TempDir()
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, out, "dex.test")
	if rep.Sources[0].Links != "templated" || c.Entries[0].URL != "https://dex.test/title/dex-001" {
		t.Errorf("row %+v first %+v", rep.Sources[0], c.Entries[0])
	}
}

func TestSuwayomiTemplatedLinksNeedTwoChecks(t *testing.T) {
	// mangadex-like: /manga/<x> listed, /title/<x> real: one sample gives the
	// template, a second confirms it, no fetch per title
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Dex", "en", "https://dex.test", false)}
	titles := titlesN("Dex", 1, 30)
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titles, HasNext: false}
	for _, title := range titles {
		f.realURL[title] = "https://dex.test/title/" + slugOf(title)
	}
	out := t.TempDir()
	rep, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out))
	if err != nil {
		t.Fatal(err)
	}
	if n := f.called("/api/v1/manga/"); n != 2 {
		t.Errorf("details fetched %d times, want two checks", n)
	}
	c := readCatalog(t, out, "dex.test")
	if len(c.Entries) != 30 || !strings.HasPrefix(c.Entries[0].URL, "https://dex.test/title/dex-0") {
		t.Errorf("got %d entries, first %+v", len(c.Entries), c.Entries[0])
	}
	if rep.Sources[0].Links != "templated" || !rep.Sources[0].Complete {
		t.Errorf("row: %+v", rep.Sources[0])
	}
}

func TestSuwayomiReplacesJoinedLinksOnceTheJoinIsKnownWrong(t *testing.T) {
	// a file published with joined links (links "" or "joined") is rebuilt
	// from the real ones, not unioned with the wrong ones
	f := newFakeSuwayomi(t)
	f.sources = []map[string]any{source("1", "Lily", "en", "https://lily.test", false)}
	titles := titlesN("Lily", 1, 3)
	f.pages["/api/v1/source/1/popular/1"] = suwayomiPage{Titles: titles, HasNext: false}
	realFor(f, "https://lily.test", titles...)
	out := t.TempDir()
	old := NewStore(out)
	var wrong []Entry
	for _, title := range titles {
		wrong = append(wrong, Entry{Title: title, URL: "https://lily.test/manga/" + slugOf(title)})
	}
	wrong = append(wrong, Entry{Title: "Gone", URL: "https://lily.test/manga/gone"})
	if err := old.Write(&SiteCatalog{Site: "lily.test", Home: "https://lily.test", Language: "en", Entries: wrong,
		Source: &SourceInfo{ID: "1", Name: "Lily", Lang: "en", Complete: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := CrawlSuwayomi(context.Background(), &bytes.Buffer{}, crawlOpts(f, out)); err != nil {
		t.Fatal(err)
	}
	c := readCatalog(t, out, "lily.test")
	if len(c.Entries) != 3 {
		t.Fatalf("got %d entries: %+v", len(c.Entries), c.Entries)
	}
	for _, e := range c.Entries {
		if !strings.Contains(e.URL, "/series/") {
			t.Errorf("a joined link survived: %+v", e)
		}
	}
	if c.Source.Links != "fetched" {
		t.Errorf("source: %+v", c.Source)
	}
}
