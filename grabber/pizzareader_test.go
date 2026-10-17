// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestPizzaReader serves trimmed copies of hni-scantrad.net's API answers
func newTestPizzaReader(t *testing.T) *httptest.Server {
	t.Helper()
	files := map[string]string{
		"/api/comics/buyuden": `{"comic":{"title":"Buyuden","slug":"buyuden","url":"/comics/buyuden","chapters":[` +
			`{"full_title":"Tome 13 - Chapitre 134 : Devenir un champion","title":"Devenir un champion","volume":13,"chapter":134,"subchapter":null,"language":"fr","teams":[{"name":"Tennis no Sensei"},null],"slug_lang_vol_ch_sub":"fr-13-134-N","url":"/read/buyuden/fr/vol/13/ch/134"},` +
			`{"full_title":"Chapter 12.5","title":"","volume":null,"chapter":12,"subchapter":5,"language":"en","teams":[],"slug_lang_vol_ch_sub":"en-N-12-5","url":"/read/buyuden/en/ch/12/sub/5"},` +
			`{"full_title":"Inoue x Donaire","title":"","volume":null,"chapter":null,"subchapter":null,"language":"en","teams":[null],"slug_lang_vol_ch_sub":"en-N-N-N","url":"/read/buyuden/en"}]}}`,
		"/api/read/buyuden/fr/vol/13/ch/134": `{"comic":{"title":"Buyuden"},"chapter":{"pages":["https://example.com/storage/comics/b/01.jpg","https://example.com/storage/comics/b/02.jpg"]}}`,
		"/api/comics/other":                  `{"error":"not found"}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestPizzaReaderTest(t *testing.T) {
	srv := newTestPizzaReader(t)
	cases := map[string]bool{
		"/comics/buyuden":                true,
		"/read/buyuden/fr/vol/13/ch/134": true,
		"/comics/other":                  false,
		"/comics/missing":                false,
		"/manga/buyuden":                 false,
	}
	for path, want := range cases {
		got, err := NewPizzaReader(&Grabber{URL: srv.URL + path, Settings: &Settings{}}).Test()
		if err != nil || got != want {
			t.Errorf("Test(%q) = %v, %v, want %v", path, got, err, want)
		}
	}
}

func TestPizzaReaderFetch(t *testing.T) {
	srv := newTestPizzaReader(t)
	p := NewPizzaReader(&Grabber{URL: srv.URL + "/read/buyuden/fr/vol/13/ch/134", Settings: &Settings{}})

	title, err := p.FetchTitle()
	if err != nil || title != "Buyuden" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := p.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d chapters, %v", len(chapters), errs)
	}
	first := chapters[0].(*PizzaReaderChapter)
	if first.Number != 134 || first.Volume != 13 || first.Title != "Devenir un champion" || first.Group != "Tennis no Sensei" || first.ID != "fr-13-134-N" {
		t.Errorf("first = %+v", first.Chapter)
	}
	if got := ChapterLabel(chapters[1]); chapters[1].GetNumber() != 12.5 || got != "Chapter 12.5" {
		t.Errorf("subchapter = %v %q", chapters[1].GetNumber(), got)
	}
	if got := ChapterLabel(chapters[2]); chapters[2].GetNumber() != 0 || got != "Inoue x Donaire" {
		t.Errorf("one-shot = %v %q", chapters[2].GetNumber(), got)
	}

	chapter, err := p.FetchChapter(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://example.com/storage/comics/b/02.jpg" || chapter.Group != "Tennis no Sensei" {
		t.Errorf("chapter = %+v", chapter)
	}
}

func TestPizzaReaderLanguage(t *testing.T) {
	srv := newTestPizzaReader(t)
	p := NewPizzaReader(&Grabber{URL: srv.URL + "/comics/buyuden", Settings: &Settings{Language: "fr"}})
	chapters, _ := p.FetchChapters()
	if len(chapters) != 1 || chapters[0].GetNumber() != 134 {
		t.Errorf("fr chapters = %d", len(chapters))
	}
	// a language the series doesn't have keeps them all
	p = NewPizzaReader(&Grabber{URL: srv.URL + "/comics/buyuden", Settings: &Settings{Language: "es"}})
	if chapters, _ := p.FetchChapters(); len(chapters) != 3 {
		t.Errorf("es chapters = %d", len(chapters))
	}
}
