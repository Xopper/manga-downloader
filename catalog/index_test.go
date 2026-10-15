// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildIndexHashes(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}, {Title: "B", URL: "https://a.test/b/"}}})
	writeCatalog(t, dir, SiteCatalog{Site: "b.test", Crawled: time.Unix(200, 0).UTC(), Entries: []Entry{{Title: "C", URL: "https://b.test/c/"}}})
	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Sites) != 2 || idx.Sites[0].Site != "a.test" || idx.Sites[0].File != "a.test.json" || idx.Sites[0].Entries != 2 || idx.Sites[1].Entries != 1 {
		t.Fatalf("got %+v", idx.Sites)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.test.json"))
	sum := sha256.Sum256(b)
	if idx.Sites[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Error("hash is not the file's sha256")
	}
	if !idx.Sites[0].Crawled.Equal(time.Unix(100, 0)) {
		t.Error("crawled time not carried")
	}
	if err := WriteIndex(dir, idx); err != nil {
		t.Fatal(err)
	}
	back, err := ReadIndex(dir)
	if err != nil || len(back.Sites) != 2 {
		t.Fatalf("read back: %+v %v", back, err)
	}
	if _, err := ReadIndex(t.TempDir()); !os.IsNotExist(err) {
		t.Errorf("missing index: %v", err)
	}
}

func TestBaseURLEnvOverride(t *testing.T) {
	t.Setenv("MANGA_DOWNLOADER_CATALOG_URL", "http://x.test/cat")
	if got := BaseURL(); got != "http://x.test/cat/" {
		t.Errorf("got %q (a trailing slash is added)", got)
	}
	t.Setenv("MANGA_DOWNLOADER_CATALOG_URL", "")
	if got := BaseURL(); got != "https://raw.githubusercontent.com/Xopper/manga-downloader/catalog-data/catalog/" {
		t.Errorf("got %q", got)
	}
}

// published is a fake publishing server: a dir of catalog files with its
// index, served over HTTP, counting requests and failing chosen files
type published struct {
	dir   string
	srv   *httptest.Server
	hits  atomic.Int32
	fails map[string]bool
}

func publish(t *testing.T, catalogs ...SiteCatalog) *published {
	t.Helper()
	p := &published{dir: t.TempDir(), fails: map[string]bool{}}
	for _, c := range catalogs {
		writeCatalog(t, p.dir, c)
	}
	idx, err := BuildIndex(p.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteIndex(p.dir, idx); err != nil {
		t.Fatal(err)
	}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		name := filepath.Base(r.URL.Path)
		if p.fails[name] {
			http.Error(w, "boom", 500)
			return
		}
		http.ServeFile(w, r, filepath.Join(p.dir, name))
	}))
	t.Cleanup(p.srv.Close)

	return p
}

func (p *published) base() string { return p.srv.URL + "/catalog/" }

func TestUpdateDownloadsOnlyChanged(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	b := SiteCatalog{Site: "b.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "B", URL: "https://b.test/b/"}}}
	p := publish(t, a, b)
	s := NewStore(t.TempDir())
	s.Load()
	changed, err := s.Update(context.Background(), p.base())
	if err != nil || len(changed) != 2 {
		t.Fatalf("first update: %v %v", changed, err)
	}
	if p.hits.Load() != 3 {
		t.Errorf("index + 2 files, got %d requests", p.hits.Load())
	}
	if hits := s.Search("b", nil, 0); len(hits) != 1 {
		t.Error("downloaded catalog not searchable")
	}
	// nothing changed: only the index is fetched
	p.hits.Store(0)
	changed, err = s.Update(context.Background(), p.base())
	if err != nil || len(changed) != 0 || p.hits.Load() != 1 {
		t.Fatalf("second update: %v %v, %d requests", changed, err, p.hits.Load())
	}
	// b changes upstream
	b.Entries = append(b.Entries, Entry{Title: "B2", URL: "https://b.test/b2/"})
	p2 := publish(t, a, b)
	changed, err = s.Update(context.Background(), p2.base())
	if err != nil || len(changed) != 1 || changed[0] != "b.test" || p2.hits.Load() != 2 {
		t.Fatalf("third update: %v %v, %d requests", changed, err, p2.hits.Load())
	}
	if c, _ := s.Catalog("b.test"); len(c.Entries) != 2 {
		t.Error("b not refreshed in memory")
	}
}

func TestUpdateKeepsOldFilesOnFailure(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	b := SiteCatalog{Site: "b.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "B", URL: "https://b.test/b/"}}}
	p := publish(t, a, b)
	s := NewStore(t.TempDir())
	if _, err := s.Update(context.Background(), p.base()); err != nil {
		t.Fatal(err)
	}
	oldIndex, _ := ReadIndex(s.Dir)
	// both change upstream, a's download breaks
	a.Entries = append(a.Entries, Entry{Title: "A2", URL: "https://a.test/a2/"})
	b.Entries = append(b.Entries, Entry{Title: "B2", URL: "https://b.test/b2/"})
	p2 := publish(t, a, b)
	p2.fails["a.test.json"] = true
	changed, err := s.Update(context.Background(), p2.base())
	if err == nil {
		t.Fatal("the failed file must be reported")
	}
	if len(changed) != 1 || changed[0] != "b.test" {
		t.Errorf("changed %v", changed)
	}
	if c, _ := s.Catalog("a.test"); len(c.Entries) != 1 {
		t.Error("a's old catalog lost")
	}
	if c, _ := s.Catalog("b.test"); len(c.Entries) != 2 {
		t.Error("b not updated")
	}
	idx, _ := ReadIndex(s.Dir)
	for _, e := range idx.Sites {
		switch e.Site {
		case "a.test":
			if e.SHA256 != oldIndex.Sites[0].SHA256 {
				t.Error("a's index entry must stay the old one")
			}
		case "b.test":
			if e.Entries != 2 {
				t.Error("b's index entry must be the new one")
			}
		}
	}
	// next update retries only a
	p3 := publish(t, a, b)
	changed, err = s.Update(context.Background(), p3.base())
	if err != nil || len(changed) != 1 || changed[0] != "a.test" {
		t.Fatalf("retry: %v %v", changed, err)
	}
}

func TestUpdateRejectsBadHash(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	p := publish(t, a)
	// tamper with the published index so the hash can't match
	idx, _ := ReadIndex(p.dir)
	idx.Sites[0].SHA256 = "deadbeef"
	WriteIndex(p.dir, idx)
	s := NewStore(t.TempDir())
	changed, err := s.Update(context.Background(), p.base())
	if err == nil || len(changed) != 0 {
		t.Fatalf("got %v %v", changed, err)
	}
	if _, ok := s.Catalog("a.test"); ok {
		t.Error("a file with the wrong hash was kept")
	}
}

func TestUpdateCancelled(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	p := publish(t, a)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewStore(t.TempDir()).Update(ctx, p.base()); err == nil {
		t.Fatal("cancelled update must fail")
	}
	_ = json.Valid
}

func TestUpdateRejectsSitesOutsideTheDir(t *testing.T) {
	// a published index naming a site like "../gui" must never write outside Dir
	a := SiteCatalog{Site: "../gui", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	b := SiteCatalog{Site: "b.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "B", URL: "https://b.test/b/"}}}
	p := &published{dir: t.TempDir(), fails: map[string]bool{}}
	ab, _ := json.Marshal(a)
	os.WriteFile(filepath.Join(p.dir, "evil.json"), ab, 0o600)
	writeCatalog(t, p.dir, b)
	sum := sha256.Sum256(ab)
	bb, _ := os.ReadFile(filepath.Join(p.dir, "b.test.json"))
	sumB := sha256.Sum256(bb)
	idx := &Index{Sites: []IndexEntry{
		{Site: "../gui", File: "evil.json", Entries: 1, SHA256: hex.EncodeToString(sum[:])},
		{Site: "b.test", File: "../b.test.json", Entries: 1, SHA256: hex.EncodeToString(sumB[:])},
	}}
	WriteIndex(p.dir, idx)
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(p.dir, filepath.Base(r.URL.Path)))
	}))
	t.Cleanup(p.srv.Close)
	root := t.TempDir()
	s := NewStore(filepath.Join(root, "catalog"))
	changed, err := s.Update(context.Background(), p.base())
	if err == nil || len(changed) != 0 {
		t.Fatalf("got %v %v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(root, "gui.json")); err == nil {
		t.Fatal("wrote outside the catalog dir")
	}
	if _, ok := s.Catalog("b.test"); ok {
		t.Error("a file named with a path separator was accepted")
	}
}

func TestUpdateCancelledMidwayKeepsTheIndexWhole(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	b := SiteCatalog{Site: "b.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "B", URL: "https://b.test/b/"}}}
	p := publish(t, a, b)
	s := NewStore(t.TempDir())
	if _, err := s.Update(context.Background(), p.base()); err != nil {
		t.Fatal(err)
	}
	before, _ := ReadIndex(s.Dir)
	info, _ := os.Stat(filepath.Join(s.Dir, indexFile))
	old := time.Now().Add(-48 * time.Hour)
	os.Chtimes(filepath.Join(s.Dir, indexFile), old, old)
	// both change upstream; the context is cancelled once a's file is served
	a.Entries = append(a.Entries, Entry{Title: "A2", URL: "https://a.test/a2/"})
	b.Entries = append(b.Entries, Entry{Title: "B2", URL: "https://b.test/b2/"})
	p2 := publish(t, a, b)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	served := p2.srv.Config.Handler
	p2.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served.ServeHTTP(w, r)
		if filepath.Base(r.URL.Path) == "a.test.json" {
			cancel()
		}
	})
	changed, err := s.Update(ctx, p2.base())
	if err == nil {
		t.Fatal("a cancelled update must say so")
	}
	if len(changed) != 1 || changed[0] != "a.test" {
		t.Errorf("changed %v", changed)
	}
	after, err := ReadIndex(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Sites) != 2 {
		t.Fatalf("the index lost sites: %+v", after.Sites)
	}
	for _, e := range after.Sites {
		if e.Site == "b.test" && e.SHA256 != before.Sites[1].SHA256 {
			t.Error("b, never reached, must keep its old index entry")
		}
	}
	if fi, _ := os.Stat(filepath.Join(s.Dir, indexFile)); !fi.ModTime().Equal(info.ModTime()) && fi.ModTime().After(time.Now().Add(-time.Hour)) {
		t.Error("a cancelled update must not make the index look fresh")
	}
}

// gz compresses b
func gz(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(b)
	zw.Close()

	return buf.Bytes()
}

func TestGzipDir(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "a.test", Entries: []Entry{{Title: "A", URL: "https://a.test/a/", Latest: 26.5}}})
	writeRows(t, dir, "sources.json", []SourceRow{{Site: "a.test"}})
	WriteIndex(dir, &Index{})
	if err := Gzip(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.test.json")); err == nil {
		t.Error("plain file kept")
	}
	if _, err := os.Stat(filepath.Join(dir, "a.test.json.gz")); err != nil {
		t.Error("gz file missing")
	}
	for _, keep := range []string{"sources.json", indexFile} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("%s must stay plain", keep)
		}
	}
	// the gz round-trips through the store
	s := NewStore(dir)
	if warns := s.Load(); len(warns) != 0 {
		t.Fatal(warns)
	}
	if c, ok := s.Catalog("a.test"); !ok || len(c.Entries) != 1 || c.Entries[0].Latest != 26.5 {
		t.Error("Load doesn't read .json.gz, or loses the latest chapter")
	}
}

func TestBuildIndexReadsGzip(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}, {Title: "B", URL: "https://a.test/b/"}}})
	writeRows(t, dir, "sources.json", []SourceRow{{Site: "a.test", Name: "A"}})
	if err := Gzip(dir); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Sites) != 1 || idx.Sites[0].File != "a.test.json.gz" || idx.Sites[0].Entries != 2 || idx.Sites[0].Site != "a.test" {
		t.Fatalf("got %+v", idx.Sites)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.test.json.gz"))
	sum := sha256.Sum256(b)
	if idx.Sites[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Error("the hash is of the gzipped bytes")
	}
	if len(idx.Extra) != 1 || idx.Extra[0].File != "sources.json" || idx.Extra[0].SHA256 == "" {
		t.Errorf("extra: %+v", idx.Extra)
	}
}

// publishGz is publish with the files gzipped and sources.json beside them
func publishGz(t *testing.T, rows []SourceRow, catalogs ...SiteCatalog) *published {
	t.Helper()
	p := &published{dir: t.TempDir(), fails: map[string]bool{}}
	for _, c := range catalogs {
		writeCatalog(t, p.dir, c)
	}
	if rows != nil {
		writeRows(t, p.dir, "sources.json", rows)
	}
	if err := Gzip(p.dir); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(p.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteIndex(p.dir, idx); err != nil {
		t.Fatal(err)
	}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.hits.Add(1)
		name := filepath.Base(r.URL.Path)
		if p.fails[name] {
			http.Error(w, "boom", 500)
			return
		}
		http.ServeFile(w, r, filepath.Join(p.dir, name))
	}))
	t.Cleanup(p.srv.Close)

	return p
}

func TestUpdateGunzips(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	p := publishGz(t, []SourceRow{{Site: "a.test", Name: "A", SourceID: "1"}}, a)
	s := NewStore(t.TempDir())
	changed, err := s.Update(context.Background(), p.base())
	if err != nil || len(changed) != 1 {
		t.Fatalf("%v %v", changed, err)
	}
	// the local file is plain json
	b, err := os.ReadFile(filepath.Join(s.Dir, "a.test.json"))
	if err != nil || !json.Valid(b) {
		t.Fatalf("local file: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "a.test.json.gz")); err == nil {
		t.Error("the gz was kept locally")
	}
	if len(s.Sources()) != 1 || s.Sources()[0].Name != "A" {
		t.Errorf("sources.json not fetched: %+v", s.Sources())
	}
	// unchanged: only the index again
	p.hits.Store(0)
	s.Update(context.Background(), p.base())
	if p.hits.Load() != 1 {
		t.Errorf("%d requests for nothing changed", p.hits.Load())
	}
}

func TestUpdateRejectsGzipOfAnotherSite(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	p := publishGz(t, nil, a)
	// swap the file for b's content, with the index hash matching the swap
	bb, _ := json.Marshal(SiteCatalog{Site: "b.test", Entries: []Entry{{Title: "B", URL: "https://b.test/b/"}}})
	os.WriteFile(filepath.Join(p.dir, "a.test.json.gz"), gz(t, bb), 0o600)
	idx, _ := ReadIndex(p.dir)
	sum := sha256.Sum256(gz(t, bb))
	idx.Sites[0].SHA256 = hex.EncodeToString(sum[:])
	WriteIndex(p.dir, idx)
	s := NewStore(t.TempDir())
	if _, err := s.Update(context.Background(), p.base()); err == nil {
		t.Fatal("accepted another site's content")
	}
	if _, ok := s.Catalog("a.test"); ok {
		t.Error("a.test written with b's content")
	}
	if _, ok := s.Catalog("b.test"); ok {
		t.Error("b.test written")
	}
}

func TestUpdateStillAcceptsPlainFiles(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	p := publish(t, a)
	s := NewStore(t.TempDir())
	if changed, err := s.Update(context.Background(), p.base()); err != nil || len(changed) != 1 {
		t.Fatalf("%v %v", changed, err)
	}
}

func TestUpdateFetchesExtraSourcesWhenChanged(t *testing.T) {
	a := SiteCatalog{Site: "a.test", Crawled: time.Unix(100, 0).UTC(), Entries: []Entry{{Title: "A", URL: "https://a.test/a/"}}}
	p := publishGz(t, []SourceRow{{Site: "a.test", Name: "A"}}, a)
	s := NewStore(t.TempDir())
	s.Update(context.Background(), p.base())
	p2 := publishGz(t, []SourceRow{{Site: "a.test", Name: "A renamed"}}, a)
	changed, err := s.Update(context.Background(), p2.base())
	if err != nil || len(changed) != 0 {
		t.Fatalf("%v %v", changed, err)
	}
	if p2.hits.Load() != 2 {
		t.Errorf("index + sources.json expected, %d requests", p2.hits.Load())
	}
	if s.Sources()[0].Name != "A renamed" {
		t.Error("sources.json not refreshed")
	}
}

func TestPlainFileWinsOverItsGzipTwin(t *testing.T) {
	dir := t.TempDir()
	writeCatalog(t, dir, SiteCatalog{Site: "a.test", Entries: []Entry{{Title: "Old", URL: "https://a.test/old/"}}})
	if err := Gzip(dir); err != nil {
		t.Fatal(err)
	}
	writeCatalog(t, dir, SiteCatalog{Site: "a.test", Entries: []Entry{{Title: "New", URL: "https://a.test/new/"}, {Title: "New2", URL: "https://a.test/new2/"}}})
	s := NewStore(dir)
	s.Load()
	if c, _ := s.Catalog("a.test"); c == nil || len(c.Entries) != 2 {
		t.Fatalf("got %+v", c)
	}
	idx, _ := BuildIndex(dir)
	if len(idx.Sites) != 1 || idx.Sites[0].File != "a.test.json" {
		t.Errorf("index %+v", idx.Sites)
	}
	// gzipping again replaces the stale gz with the fresh content
	Gzip(dir)
	s.Load()
	if c, _ := s.Catalog("a.test"); len(c.Entries) != 2 {
		t.Error("stale gz survived")
	}
}
