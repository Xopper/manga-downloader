// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"net/url"
	"testing"
)

func TestInmangaPostsTheConsultForm(t *testing.T) {
	const list = "https://inmanga.com/manga/getMangasConsultResult"
	form := func(skip string) string {
		return url.Values{
			"filter[generes][]": {"-1"}, "filter[queryString]": {""}, "filter[skip]": {skip},
			"filter[take]": {"200"}, "filter[sortby]": {"1"}, "filter[broadcastStatus]": {"0"},
			"filter[onlyFavorites]": {"false"}, "d": {""},
		}.Encode()
	}
	f := withFakeBodies(t, map[string]string{
		list + " " + form("0"):   read(t, "inmanga/page-1.html"),
		list + " " + form("200"): read(t, "inmanga/page-2.html"),
	})
	got, err := crawlers["inmanga"].Crawl(context.Background(), SiteInfo{Site: "inmanga.com", Home: "https://inmanga.com", List: list}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "One Piece" ||
		got[0].URL != "https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935" {
		t.Fatalf("got %+v", got)
	}
	if len(f.calls) != 2 {
		t.Errorf("the empty second page ends the walk: %+v", f.calls)
	}
}
