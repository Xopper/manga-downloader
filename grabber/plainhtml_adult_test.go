// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"slices"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestAdultSelectors reads a trimmed series page and reader of each
// adultSelectors entry the way PlainHTML does
func TestAdultSelectors(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		series  string
		reader  string
		title   string
		numbers []float64
		pages   []string
	}{
		{
			name: "manhwa18.cc script",
			url:  "https://manhwa18.cc/webtoon/murim",
			series: `<h1>I Ended Up in the World of Murim <span>18+</span></h1>
<ul class="row-content-chapter wleft">
<li class="a-h"><a class="chapter-name text-nowrap" href="/webtoon/murim/chapter-80-5" title="Murim Chapter 80.5">Chapter 80.5</a></li>
<li class="a-h"><a class="chapter-name text-nowrap" href="/webtoon/murim/chapter-1" title="Murim Chapter 1">Chapter 1</a></li>
</ul>`,
			reader: `<div class="read-content">
<img class="loading p1" data-src="https://img33.example.com/chapters/2055/1/1-45669.jpg" src="https://img33.example.com/chapters/2055/1/1-45669.jpg">
<img class="loading p2" data-src="https://img33.example.com/chapters/2055/1/2-45669.jpg" src="https://img33.example.com/chapters/2055/1/2-45669.jpg">
</div><div class="related"><img data-src="https://example.com/covers/other.jpg"></div>`,
			title:   "I Ended Up in the World of Murim",
			numbers: []float64{80.5, 1},
			pages: []string{
				"https://img33.example.com/chapters/2055/1/1-45669.jpg",
				"https://img33.example.com/chapters/2055/1/2-45669.jpg",
			},
		},
		{
			name: "manhwa18.com chap rows",
			url:  "https://manhwa18.com/manga/run-away",
			series: `<div class="series-name"><a href="/manga/run-away">Run away</a></div>
<ul class="list-chapters at-series"><a href="https://manhwa18.com/manga/run-away/chap-61-90565" title="chap 61"><li><div class="chapter-name text-truncate"> chap 61 END </div><div class="chapter-time">1904 view - 03/05/2026</div></li></a><a href="https://manhwa18.com/manga/run-away/chapter-60-net-90109" title="Chapter 60"><li><div class="chapter-name text-truncate"> Chapter 60 </div></li></a></ul>`,
			reader:  `<div id="chapter-content"><img class="lazy" data-src="https://min.example.net/chapters/90565/1.jpg"></div>`,
			title:   "Run away",
			numbers: []float64{61, 60},
			pages:   []string{"https://min.example.net/chapters/90565/1.jpg"},
		},
		{
			name: "hentara.com",
			url:  "https://hentara.com/manhwa/common-sense",
			series: `<h1>Read Common Sense Manhwa Online Free</h1>
<nav class="ssr-breadcrumb"><a href="/">Home</a><span>›</span><a href="/browse">Browse</a><span>›</span><span>Common Sense UNCENSORED</span></nav>
<nav class="ssr-chapters-grid"><a class="ssr-chapter" href="/manhwa/common-sense/chapter-02">Ch. 2</a><a class="ssr-chapter" href="/manhwa/common-sense/chapter-01">Ch. 1</a></nav>`,
			reader: `<img src="https://cdn.example.com/common-sense/chapter-001/001.jpg">
<img src="https://cdn.example.com/common-sense/chapter-001/002.jpg">
<a href="/manhwa/other"><img src="https://cdn.example.com/other/thumbnail.png"></a>`,
			title:   "Common Sense UNCENSORED",
			numbers: []float64{2, 1},
			pages: []string{
				"https://cdn.example.com/common-sense/chapter-001/001.jpg",
				"https://cdn.example.com/common-sense/chapter-001/002.jpg",
			},
		},
		{
			name: "pornhwa18.com",
			url:  "https://pornhwa18.com/comic/irl-pvp/",
			series: `<h1 class="text-3xl font-bold drop-shadow-solid">IRL PvP</h1>
<div class="mt-4 flex w-full items-center"><a href="/comic/irl-pvp/chapter-1/">First Chapter</a><a href="/comic/irl-pvp/chapter-35/">Last Chapter</a></div>
<div class="mt-4 flex max-h-96 flex-col gap-2 overflow-y-auto"><a href="/comic/irl-pvp/chapter-35/"><div><p>Chapter 35 </p><p class="text-xs font-medium">10 mth ago</p></div></a><a href="/comic/irl-pvp/chapter-34/"><div><p>Chapter 34 </p><p class="text-xs font-medium">10 mth ago</p></div></a></div>`,
			reader: `<img class="imgku" src="/readerarea.svg" data-src="/banner.jpg">
<img class="imgku" src="/readerarea.svg" data-src="https://cdn.example.com/1408/1/6813775fd404e.jpg">`,
			title:   "IRL PvP",
			numbers: []float64{35, 34},
			pages:   []string{"https://cdn.example.com/1408/1/6813775fd404e.jpg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			series, _ := goquery.NewDocumentFromReader(strings.NewReader(tt.series))
			var site SiteSelector
			found := false
			for _, s := range plainHTMLSelectors {
				if series.Find(s.Rows).Length() > 0 {
					site, found = s, true
					break
				}
			}
			if !found {
				t.Fatal("no entry matches the series page")
			}
			m := PlainHTML{Grabber: &Grabber{URL: tt.url}, doc: series, rows: series.Find(site.Rows), site: site}

			if title, _ := m.FetchTitle(); title != tt.title {
				t.Errorf("FetchTitle() = %q, want %q", title, tt.title)
			}
			chapters, errs := m.FetchChapters()
			var numbers []float64
			for _, c := range chapters {
				numbers = append(numbers, c.GetNumber())
			}
			if len(errs) != 0 || !slices.Equal(numbers, tt.numbers) {
				t.Errorf("chapter numbers = %v (errors %v), want %v", numbers, errs, tt.numbers)
			}

			reader, _ := goquery.NewDocumentFromReader(strings.NewReader(tt.reader))
			chapter := m.chapterFromDoc(&PlainHTMLChapter{Chapter: Chapter{Number: 1}}, reader)
			var pages []string
			for _, p := range chapter.Pages {
				pages = append(pages, p.URL)
			}
			if !slices.Equal(pages, tt.pages) {
				t.Errorf("pages = %v, want %v", pages, tt.pages)
			}
		})
	}
}
