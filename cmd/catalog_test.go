// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/catalog"
)

// fakeCrawler returns fixed entries, or an error, per site
type fakeCrawler struct {
	entries map[string][]catalog.Entry
	fail    map[string]error
}

func (f fakeCrawler) Crawl(_ context.Context, site catalog.SiteInfo, _ int) ([]catalog.Entry, error) {
	if err := f.fail[site.Site]; err != nil {
		return nil, err
	}

	return f.entries[site.Site], nil
}

func crawlSetup(t *testing.T, f fakeCrawler, sites ...catalog.SiteInfo) string {
	t.Helper()
	catalog.RegisterCrawler("fake", f)
	prev := sitesForCrawl
	sitesForCrawl = func() []catalog.SiteInfo { return sites }
	t.Cleanup(func() { sitesForCrawl = prev })

	return t.TempDir()
}

func entries(n int, site string) []catalog.Entry {
	out := make([]catalog.Entry, n)
	for i := range out {
		out[i] = catalog.Entry{Title: site + " " + string(rune('A'+i%26)) + string(rune('a'+i/26)), URL: "https://" + site + "/manga/" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "/"}
	}

	return out
}

func TestCrawlWritesFilesAndIndex(t *testing.T) {
	a := catalog.SiteInfo{Site: "a.test", Home: "https://a.test", Family: "fake", Lang: "en", Crawl: true}
	b := catalog.SiteInfo{Site: "b.test", Home: "https://b.test", Family: "fake", Lang: "es", Crawl: true}
	skipped := catalog.SiteInfo{Site: "c.test", Home: "https://c.test", Family: "custom", Crawl: false}
	out := crawlSetup(t, fakeCrawler{entries: map[string][]catalog.Entry{"a.test": entries(3, "a.test"), "b.test": entries(5, "b.test")}}, a, b, skipped)
	var log bytes.Buffer
	if code := runCrawl(context.Background(), &log, out, "", 0); code != 0 {
		t.Fatalf("exit %d: %s", code, log.String())
	}
	s := catalog.NewStore(out)
	s.Load()
	ca, _ := s.Catalog("a.test")
	cb, _ := s.Catalog("b.test")
	if ca == nil || cb == nil || len(ca.Entries) != 3 || len(cb.Entries) != 5 || cb.Language != "es" || cb.Home != "https://b.test" || time.Since(cb.Crawled) > time.Minute {
		t.Fatalf("a %+v b %+v", ca, cb)
	}
	idx, err := catalog.ReadIndex(out)
	if err != nil || len(idx.Sites) != 2 {
		t.Fatalf("index: %+v %v", idx, err)
	}
	if !strings.Contains(log.String(), "a.test") || !strings.Contains(log.String(), "5 entries") {
		t.Errorf("log: %s", log.String())
	}
	if _, err := os.Stat(filepath.Join(out, "c.test.json")); err == nil {
		t.Error("a site without crawl must not be written")
	}
}

func TestCrawlHalfSizeGuard(t *testing.T) {
	a := catalog.SiteInfo{Site: "a.test", Home: "https://a.test", Family: "fake", Lang: "en", Crawl: true}
	out := crawlSetup(t, fakeCrawler{entries: map[string][]catalog.Entry{"a.test": entries(40, "a.test")}}, a)
	old := catalog.NewStore(out)
	if err := old.Write(&catalog.SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0), Entries: entries(100, "a.test")}); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if code := runCrawl(context.Background(), &log, out, "", 0); code != 2 {
		t.Fatalf("exit %d: %s", code, log.String())
	}
	s := catalog.NewStore(out)
	s.Load()
	if c, _ := s.Catalog("a.test"); c == nil || len(c.Entries) != 100 {
		t.Fatalf("old file lost: %+v", c)
	}
	if !strings.Contains(log.String(), "kept") {
		t.Errorf("log: %s", log.String())
	}
	// a crawl at least half the old size replaces it
	catalog.RegisterCrawler("fake", fakeCrawler{entries: map[string][]catalog.Entry{"a.test": entries(50, "a.test")}})
	if code := runCrawl(context.Background(), &log, out, "", 0); code != 0 {
		t.Fatalf("exit %d: %s", code, log.String())
	}
	s.Load()
	if c, _ := s.Catalog("a.test"); len(c.Entries) != 50 {
		t.Errorf("not replaced: %d", len(c.Entries))
	}
}

func TestCrawlFailedSiteKeepsOthers(t *testing.T) {
	a := catalog.SiteInfo{Site: "a.test", Home: "https://a.test", Family: "fake", Lang: "en", Crawl: true}
	b := catalog.SiteInfo{Site: "b.test", Home: "https://b.test", Family: "fake", Lang: "en", Crawl: true}
	out := crawlSetup(t, fakeCrawler{entries: map[string][]catalog.Entry{"b.test": entries(2, "b.test")}, fail: map[string]error{"a.test": errors.New("down")}}, a, b)
	var log bytes.Buffer
	if code := runCrawl(context.Background(), &log, out, "", 0); code != 2 {
		t.Fatalf("exit %d: %s", code, log.String())
	}
	if _, err := os.Stat(filepath.Join(out, "b.test.json")); err != nil {
		t.Error("b not written")
	}
	if !strings.Contains(log.String(), "FAIL") || !strings.Contains(log.String(), "down") {
		t.Errorf("log: %s", log.String())
	}
	// an empty crawl is a failure too, never an empty file
	catalog.RegisterCrawler("fake", fakeCrawler{})
	if code := runCrawl(context.Background(), &log, out, "b.test", 0); code != 2 {
		t.Errorf("empty crawl: exit %d", code)
	}
}

func TestCrawlOneSite(t *testing.T) {
	a := catalog.SiteInfo{Site: "a.test", Home: "https://a.test", Family: "fake", Lang: "en", Crawl: true}
	b := catalog.SiteInfo{Site: "b.test", Home: "https://b.test", Family: "fake", Lang: "en", Crawl: true}
	out := crawlSetup(t, fakeCrawler{entries: map[string][]catalog.Entry{"a.test": entries(1, "a.test"), "b.test": entries(1, "b.test")}}, a, b)
	var log bytes.Buffer
	if code := runCrawl(context.Background(), &log, out, "www.B.test", 0); code != 0 {
		t.Fatalf("exit %d: %s", code, log.String())
	}
	if _, err := os.Stat(filepath.Join(out, "a.test.json")); err == nil {
		t.Error("a crawled too")
	}
	if code := runCrawl(context.Background(), &log, out, "nope.test", 0); code != 1 || !strings.Contains(log.String(), "nope.test") {
		t.Errorf("unknown site: exit %d, %s", code, log.String())
	}
}

func TestStatusJSON(t *testing.T) {
	dir := t.TempDir()
	s := catalog.NewStore(dir)
	s.Write(&catalog.SiteCatalog{Site: "mangaread.org", Crawled: time.Unix(100, 0), Entries: entries(3, "mangaread.org")})
	var out bytes.Buffer
	runStatus(&out, s, true)
	var rows []statusRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err, out.String())
	}
	var found bool
	for _, r := range rows {
		if r.Site == "mangaread.org" {
			found = r.Entries == 3 && r.Catalog
		}
	}
	if !found || len(rows) != len(catalog.Sites()) {
		t.Errorf("rows %d, mangaread found %v", len(rows), found)
	}
	out.Reset()
	runStatus(&out, s, false)
	if !strings.Contains(out.String(), "mangaread.org") || !strings.Contains(out.String(), "SITE") {
		t.Errorf("table: %s", out.String())
	}
}

// partialCrawler returns some entries and then an error, like a walk that hit a 503
type partialCrawler struct{}

func (partialCrawler) Crawl(context.Context, catalog.SiteInfo, int) ([]catalog.Entry, error) {
	return entries(3, "a.test"), errors.New("received 503 response code")
}

func TestCrawlStoppedEarlyIsNotWritten(t *testing.T) {
	a := catalog.SiteInfo{Site: "a.test", Home: "https://a.test", Family: "fake", Lang: "en", Crawl: true}
	out := crawlSetup(t, fakeCrawler{}, a)
	catalog.RegisterCrawler("fake", partialCrawler{})
	old := catalog.NewStore(out)
	if err := old.Write(&catalog.SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0), Entries: entries(5, "a.test")}); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if code := runCrawl(context.Background(), &log, out, "", 0); code != 2 {
		t.Fatalf("exit %d: %s", code, log.String())
	}
	s := catalog.NewStore(out)
	s.Load()
	if c, _ := s.Catalog("a.test"); c == nil || len(c.Entries) != 5 {
		t.Fatalf("a partial crawl replaced the file: %+v", c)
	}
	if !strings.Contains(log.String(), "FAIL") || !strings.Contains(log.String(), "503") {
		t.Errorf("log: %s", log.String())
	}
}

func TestStatusJSONOmitsCrawledWithoutCatalog(t *testing.T) {
	var out bytes.Buffer
	runStatus(&out, catalog.NewStore(t.TempDir()), true)
	if strings.Contains(out.String(), "0001-01-01") {
		t.Error("a site without a catalog prints a zero crawl time")
	}
}

func TestCatalogIndexCommand(t *testing.T) {
	dir := t.TempDir()
	s := catalog.NewStore(dir)
	s.Write(&catalog.SiteCatalog{Site: "x.test", Crawled: time.Unix(100, 0), Entries: entries(2, "x.test")})
	b, _ := json.Marshal([]catalog.SourceRow{{Site: "x.test", Name: "X", Lang: "en", SourceID: "2", Entries: 2}})
	os.WriteFile(filepath.Join(dir, "sources-0.json"), b, 0o600)
	var log bytes.Buffer
	if code := runIndex(&log, dir, false); code != 0 {
		t.Fatalf("exit %d: %s", code, log.String())
	}
	idx, err := catalog.ReadIndex(dir)
	if err != nil || len(idx.Sites) != 1 {
		t.Fatalf("index %+v %v", idx, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sources.json")); err != nil {
		t.Error("sources.json not written")
	}
	if !strings.Contains(log.String(), "1 site") {
		t.Errorf("log: %s", log.String())
	}
}

func TestRunStatusDLColumn(t *testing.T) {
	dir := t.TempDir()
	s := catalog.NewStore(dir)
	s.Write(&catalog.SiteCatalog{Site: "x.test", Crawled: time.Unix(100, 0), Entries: entries(1, "x.test")})
	b, _ := json.Marshal([]catalog.SourceRow{{Site: "x.test", Name: "X", Lang: "en", SourceID: "2", Entries: 1, NSFW: true}})
	os.WriteFile(filepath.Join(dir, "sources.json"), b, 0o600)
	s.Load()
	var out bytes.Buffer
	runStatus(&out, s, false)
	if !strings.Contains(out.String(), "DL") {
		t.Errorf("no DL column: %s", out.String())
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "x.test") && (!strings.Contains(line, "nsfw") || !strings.Contains(line, " - ")) {
			t.Errorf("x row: %q", line)
		}
	}
	out.Reset()
	runStatus(&out, s, true)
	var rows []statusRow
	json.Unmarshal(out.Bytes(), &rows)
	for _, r := range rows {
		if r.Site == "x.test" && (r.Downloadable || !r.NSFW || r.Source != "X") {
			t.Errorf("json row: %+v", r)
		}
	}
}

func TestParseShard(t *testing.T) {
	for in, want := range map[string][2]int{"": {0, 0}, "0/4": {0, 4}, "3/4": {3, 4}} {
		shard, shards, err := parseShard(in)
		if err != nil || shard != want[0] || shards != want[1] {
			t.Errorf("%q: %d/%d %v", in, shard, shards, err)
		}
	}
	for _, bad := range []string{"4/4", "a/b", "1", "-1/2", "1/0"} {
		if _, _, err := parseShard(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCrawlKeepsSuwayomiEntries(t *testing.T) {
	// a site a Suwayomi source also covers keeps that source's entries when
	// the hand-written crawler rewrites the file
	a := catalog.SiteInfo{Site: "a.test", Home: "https://a.test", Family: "fake", Lang: "en", Crawl: true}
	out := crawlSetup(t, fakeCrawler{entries: map[string][]catalog.Entry{"a.test": entries(3, "a.test")}}, a)
	old := catalog.NewStore(out)
	merged := append(entries(2, "a.test"), catalog.Entry{Title: "From Suwayomi", URL: "https://a.test/manga/suwa/"})
	if err := old.Write(&catalog.SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0), Entries: merged,
		Source: &catalog.SourceInfo{ID: "9", Name: "A Source", Lang: "en", Complete: true}}); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if code := runCrawl(context.Background(), &log, out, "", 0); code != 0 {
		t.Fatalf("exit %d: %s", code, log.String())
	}
	s := catalog.NewStore(out)
	s.Load()
	c, _ := s.Catalog("a.test")
	if c == nil || len(c.Entries) != 4 || c.Source == nil || c.Source.Name != "A Source" {
		t.Fatalf("got %+v", c)
	}
	if c.Entries[0].Title != "a.test Aa" || c.Entries[3].Title != "From Suwayomi" {
		t.Errorf("the crawl's entries come first, the source's after: %+v", c.Entries)
	}
}
