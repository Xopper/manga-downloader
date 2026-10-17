// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// plainHTMLEntryCase is a trimmed series page and reader page of a site served
// by a PlainHTML entry, and what the entry must read from them
type plainHTMLEntryCase struct {
	name       string
	series     string
	reader     string
	wantRows   string
	wantTitle  string
	wantNums   []float64
	wantPages  []string
	seriesPath string
}

// checkPlainHTMLEntry serves the series page, checks that Test() picks the
// expected entry (no earlier one shadows it) and that the title, chapter
// numbers and reader pages come out right
func checkPlainHTMLEntry(t *testing.T, c plainHTMLEntryCase) {
	t.Helper()
	path := c.seriesPath
	if path == "" {
		path = "/series/test/"
	}
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(c.series))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	m := NewPlainHTML(&Grabber{URL: srv.URL + path, Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
	}
	if m.site.Rows != c.wantRows {
		t.Fatalf("Test() matched rows %q, want %q", m.site.Rows, c.wantRows)
	}
	if title, _ := m.FetchTitle(); title != c.wantTitle {
		t.Errorf("FetchTitle() = %q, want %q", title, c.wantTitle)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	if len(chapters) != len(c.wantNums) {
		t.Fatalf("FetchChapters() = %d chapters, want %d", len(chapters), len(c.wantNums))
	}
	for i, want := range c.wantNums {
		if got := chapters[i].GetNumber(); got != want {
			t.Errorf("chapter %d number = %v, want %v", i, got, want)
		}
	}

	chapter := m.chapterFromDoc(chapters[0], docFromHTML(t, c.reader))
	if len(chapter.Pages) != len(c.wantPages) {
		t.Fatalf("got %d pages, want %d: %v", len(chapter.Pages), len(c.wantPages), pageURLs(chapter))
	}
	for i, want := range c.wantPages {
		want = strings.Replace(want, "BASE", srv.URL, 1)
		if chapter.Pages[i].URL != want {
			t.Errorf("page %d = %q, want %q", i+1, chapter.Pages[i].URL, want)
		}
	}
}

func TestPlainHTMLEntries(t *testing.T) {
	cases := []plainHTMLEntryCase{{
		name: "readfairytail",
		series: `<html><body>
<h1 class="text-white mb-3 uppercase sm:text-lg md:text-3xl font-bold">Read Fairy Tail &amp; Eden's Zero Manga Online</h1>
<h1 class="my-3 font-bold text-2xl md:text-3xl">Fairy Tail: Happy’s Grand Adventure</h1>
<div class="bg-bg-secondary p-3 rounded mb-3 shadow"><div class="grid grid-cols-4 items-center">
<div class="col-span-4 lg:col-span-3"><a class="text-sm md:text-base font-bold mb-1 text-text-primary" href="https://ww9.readfairytail.com/chapter/fairy-tail-happys-grand-adventure-chapter-79/">Fairy Tail: Happy’s Grand Adventure Chapter 79</a></div>
<div class="lg:col-span-1"><a class="mt-3 lg:mt-0 uppercase text-xs font-bold py-1 px-2 rounded" href="https://ww9.readfairytail.com/chapter/fairy-tail-happys-grand-adventure-chapter-79/">Read</a></div>
</div></div>
<div class="bg-bg-secondary p-3 rounded mb-3 shadow"><div class="grid grid-cols-4 items-center">
<div class="col-span-4 lg:col-span-3"><a class="text-sm md:text-base font-bold mb-1 text-text-primary" href="https://ww9.readfairytail.com/chapter/fairy-tail-happys-grand-adventure-chapter-78-5/">Fairy Tail: Happy’s Grand Adventure Chapter 78.5</a></div>
<div class="lg:col-span-1"><a class="mt-3 lg:mt-0 uppercase text-xs font-bold py-1 px-2 rounded" href="https://ww9.readfairytail.com/chapter/fairy-tail-happys-grand-adventure-chapter-78-5/">Read</a></div>
</div></div>
</body></html>`,
		reader: `<html><body><div class="js-pages-container"><div class="text-center">
<img src="https://cdn.readfairytail.com/file/mangap/1176/10079000/1.jpeg?t=1721214406" class="mb-3 mx-auto js-page" alt="Page 1" />
<img src="https://cdn.readfairytail.com/file/mangap/1176/10079000/2.jpeg?t=1721214406" class="mb-3 mx-auto js-page" alt="Page 2" />
</div></div></body></html>`,
		wantRows:  `div.bg-bg-secondary:has(a.mb-1[href*="/chapter/"])`,
		wantTitle: "Fairy Tail: Happy’s Grand Adventure",
		wantNums:  []float64{79, 78.5},
		wantPages: []string{
			"https://cdn.readfairytail.com/file/mangap/1176/10079000/1.jpeg?t=1721214406",
			"https://cdn.readfairytail.com/file/mangap/1176/10079000/2.jpeg?t=1721214406",
		},
	}, {
		name: "readberserk",
		series: `<html><body><h1 class="font-weight-normal text-uppercase">Read Berserk Manga Online</h1>
<h2 class="mb-0">Manga: <span>Duranki</span></h2>
<div class="card card-table"><div class="card-body table-responsive p-0"><table class="table table-striped table-borderless">
<thead><tr><th>Chapter</th><th>Date</th><th>Action</th></tr></thead>
<tbody class="no-border-x">
<tr><td>Duranki Chapter 7</td><td>Oct 2, 2026</td><td><div class="d-flex"><a href="https://readberserk.com/chapter/duranki-chapter-7/" class="btn btn-sm btn-primary mr-2">Read</a></div></td></tr>
<tr><td>Duranki Chapter 6</td><td>Sep 2, 2026</td><td><div class="d-flex"><a href="https://readberserk.com/chapter/duranki-chapter-6/" class="btn btn-sm btn-primary mr-2">Read</a></div></td></tr>
</tbody></table></div></div></body></html>`,
		reader: `<html><body><div class="pages text-center">
<div class="img_container mb-2"><img class="pages__img" src="https://live.staticflickr.com/65535/50931739641_6452126096_o.png" loading="eager"></div>
<div class="img_container mb-2"><img class="pages__img" src="https://live.staticflickr.com/65535/50931739642_1a2b3c4d5e_o.jpg" loading="lazy"></div>
</div></body></html>`,
		wantRows:  `.card-table tbody tr:has(a[href*="/chapter/"])`,
		wantTitle: "Duranki",
		wantNums:  []float64{7, 6},
		wantPages: []string{
			"https://live.staticflickr.com/65535/50931739641_6452126096_o.png",
			"https://live.staticflickr.com/65535/50931739642_1a2b3c4d5e_o.jpg",
		},
	}, {
		name: "comic easel widget",
		series: `<html><body><h1 id="site-title" class="site-title"><a href="/">Tower Of God Manga Online</a></h1>
<article><header><h1 class="entry-title" itemprop="headline">Tower Of God</h1></header><div class="entry-content">
<div id='Chapters_List' class='widgets_on_page'><ul><li id="ceo_latest_comics_widget-3" class="widget ceo_latest_comics_widget"><h2 class="widgettitle">All Chapters</h2><ul>
<li><a href="https://w82.thetowerofgod.com/manga/tower-of-god-chapter-603/">Tower Of God Chapter 603</a></li>
<li><a href="https://w82.thetowerofgod.com/manga/tower-of-god-chapter-602/">Tower Of God Chapter 602</a></li>
</ul></li></ul></div></div></article></body></html>`,
		// a3 Lazy Load leaves a shared placeholder in src, the page in
		// data-src, and a <noscript> copy that isn't parsed as markup
		reader: `<html><body><div class="entry-content"><h2>Tower Of God</h2>
<div class="separator"><a href="https://blogger.googleusercontent.com/img/b/R29v/s9172/tower_of_god_603_1.jpg"><img class="lazy lazy-hidden" src="//w82.thetowerofgod.com/wp-content/plugins/a3-lazy-load/assets/images/lazy_placeholder.gif" data-lazy-type="image" data-src="https://blogger.googleusercontent.com/img/b/R29v/s15000/tower_of_god_603_1.jpg"><noscript><img src="https://blogger.googleusercontent.com/img/b/R29v/s15000/tower_of_god_603_1.jpg"></noscript></a></div>
<div class="separator"><a href="https://blogger.googleusercontent.com/img/b/R29v/s9172/tower_of_god_603_2.jpg"><img class="lazy lazy-hidden" src="//w82.thetowerofgod.com/wp-content/plugins/a3-lazy-load/assets/images/lazy_placeholder.gif" data-lazy-type="image" data-src="https://blogger.googleusercontent.com/img/b/R29v/s15000/tower_of_god_603_2.jpg"></a></div>
</div></body></html>`,
		wantRows:  "li.ceo_latest_comics_widget li:has(a)",
		wantTitle: "Tower Of God",
		wantNums:  []float64{603, 602},
		wantPages: []string{
			"https://blogger.googleusercontent.com/img/b/R29v/s15000/tower_of_god_603_1.jpg",
			"https://blogger.googleusercontent.com/img/b/R29v/s15000/tower_of_god_603_2.jpg",
		},
	}, {
		name: "saymanhwa",
		// the chapter grid sits in an element with id="chapters", which the
		// meowing.org entry's "#chapters a" would otherwise claim
		series: `<html><body><h1>Dusk Divide</h1><section id="chapters">
<div class="series-v72-chapter-list" data-chapter-grid>
<a class="series-v72-chapter-row series-chapter-card is-unread " data-chapter-item data-chapter-number="29" href="/en/series/dusk-divide/chapter-29"><span class="chapter-read-mark" title="Unread">○</span><strong><span class="series-chapter-number-text">Chapter 29</span></strong><time class="series-chapter-date" datetime="2026-09-04T10:02:29+07:00">2026-09-04</time></a>
<a class="series-v72-chapter-row series-chapter-card is-unread " data-chapter-item data-chapter-number="28" href="/en/series/dusk-divide/chapter-28"><span class="chapter-read-mark" title="Unread">○</span><strong><span class="series-chapter-number-text">Chapter 28</span></strong><time class="series-chapter-date" datetime="2026-09-03T10:02:29+07:00">2026-09-03</time></a>
</div></section></body></html>`,
		reader: `<html><body><header class="reader-heading"><h1>Dusk Divide — Chapter 29<span class="reader-language-badge"><img src="/public/assets/images/flags/en.webp" alt=""><b>EN</b></span></h1></header>
<div class="reader-pages" data-reader-pages>
<img src="https://imgsrv5.com/mg1/fastcdn/cdn_mangaraw/dusk-divide/chapter-29/0.jpg" loading="eager" referrerpolicy="no-referrer" alt="Dusk Divide Chapter 29 page 1">
<img src="https://imgsrv5.com/mg1/fastcdn/cdn_mangaraw/dusk-divide/chapter-29/1.jpg" loading="lazy" referrerpolicy="no-referrer" alt="Dusk Divide Chapter 29 page 2">
</div></body></html>`,
		wantRows:  "a[data-chapter-item][data-chapter-number]",
		wantTitle: "Dusk Divide",
		wantNums:  []float64{29, 28},
		wantPages: []string{
			"https://imgsrv5.com/mg1/fastcdn/cdn_mangaraw/dusk-divide/chapter-29/0.jpg",
			"https://imgsrv5.com/mg1/fastcdn/cdn_mangaraw/dusk-divide/chapter-29/1.jpg",
		},
	}, {
		name: "hentaisco",
		series: `<html><body><h1 class="mt-2 font-display text-4xl">I’ll Plug It In!</h1>
<ol class="mt-4 grid gap-1 sm:grid-cols-2" data-chapter-list>
<li data-chapter-number="15" data-chapter-name="chapter 15"><a href="/hentai/ill-plug-it-in/chapter-15/" class="flex items-center"><span class="truncate">Chapter 15</span><time class="shrink-0 text-xs">2 days ago</time></a></li>
<li data-chapter-number="14" data-chapter-name="chapter 14"><a href="/hentai/ill-plug-it-in/chapter-14/" class="flex items-center"><span class="truncate">Chapter 14</span><time class="shrink-0 text-xs">9 days ago</time></a></li>
</ol></body></html>`,
		reader: `<html><body>
<img src="https://cdn.hentaisco.cc/manga_6a2c469a6c626/chapter-15/001.jpg" alt="Page 1" class="reader-page" data-page="1">
<img src="https://cdn.hentaisco.cc/manga_6a2c469a6c626/chapter-15/002.jpg" alt="Page 2" class="reader-page" loading="lazy" data-page="2">
</body></html>`,
		wantRows:   "ol[data-chapter-list] li[data-chapter-number]",
		wantTitle:  "I’ll Plug It In!",
		wantNums:   []float64{15, 14},
		seriesPath: "/hentai/ill-plug-it-in/",
		wantPages: []string{
			"https://cdn.hentaisco.cc/manga_6a2c469a6c626/chapter-15/001.jpg",
			"https://cdn.hentaisco.cc/manga_6a2c469a6c626/chapter-15/002.jpg",
		},
	}, {
		name: "webnex",
		// a chapter can have another group's upload in a menu; the reader
		// renders its first pages and lists them all in the flight data
		series: `<html><body><h1>I Took Over the Demonic Ancestor</h1>
<section class="chapters" id="chapters"><ul>
<li class="ch-row"><a class="ch-link" href="/read/chapter-mangadex-4c1c7a31-6f990926f518"><span class="ch-num"><span class="visually-hidden">Chapter </span>48</span><span class="ch-title">[Season 1 End]</span></a><span class="ch-source">ManhwaFreak</span></li>
<li class="ch-row"><a class="ch-link" href="/read/chapter-mangadex-baa935a6-c12087afa3a5"><span class="ch-num"><span class="visually-hidden">Chapter </span>47</span><span class="ch-title is-plain">Chapter 47</span></a><div class="ch-sources"><button class="ch-source-trigger" type="button"><span class="ch-source-name">Ansh Scans</span></button><div class="ch-sources-menu"><ul><li><span class="ch-sources-item is-current"><span class="ch-sources-name">Ansh Scans</span></span></li><li><a class="ch-sources-item" href="/read/chapter-mangadex-072ee120-f2edfcbbe4a1"><span class="ch-sources-name">ManhwaFreak</span></a></li></ul></div></div></li>
</ul></section></body></html>`,
		reader: `<html><body>
<img src="/api/image/e94f7a56ab088fabd8511998be6fb420/001.webp" class="page-image" loading="eager">
<script>self.__next_f.push([1,"3:{\"group\":\"ManhwaFreak\",\"pages\":[\"/api/image/e94f7a56ab088fabd8511998be6fb420/001.webp\",\"/api/image/e94f7a56ab088fabd8511998be6fb420/002.webp\"]}\n"])</script>
</body></html>`,
		wantRows:  "li.ch-row:has(a.ch-link)",
		wantTitle: "I Took Over the Demonic Ancestor",
		wantNums:  []float64{48, 47},
		wantPages: []string{
			"BASE/api/image/e94f7a56ab088fabd8511998be6fb420/001.webp",
			"BASE/api/image/e94f7a56ab088fabd8511998be6fb420/002.webp",
		},
	}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { checkPlainHTMLEntry(t, c) })
	}
}
