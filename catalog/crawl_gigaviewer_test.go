// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"reflect"
	"testing"
)

// two GigaViewer layouts: the classic list (a heading in the link) and a
// card with only a picture; a banner repeats a series, an episode list
// names an episode, and a link goes to another host
const gigaviewerPage = `<html><body>
<a href="https://g.test/episode/111"><img alt="Banner"><h2 class="series-list-title">SPY×FAMILY</h2></a>
<ul>
<li><a href="https://g.test/episode/111"><h2 class="series-list-title">SPY×FAMILY</h2><p>遠藤達哉</p></a></li>
<li><a href="/episode/222?from=list"><div class="series-title"> ケントゥリア </div><span class="episode-title">第12話</span></a></li>
<li><a href="https://g.test/episode/333"><img src="x.jpg" alt="ダンダダン"></a></li>
<li><a href="https://g.test/episode/334">最新話へ</a></li>
<li><a href="https://g.test/episode/335"><img alt="ふつうの女の子 ４【電子単行本版】"></a></li>
<li><div class="title">社畜OL</div><a href="https://g.test/episode/666" data-series-title="社畜OL ３巻">最初から読む</a></li>
<li><a href="https://other.test/episode/444"><h2>Elsewhere</h2></a></li>
<li><a href="https://g.test/episode/555/comments"><h2>Comments</h2></a></li>
</ul></body></html>`

func TestGigaviewerListsTheSeriesLinks(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://g.test/series": gigaviewerPage})
	site := SiteInfo{Site: "g.test", Home: "https://g.test", Family: "gigaviewer"}
	got, err := crawlers["gigaviewer"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Title: "SPY×FAMILY", URL: "https://g.test/episode/111"},
		{Title: "ケントゥリア", URL: "https://g.test/episode/222"},
		{Title: "ダンダダン", URL: "https://g.test/episode/333"},
		{Title: "ふつうの女の子", URL: "https://g.test/episode/335"},
		{Title: "社畜OL", URL: "https://g.test/episode/666"},
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

func TestGigaviewerUsesTheListURL(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://g.test/comics": gigaviewerPage})
	site := SiteInfo{Site: "g.test", Home: "https://g.test", Family: "gigaviewer", List: "https://g.test/comics"}
	if got, err := crawlers["gigaviewer"].Crawl(context.Background(), site, 0); err != nil || len(got) != 5 {
		t.Fatalf("got %d entries, err %v", len(got), err)
	}
}

func TestGigaviewerWithoutLinksFails(t *testing.T) {
	withFakeFetch(t, map[string]string{"https://g.test/series": `<html><body><a href="/about">About</a></body></html>`})
	site := SiteInfo{Site: "g.test", Home: "https://g.test", Family: "gigaviewer"}
	if _, err := crawlers["gigaviewer"].Crawl(context.Background(), site, 0); err == nil {
		t.Fatal("a page with no series links must be an error")
	}
}

// kuragebunchList is a trimmed /series/{label} page of kuragebunch.com: a
// thumbnail link (the title in its picture's alt), a data link (an h4), and
// the first and latest episode buttons
const kuragebunchList = `<html><body><ul><li class="page-series-list-item"><div class="item-box">
<a href="https://g.test/episode/12207421983893652478" class="series-thumb"><img src="x.png" alt="ファーストペンギン・レクイエム" /></a>
<div class="series-data">
<a href="https://g.test/episode/12207421983893652478" class="series-data-container"><div class="series-tagline"><p>生存率未知数の怪異調査ホラー開幕！</p></div><h4>ファーストペンギン・レクイエム</h4><h5>タダノなつ</h5></a>
<ul class="series-episode-link-list"><li class="first-episode-btn"><a href="https://g.test/episode/12207421983893652478">最初から読む</a></li><li class="latest-episode-btn"><a href="https://g.test/episode/12207421984130957366">最新話を読む</a></li></ul>
</div></div></li></ul></body></html>`

const kuragebunchOneshot = `<html><body><ul><li class="page-series-list-item"><div class="item-box">
<a href="https://g.test/episode/3269632237305146937" class="series-thumb"><img src="x.png" alt="全裸監督" /></a>
</div></li></ul></body></html>`

func TestGigaviewerReadsEveryListPage(t *testing.T) {
	f := withFakeFetch(t, map[string]string{
		"https://g.test/series/kuragebunch": kuragebunchList,
		"https://g.test/series/oneshot":     kuragebunchOneshot,
	})
	site := SiteInfo{Site: "g.test", Home: "https://g.test", List: "https://g.test/series/kuragebunch https://g.test/series/oneshot"}
	got, err := crawlers["gigaviewer"].Crawl(context.Background(), site, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Title: "ファーストペンギン・レクイエム", URL: "https://g.test/episode/12207421983893652478"},
		{Title: "全裸監督", URL: "https://g.test/episode/3269632237305146937"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}

	// the limit caps the pages read
	f.calls = nil
	got, err = crawlers["gigaviewer"].Crawl(context.Background(), site, 1)
	if err != nil || len(got) != 1 || len(f.calls) != 1 {
		t.Fatalf("limit 1: %+v, %d calls, %v", got, len(f.calls), err)
	}
}
