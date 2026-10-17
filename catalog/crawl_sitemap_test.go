// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"testing"
)

const sitemapIndex = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://s.test/wp-sitemap-posts-post-1.xml</loc></sitemap>
<sitemap><loc>https://s.test/wp-sitemap-posts-manga-1.xml</loc></sitemap>
</sitemapindex>`

const sitemapSeries = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://s.test/manga/</loc></url>
<url><loc>https://s.test/manga/sekiro-manga/</loc></url>
<url><loc>https://s.test/manga/the-scorched-desert&apos;s-sand-mage/</loc></url>
<url><loc>https://s.test/manga/466933-07-ghost/</loc></url>
</urlset>`

const sitemapChapters = `<urlset>
<url><loc>https://c.test/manga/blue-box/159/1</loc></url>
<url><loc>https://c.test/manga/blue-box/158/1</loc></url>
<url><loc>https://c.test/manga/grand-blue/91/1</loc></url>
<url><loc>https://c.test/teams/someone</loc></url>
</urlset>`

func TestSitemapReadsTheSeriesSitemapsOfAnIndex(t *testing.T) {
	withFakeFetch(t, map[string]string{
		"https://s.test/wp-sitemap.xml":               sitemapIndex,
		"https://s.test/wp-sitemap-posts-manga-1.xml": sitemapSeries,
	})
	site := SiteInfo{Site: "s.test", Family: "sitemap", List: "https://s.test/wp-sitemap.xml", Sitemaps: `posts-manga`, Series: `/manga/[^/]+/$`}
	got, err := crawlers["sitemap"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Title: "Sekiro Manga", URL: "https://s.test/manga/sekiro-manga/"},
		{Title: "The Scorched Desert's Sand Mage", URL: "https://s.test/manga/the-scorched-desert's-sand-mage/"},
		{Title: "07 Ghost", URL: "https://s.test/manga/466933-07-ghost/"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Title != want[i].Title || got[i].URL != want[i].URL {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSitemapMapsChaptersToTheirSeries(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://c.test/sitemap.xml": sitemapChapters})
	site := SiteInfo{Site: "c.test", Family: "sitemap", List: "https://c.test/sitemap.xml", Series: `^(https://c\.test/manga/[^/]+)/`}
	got, err := crawlers["sitemap"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].URL != "https://c.test/manga/blue-box" || got[0].Title != "Blue Box" || got[1].Title != "Grand Blue" {
		t.Errorf("got %+v", got)
	}
}

func TestSlugTitle(t *testing.T) {
	cases := map[string]string{
		"https://mangalib.me/ru/manga/264101--tonari-no-yosei-san": "Tonari No Yosei San",
		"https://utoon.us/en/manga/gachiakuta.html":                "Gachiakuta",
		"https://x.test/manga/one_piece/":                          "One Piece",
		"https://x.test/":                                          "",
	}
	for u, want := range cases {
		if got := slugTitle(u); got != want {
			t.Errorf("slugTitle(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestSitemapBuildsTheSeriesURL(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://u.test/sitemap.xml": `<urlset>
<url><loc>https://u.test/en/gachiakuta/gachiakuta-chapter-167.html</loc></url>
<url><loc>https://u.test/en/gachiakuta/gachiakuta-chapter-166.html</loc></url>
</urlset>`})
	site := SiteInfo{Site: "u.test", Family: "sitemap", List: "https://u.test/sitemap.xml", Series: `/en/([^/]+)/[^/]+-chapter-[^/]+\.html$`, SeriesURL: "https://u.test/en/manga/{1}.html"}
	got, err := crawlers["sitemap"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://u.test/en/manga/gachiakuta.html" || got[0].Title != "Gachiakuta" {
		t.Errorf("got %+v", got)
	}
}
