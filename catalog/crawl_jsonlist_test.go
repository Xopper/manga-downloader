// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"reflect"
	"testing"
)

func TestJSONListOneCall(t *testing.T) {
	withFakeFetch(t, map[string]string{
		"https://p.test/api/comics": read(t, "jsonlist/pizzareader.json"),
	})
	site := SiteInfo{Site: "p.test", Family: "jsonlist", List: "https://p.test/api/comics", Items: "comics", ItemAlt: "alt_titles"}
	got, err := crawlers["jsonlist"].Crawl(context.Background(), site, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Title: "Aishiteru Game wo Owarasetai", URL: "https://p.test/comics/aishiteru-game-wo-owarasetai", AltNames: []string{"愛してるゲームを終わらせたい", "I Want to End This Love Game"}},
		{Title: "Alien Headbutt", URL: "https://p.test/comics/alien-headbutt", AltNames: []string{"エイリアンヘッドバット"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestJSONListPagesWithTemplate(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://v.test/api/search?page=1": read(t, "jsonlist/mangavault-1.json"),
		"https://v.test/api/search?page=2": read(t, "jsonlist/mangavault-2.json"),
		"https://v.test/api/search?page=3": read(t, "jsonlist/mangavault-3.json"),
	})
	site := SiteInfo{
		Site: "v.test", Family: "jsonlist", List: "https://v.test/api/search?page={page}",
		Items: "results", ItemAlt: "alternative_titles", SeriesURL: "https://v.test/manga/{slug}",
	}
	got, err := crawlers["jsonlist"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || len(f.calls) != 3 {
		t.Fatalf("got %d entries in %d calls: %+v", len(got), len(f.calls), got)
	}
	first := Entry{
		Title:    "La Ascensión del Rey Demonio Elfo",
		URL:      "https://v.test/manga/la-ascension-del-rey-demonio-elfo-329d04",
		AltNames: []string{"City of Sin", "Ciudad Del Pecado", "罪恶之城"},
	}
	if !reflect.DeepEqual(got[0], first) {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].AltNames[0] != "The Strongest Reincarnated as a Mediocre Heir of a Prestigious Family" {
		t.Errorf("a null alt name must be skipped: %+v", got[1].AltNames)
	}

	// the limit caps the pages
	f.calls = nil
	got, err = crawlers["jsonlist"].Crawl(context.Background(), site, 1)
	if err != nil || len(got) != 2 || len(f.calls) != 1 {
		t.Fatalf("limit 1: %d entries, %d calls, %v", len(got), len(f.calls), err)
	}
}

func TestJSONListWordPressCategories(t *testing.T) {
	list := "https://w.test/wp-json/wp/v2/categories?per_page=100&page={page}"
	withFakeFetch(t, map[string]string{
		pageURL(list, 1): read(t, "jsonlist/wp-categories.json"),
		pageURL(list, 2): "[]",
	})
	site := SiteInfo{Site: "w.test", Family: "jsonlist", List: list, ItemTitle: "name", ItemURL: "link", Exclude: "^Uncategorized$"}
	got, err := crawlers["jsonlist"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Title: "Berserk", URL: "https://animeallstar30.com/category/berserk/"},
		{Title: "Shuumatsu no Valkyrie: Gods’ Apocalypse", URL: "https://animeallstar30.com/category/shuumatsu-no-valkyrie-gods-apocalypse/"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestJSONListPageMissingEndsTheWalk(t *testing.T) {
	withFakeFetch(t, map[string]string{
		"https://b.test/api/list?page=1": read(t, "jsonlist/baozimh.json"),
	})
	site := SiteInfo{
		Site: "b.test", Family: "jsonlist", List: "https://b.test/api/list?page={page}",
		Items: "items", ItemTitle: "name", SeriesURL: "https://b.test/comic/{comic_id}",
	}
	got, err := crawlers["jsonlist"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "一人之下" || got[0].URL != "https://b.test/comic/yirenzhixia-dongmantang" {
		t.Fatalf("got %+v", got)
	}
}

func TestJSONListErrors(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://e.test/list": `{"comics":[]}`, "https://e.test/bad": `<html>`})
	for _, site := range []SiteInfo{
		{Site: "e.test", List: "https://e.test/list", Items: "comics"},
		{Site: "e.test", List: "https://e.test/bad"},
		{Site: "e.test", List: "https://e.test/none"},
		{Site: "e.test"},
	} {
		if got, err := crawlers["jsonlist"].Crawl(context.Background(), site, 0); err == nil {
			t.Errorf("%s: no error, got %+v", site.List, got)
		}
	}
}

func TestFillFields(t *testing.T) {
	item := map[string]any{"slug": "x-1", "id": float64(42), "meta": map[string]any{"lang": "es"}}
	if got := fillFields("https://a.test/{meta.lang}/{slug}/{id}", item); got != "https://a.test/es/x-1/42" {
		t.Errorf("got %q", got)
	}
	if got := fillFields("https://a.test/{missing}", item); got != "" {
		t.Errorf("a missing field must give no URL, got %q", got)
	}
}
