// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestKeyoappTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://sirenscans.org/choking-on-love", true},
		{"https://www.sirenscans.org/choking-on-love-chapter-3", true},
		{"https://grimscans.com/series/calm-down-lady-ophelia/", false},
	}
	for _, tt := range tests {
		got, _ := NewKeyoapp(&Grabber{URL: tt.url}).Test()
		if got != tt.want {
			t.Errorf("Test(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

// keyoappRows is a trimmed rows_html: a coin chapter and a free one
const keyoappRows = `<a href="/choking-on-love-chapter-30"
    class="chapter-row flex items-center" data-title="chapter 30" data-index="1"
    data-ch-uid="65cf1bab84e" data-ch-url="/choking-on-love-chapter-30"
    data-ch-label="Chapter 30" data-ch-locked="1" data-ch-paid="1" data-ch-coins="100"></a>
<a href="/choking-on-love-chapter-1-5"
    class="chapter-row flex items-center" data-title="chapter 1.5" data-index="2"
    data-ch-uid="6460af341f0" data-ch-url="/choking-on-love-chapter-1-5"
    data-ch-label="Chapter 1.5 - Prologue" data-ch-locked="0" data-ch-paid="0" data-ch-coins="0"></a>`

func TestKeyoappChapters(t *testing.T) {
	chapters, errs := keyoappChapters(keyoappRows, "https://sirenscans.org/choking-on-love")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	locked := chapters[0].(*KeyoappChapter)
	if !locked.Locked || locked.Number != 30 || locked.ID != "65cf1bab84e" || locked.Title != "" {
		t.Errorf("locked chapter = %+v", locked.Chapter)
	}
	free := chapters[1].(*KeyoappChapter)
	if free.Locked || free.Number != 1.5 || free.Title != "Prologue" || free.URL != "https://sirenscans.org/choking-on-love-chapter-1-5" {
		t.Errorf("free chapter = %+v", free)
	}
}

func TestKeyoappPages(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<img alt="Siren Scans" src="https://wsrv.nl/?url=cdn.meowing.org/uploads/logo">
<div id="strip-reader">
<div class="reader-page-ph"><img class="reader-page w-full block" data-page="1" src="https://cdn.meowing.org/uploads/ozZn-IQ1SI2"></div>
<div class="reader-page-ph"><img class="reader-page w-full block" data-page="2" src="data:image/gif;base64,R0lGOD" data-src="https://cdn.meowing.org/uploads/wdbE9ABMNMi"></div>
</div>`))
	if err != nil {
		t.Fatal(err)
	}
	got := keyoappPages(doc, "https://sirenscans.org/choking-on-love-chapter-1")
	if len(got) != 2 || got[0] != "https://cdn.meowing.org/uploads/ozZn-IQ1SI2" || got[1] != "https://cdn.meowing.org/uploads/wdbE9ABMNMi" {
		t.Errorf("pages = %v", got)
	}
}

func TestKeyoappResolveSeriesURL(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/choking-on-love", `<h1>Choking on Love</h1><div data-series-uid="646094761b7" data-series-slug="choking-on-love"></div>`)
	respond(mux, "/choking-on-love-chapter-1", `<div id="sidebar-ch-list" data-series-slug="choking-on-love"></div>`)
	srv := serve(t, mux)

	tests := []struct {
		name string
		url  string
	}{
		{"series URL", srv.URL + "/choking-on-love"},
		{"chapter URL", srv.URL + "/choking-on-love-chapter-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := NewKeyoapp(&Grabber{URL: tt.url, Settings: &Settings{}})
			title, err := k.FetchTitle()
			if err != nil {
				t.Fatal(err)
			}
			if k.URL != srv.URL+"/choking-on-love" || k.slug != "choking-on-love" || title != "Choking on Love" {
				t.Errorf("URL = %q, slug = %q, title = %q", k.URL, k.slug, title)
			}
		})
	}
}
