// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
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
