// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import "testing"

func TestSitesLoadsEmbeddedTable(t *testing.T) {
	sites := Sites()
	if len(sites) < 100 {
		t.Fatalf("expected the full site table, got %d", len(sites))
	}
	seen := map[string]bool{}
	for _, s := range sites {
		if s.Site == "" || s.Home == "" || s.Family == "" || s.Lang == "" {
			t.Errorf("incomplete row: %+v", s)
		}
		if seen[s.Site] {
			t.Errorf("duplicate site %s", s.Site)
		}
		seen[s.Site] = true
		if s.Crawl && s.Family != "single" && s.List == "" {
			t.Errorf("%s crawls but has no list URL", s.Site)
		}
		if s.Crawl && s.Family == "custom" {
			t.Errorf("%s has no crawler family but crawls", s.Site)
		}
		if _, ok := CrawlerFor(s); s.Crawl && !ok {
			t.Errorf("%s crawls but its family %q has no crawler", s.Site, s.Family)
		}
	}
}

func TestSitesReturnsACopy(t *testing.T) {
	a := Sites()
	a[0].Site = "changed"
	if b := Sites(); b[0].Site == "changed" {
		t.Error("Sites() exposes the embedded table")
	}
}

func TestSiteByHost(t *testing.T) {
	for _, host := range []string{"kingofshojo.com", "www.kingofshojo.com", "WWW.KingOfShojo.com"} {
		if s, ok := SiteByHost(host); !ok || s.Site != "kingofshojo.com" {
			t.Errorf("%s: got %+v %v", host, s, ok)
		}
	}
	if s, ok := SiteByHost("genzupdates.com"); !ok || s.Site != "genztoons.org" {
		t.Errorf("alias: got %+v %v", s, ok)
	}
	if _, ok := SiteByHost("nope.example"); ok {
		t.Error("unknown host matched")
	}
}
