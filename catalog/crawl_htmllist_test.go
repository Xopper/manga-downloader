// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"reflect"
	"testing"
)

func TestHTMLListOnePage(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://m.test/changeMangaList?type=text": read(t, "htmllist/mmrcms.html"),
	})
	site := SiteInfo{Site: "m.test", Family: "htmllist", List: "https://m.test/changeMangaList?type=text", Items: "ul.manga-list a.alpha-link"}
	got, err := crawlers["htmllist"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 3 || len(f.calls) != 1 {
		t.Fatalf("got %d entries in %d calls", len(got), len(f.calls))
	}
	want := Entry{Title: "2 jikanme Hoken Taiiku", URL: "https://onma.top/manga/2-jikanme-hoken-taiiku"}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Title != `A "Dimwitted" Monk fell from Heaven` {
		t.Errorf("second = %+v", got[1])
	}
}

func TestHTMLListOptionValues(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://l.test/": read(t, "htmllist/lelscans.html")})
	site := SiteInfo{Site: "l.test", Family: "htmllist", List: "https://l.test/", Items: `option[value*="/lecture-"]`, ItemURL: "value"}
	got, err := crawlers["htmllist"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Title: "Assassination Classroom", URL: "https://lelscans.net/lecture-ligne-assassination-classroom.php"},
		{Title: "Beelzebub", URL: "https://lelscans.net/lecture-en-ligne-beelzebub.php"},
		{Title: "Black Clover", URL: "https://lelscans.net/lecture-en-ligne-black-clover.php"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestHTMLListPagesUntilMissing(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://j.test/manga-list.html?page=1": read(t, "htmllist/fmreader-1.html"),
	})
	site := SiteInfo{Site: "j.test", Family: "htmllist", List: "https://j.test/manga-list.html?page={page}", Items: ".series-title a"}
	got, err := crawlers["htmllist"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || len(f.calls) != 2 {
		t.Fatalf("got %d entries in %d calls: %+v", len(got), len(f.calls), got)
	}
	want := Entry{Title: `" Yuri " Naru Damaged Car Trip!`, URL: "https://j.test/hwms--yuri-naru-damaged-car-trip.html"}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("first = %+v", got[0])
	}
}

func TestHTMLListTitleSelector(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://s.test/series/": read(t, "htmllist/silentquill.html")})
	site := SiteInfo{
		Site: "s.test", Family: "htmllist", List: "https://s.test/series/",
		Items: `li a[href^="/series/"]:not([href="/series/"])`, ItemTitle: "span.font-semibold", Exclude: "^Never$",
	}
	got, err := crawlers["htmllist"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	want := Entry{
		Title: "A Bottom-Tier Ojou-Sama Dungeon Streamer Beats Up a Nuisance Streamer, Goes Viral, and Becomes a Legend?!",
		URL:   "https://s.test/series/a-bottom-tier-ojou-sama-dungeon-streamer-beats-up-a-nuisance-streamer-goes-viral-and-becomes-a-legend/",
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("first = %+v", got[0])
	}
}

func TestHTMLListErrors(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://e.test/": "<html><body></body></html>"})
	for _, site := range []SiteInfo{
		{Site: "e.test", List: "https://e.test/", Items: "a.series"},
		{Site: "e.test", List: "https://e.test/none", Items: "a"},
		{Site: "e.test", List: "https://e.test/"},
	} {
		if got, err := crawlers["htmllist"].Crawl(context.Background(), site, 0); err == nil {
			t.Errorf("%+v: no error, got %+v", site, got)
		}
	}
}
