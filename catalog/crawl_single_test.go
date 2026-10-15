// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"testing"
)

const singlePage = `<html><head><title>Heavenly Delusion Manga Online - [Latest Chapters]</title></head><body></body></html>`

func TestSingleUsesLabel(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://w7.s.test/": singlePage})
	site := SiteInfo{Site: "s.test", Home: "https://w7.s.test", Label: "Heavenly Delusion"}
	got, err := crawlers["single"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Heavenly Delusion" || got[0].URL != "https://w7.s.test/" {
		t.Fatalf("got %+v", got)
	}
}

func TestSingleFallsBackToPageTitle(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://w7.s.test/": singlePage})
	site := SiteInfo{Site: "s.test", Home: "https://w7.s.test"}
	got, err := crawlers["single"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Heavenly Delusion Manga Online" {
		t.Fatalf("got %+v", got)
	}
}

func TestSingleFetchErrorStillOneEntry(t *testing.T) {
	withFakeFetch(t, nil)
	site := SiteInfo{Site: "s.test", Home: "https://w7.s.test"}
	got, err := crawlers["single"].Crawl(context.Background(), site, 0)
	if err == nil {
		t.Fatal("the fetch error must be reported")
	}
	if len(got) != 1 || got[0].Title != "s.test" || got[0].URL != "https://w7.s.test/" {
		t.Fatalf("got %+v", got)
	}
}
