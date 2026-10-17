// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"testing"
)

func TestJSONAPIPagesUntilEmpty(t *testing.T) {
	f := withFakeBodies(t, map[string]string{
		"https://api.h.test/query?page=1": read(t, "jsonapi/heancms-1.json"),
		"https://api.h.test/query?page=2": read(t, "jsonapi/heancms-2.json"),
	})
	site := SiteInfo{Site: "h.test", Home: "https://h.test", List: "https://api.h.test/query?page={page}",
		Items: "data", TitleKey: "title", SeriesURL: "https://h.test/series/{series_slug}"}
	got, err := crawlers["jsonapi"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	// the third item has no slug
	if len(got) != 2 || got[0].Title != "100 Curses of the Ellestone Manor" ||
		got[0].URL != "https://h.test/series/100-curses-of-the-ellestone-manor" {
		t.Fatalf("got %+v", got)
	}
	if len(f.calls) != 2 || f.calls[0].Referer != "https://h.test/" {
		t.Errorf("calls %+v: two pages, the site as referer", f.calls)
	}
}

func TestJSONAPIRootArrayEndsOn400(t *testing.T) {
	f := withFakeBodies(t, map[string]string{
		"https://w.test/wp-json/wp/v2/categories?page=1": read(t, "jsonapi/wp-categories-1.json"),
	})
	f.codes["https://w.test/wp-json/wp/v2/categories?page=2"] = 400
	site := SiteInfo{Site: "w.test", Home: "https://w.test", List: "https://w.test/wp-json/wp/v2/categories?page={page}",
		TitleKey: "name", SeriesURL: "{link}"}
	got, err := crawlers["jsonapi"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].Title != "DR. STONE & Co" || got[2].URL != "https://tvymanga3.com/category/dr-stone/" {
		t.Fatalf("got %+v", got)
	}
}

func TestJSONAPIFirstPageError(t *testing.T) {
	withFakeBodies(t, map[string]string{"https://w.test/api?page=1": "<html>not json</html>"})
	site := SiteInfo{Site: "w.test", Home: "https://w.test", List: "https://w.test/api?page={page}",
		TitleKey: "name", SeriesURL: "{link}"}
	if _, err := crawlers["jsonapi"].Crawl(context.Background(), site, 0); err == nil {
		t.Error("an answer that isn't JSON must fail")
	}
}
