// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// plainHTMLTemplateCases are trimmed series and reader pages of the theme
// families served by a PlainHTML entry with no code of their own
var plainHTMLTemplateCases = []struct {
	name   string
	rows   string // the entry expected to match
	series string
	reader string
	title  string
	// first is the first row's number and URL path
	first     float64
	firstPath string
	chapters  int
	pages     []string
}{
	{
		name: "topmanhua.fan (ZinManga, list in the HTML)",
		rows: "div.wp-manga-chapter",
		series: `<div class="post-title"><div><h1 class="post-title font-title">
                    Last Love
                </h1></div></div>
<div class="wp-manga-chapter manga-chapter-id-47"><span>
<a href="/manga/last-love/chapter-46" data-id="47">Chapter 46</a></span><span> 0 </span></div>
<div class="wp-manga-chapter manga-chapter-id-46"><span>
<a href="/manga/last-love/chapter-45" data-id="46">Chapter 45</a></span></div>`,
		reader: `<div class="reading-content"><div class='page-break no-gaps' ><img id='manga-image-0'
 class='wp-manga-chapter-img' src='https://cdn3.zinmanga1.com/x/30/0.webp' data-src='https://cdn3.zinmanga1.com/x/30/0.webp'></div></div>`,
		title: "Last Love", first: 46, firstPath: "/manga/last-love/chapter-46", chapters: 2,
		pages: []string{"https://cdn3.zinmanga1.com/x/30/0.webp"},
	},
	{
		name: "manga18fx.com",
		rows: "ul.row-content-chapter li",
		series: `<div class="post-title"><h1>
                I Got No Boy            </h1></div>
<ul class="row-content-chapter"><li class="a-h">
<a class="chapter-name text-nowrap mycover" href="/manga/i-got-no-boy/chapter-22" title="I Got No Boy Chapter 22">Chapter 22</a>
<span class="chapter-time text-nowrap" title="">04 Nov 20</span></li>
<li class="a-h"><a class="chapter-name text-nowrap" href="/manga/i-got-no-boy/chapter-21">Chapter 21</a></li></ul>`,
		reader: `<div class="logo"><img src="/images/manga18fx.png" alt="Read Manga Online in English Free"></div>
<div class="read-content"><div class="kadx"><ins class="eas6a97888e20"></ins></div>
<div class="page-break"><img class="loading p1" data-src="https://img01.manga18fx.com/uploads/619/20/1-200.jpg" src="https://img01.manga18fx.com/uploads/619/20/1-200.jpg" alt="Page 1"/></div>
<div class="page-break"><img class="loading p2" data-src="https://img01.manga18fx.com/uploads/619/20/2-200.jpg" src="https://img01.manga18fx.com/uploads/619/20/2-200.jpg" alt="Page 2"/></div></div>`,
		title: "I Got No Boy", first: 22, firstPath: "/manga/i-got-no-boy/chapter-22", chapters: 2,
		pages: []string{"https://img01.manga18fx.com/uploads/619/20/1-200.jpg", "https://img01.manga18fx.com/uploads/619/20/2-200.jpg"},
	},
	{
		name: "manhwaz.cc",
		rows: ".release-list a.release-row",
		series: `<h1>I Have A Billion Achievements!</h1>
<div class="release-list" data-release-list> <a class="release-row" href="/webtoon/comizy-vnaw7dj4/chapter-21"> <strong>Chapter 21</strong> <span>0 views</span> <time>Read now</time> </a>
<a class="release-row concealed" href="/webtoon/comizy-vnaw7dj4/chapter-20"> <strong>Chapter 20</strong> </a></div>`,
		reader: `<img src="/assets/logo.png" alt="ManhwaZ">
<div class="reader-pages"><figure class="reader-page-image" id="page-1"><img src="https://api.manhwaz.cc/api/v1/source/comizy?url=https%3A%2F%2Fx4.cmzcdn.org%2Fe%2Fa.webp" alt="page 1"></figure></div>`,
		title: "I Have A Billion Achievements!", first: 21, firstPath: "/webtoon/comizy-vnaw7dj4/chapter-21", chapters: 2,
		pages: []string{"https://api.manhwaz.cc/api/v1/source/comizy?url=https%3A%2F%2Fx4.cmzcdn.org%2Fe%2Fa.webp"},
	},
	{
		name: "mangafreak.me",
		rows: ".manga_series_list a.chapter-link",
		series: `<div class="manga_series_data"><h1>Hellsing</h1><div>Alternative Title: Hellsing Cross Fire</div></div>
<div class="series_sub_chapter_list"><a href="/Read1_Hellsing_95">Chapter 95</a></div>
<div class="manga_series_list"><table><tr><td><a class="chapter-link" href="/Read1_Hellsing_1">Chapter 1</a></td>
<td><a download="Hellsing 1.zip" href="https://images.mangafreak.me/downloads/Hellsing_1" class="download-link"></a></td></tr>
<tr><td><a class="chapter-link" href="/Read1_Hellsing_2">Chapter 2</a></td></tr></table></div>`,
		reader: `<img id="gohere" src="https://images.mangafreak.me/mangas/hellsing/hellsing_90/hellsing_90_1.jpg" alt="Hellsing Chapter 90 Page 1">`,
		title:  "Hellsing", first: 1, firstPath: "/Read1_Hellsing_1", chapters: 2,
		pages: []string{"https://images.mangafreak.me/mangas/hellsing/hellsing_90/hellsing_90_1.jpg"},
	},
	{
		name: "mangack.com (Ifenzi)",
		rows: "ul.chapterslist li",
		series: `<h1 class="entry-title">I Reincarnated As The Crazed Heir</h1>
<a href="/chapter/i-reincarnated-as-the-crazed-heir-chapter-0/" class="First_Chapter">First Chapter</a>
<ul class="chapterslist"><li><a href="/chapter/i-reincarnated-as-the-crazed-heir-chapter-200/" class="title">
                CHAPTER 200                            </a><div><span class="entry-date">2 months ago</span></div></li>
<li><a href="/chapter/i-reincarnated-as-the-crazed-heir-chapter-199/" class="title">CHAPTER 199</a></li></ul>`,
		reader: `<div class="entry-content"><div class="code-block code-block-1">
<div class="separator"><img decoding="async" class="aligncenter" src="https://cdn.black-clover.org/file/mookitosh/x/chapter-199/1.webp" alt="chapter 199" /></div></div></div>`,
		title: "I Reincarnated As The Crazed Heir", first: 200, firstPath: "/chapter/i-reincarnated-as-the-crazed-heir-chapter-200/", chapters: 2,
		pages: []string{"https://cdn.black-clover.org/file/mookitosh/x/chapter-199/1.webp"},
	},
	{
		name: "read-horimiya.online (NEW theme)",
		rows: "#chapter-list a.chapter-list-item",
		series: `<div id="manga-info-right"><h1>Horimiya</h1><h2>Hori-san and Miyamura-kun</h2></div>
<div id="chapter-list"><div class="chapter-start-end"><a href="/comic/horimiya-chapter-1/"><span>First Chapter</span><br>Chapter 1</a></div>
<a href="/comic/horimiya-chapter-128/" aria-label="Horimiya, Chapter 128" class="d-block chapter-list-item">
<span class="chapter-name">Chapter 128</span><span class="chapter-date">August 27, 2026</span></a>
<a href="/comic/horimiya-chapter-127/" class="d-block chapter-list-item"><span class="chapter-name">Chapter 127</span></a></div>`,
		reader: `<div class="images-container"><p><img lazyload src='https://laiond.com/images/Horimiya - Manga/sGbIhZ.jpg' alt='Horimiya, Chapter 127 image 01'></p></div>`,
		title:  "Horimiya", first: 128, firstPath: "/comic/horimiya-chapter-128/", chapters: 2,
		pages: []string{"https://laiond.com/images/Horimiya - Manga/sGbIhZ.jpg"},
	},
	{
		name: "18porncomic.com (Manga18)",
		rows: "a.chapter_num",
		series: `<div class="detail_name"><h1>I Need You, Noona</h1></div>
<ul><li><div class="item"><a title="I Need You, Noona Chapter 24 - The End" href="/comic/i-need-you-noona/chapter-24-the-end" class="chapter_num"># Chapter 24 - The End
</a><p class="chapter_info">04-08-2022</p></div></li>
<li><div class="item"><a href="/comic/i-need-you-noona/chapter-23" class="chapter_num"># Chapter 23</a></div></li></ul>`,
		reader: `<img src="/Porn.png" alt="">
<script>var slides_p_path = ["aHR0cHM6Ly9pbWcuMThwb3JuY29taWMuY29tL3VwbG9hZHMvbWFuZ2EvaS1uZWVkLXlvdS1ub29uYS9jaGFwdGVycy9jaGFwdGVyLTIxLzAxLnBuZw==","aHR0cHM6Ly9pbWcuMThwb3JuY29taWMuY29tL3VwbG9hZHMvbWFuZ2EvaS1uZWVkLXlvdS1ub29uYS9jaGFwdGVycy9jaGFwdGVyLTIxLzAyLnBuZw==",];
for (var i = 0; i < slides_p_path.length; i++) { html += '<img alt="" src=\'' + atob(slides_p_path[i]) + '\' class="img-responsive image-chapter" />'; }</script>`,
		title: "I Need You, Noona", first: 24, firstPath: "/comic/i-need-you-noona/chapter-24-the-end", chapters: 2,
		pages: []string{
			"https://img.18porncomic.com/uploads/manga/i-need-you-noona/chapters/chapter-21/01.png",
			"https://img.18porncomic.com/uploads/manga/i-need-you-noona/chapters/chapter-21/02.png",
		},
	},
	{
		name: "comivex.com",
		rows: "ul.ch-list li.ch-item",
		series: `<h1 class="md-title">I’m Really Not A Supervillain</h1>
<a href="/read/4144/0-eng-li/" class="btn-read">Read first</a>
<ul class="ch-list"><li class="ch-item" x-show="search === '' || '309'.includes(search)">
<a href="/read/4144/309-eng-li/" class="ch-link"><span class="ch-num">Chapter 309</span>
<span class="ch-meta"><span class="ch-views"><i class="fas fa-eye"></i> 170</span><span class="ch-date">1 year ago</span></span></a></li>
<li class="ch-item"><a href="/read/4144/308-eng-li/" class="ch-link"><span class="ch-num">Chapter 308</span></a></li></ul>`,
		reader: `<section id="chapter-images" aria-label="Chapter pages"><div class="page-wrapper" id="page-1">
<img
                        src="https://imgsrv5.com/mg1/fastcdn/cdn_mangaraw/x/chapter-300/0.jpg"
                        loading="lazy" alt="Ch.300 p1"></div></section>`,
		title: "I’m Really Not A Supervillain", first: 309, firstPath: "/read/4144/309-eng-li/", chapters: 2,
		pages: []string{"https://imgsrv5.com/mg1/fastcdn/cdn_mangaraw/x/chapter-300/0.jpg"},
	},
}

func TestPlainHTMLTemplates(t *testing.T) {
	for _, tc := range plainHTMLTemplateCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("<html><body>" + tc.series + "</body></html>"))
			}))
			t.Cleanup(srv.Close)

			m := NewPlainHTML(&Grabber{URL: srv.URL + "/series", Settings: &Settings{}})
			if ok, err := m.Test(); err != nil || !ok {
				t.Fatalf("Test() = %v, %v; want the series to match", ok, err)
			}
			if m.site.Rows != tc.rows {
				t.Fatalf("matched the %q entry, want %q", m.site.Rows, tc.rows)
			}
			if title, _ := m.FetchTitle(); title != tc.title {
				t.Errorf("FetchTitle() = %q, want %q", title, tc.title)
			}
			chapters, errs := m.FetchChapters()
			if len(errs) > 0 || len(chapters) != tc.chapters {
				t.Fatalf("FetchChapters() = %d chapters, %v; want %d", len(chapters), errs, tc.chapters)
			}
			first := chapters[0].(*PlainHTMLChapter)
			u, _ := url.Parse(first.URL)
			if first.Number != tc.first || u.Path != tc.firstPath {
				t.Errorf("first chapter = %v %q, want %v %q", first.Number, u.Path, tc.first, tc.firstPath)
			}

			chapter := m.chapterFromDoc(first, docFromHTML(t, "<html><body>"+tc.reader+"</body></html>"))
			var got []string
			for _, p := range chapter.Pages {
				got = append(got, p.URL)
			}
			if !reflect.DeepEqual(got, tc.pages) {
				t.Errorf("pages = %v, want %v", got, tc.pages)
			}
		})
	}
}

func TestSlidesPagePaths(t *testing.T) {
	// a non-URL entry (a data placeholder) is left out
	html := `var slides_p_path = ["aHR0cHM6Ly9leGFtcGxlLmNvbS8xLnBuZw==", "bm90IGEgdXJs"];`
	if got := slidesPagePaths(html); !reflect.DeepEqual(got, []string{"https://example.com/1.png"}) {
		t.Errorf("slidesPagePaths() = %v", got)
	}
	if got := slidesPagePaths(strings.ReplaceAll(html, "slides_p_path", "pages")); got != nil {
		t.Errorf("slidesPagePaths() without the array = %v, want nil", got)
	}
}
