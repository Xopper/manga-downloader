// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// newTestWebComic is a WebComic for site, at a series on a test server
func newTestWebComic(seriesURL string, site *webComicSite) *WebComic {
	return &WebComic{Grabber: &Grabber{URL: seriesURL, Settings: &Settings{}}, site: site}
}

// wcLabels lists chapters' labels
func wcLabels(chapters Filterables) string {
	var labels []string
	for _, ch := range chapters {
		labels = append(labels, ChapterLabel(ch))
	}
	return strings.Join(labels, "|")
}

func TestWebComicTestByHost(t *testing.T) {
	for _, u := range []string{"https://xkcd.com", "https://www.questionablecontent.net/archive.php", "https://w.comicfury.com/read/x/"} {
		if ok, _ := NewWebComic(&Grabber{URL: u}).Test(); !ok {
			t.Errorf("Test(%s) = false", u)
		}
	}
	if ok, _ := NewWebComic(&Grabber{URL: "https://example.com/xkcd.com"}).Test(); ok {
		t.Error("Test matched a host named in the path")
	}
}

func TestWebComicFlatArchive(t *testing.T) {
	// xkcd's shape: an archive page apart from the series URL, newest first,
	// numbered in the links
	srv := wcTestServer(t, map[string]string{
		"/archive/": `<div id="middleContainer"><a href="/3/" title="2026-10-16">Three</a><a href="/2/">Two</a><a href="/1/">One</a></div>`,
		"/2/":       `<div id="comic"><img src="//imgs.test/two.png" title="alt"></div>`,
	})
	site := &webComicSite{
		Name: "Test", Archive: "/archive/", Links: "#middleContainer a[href]",
		NumberRe: regexp.MustCompile(`/(\d+)/?$`), NewestFirst: true, Image: "#comic img",
	}
	w := newTestWebComic(srv.URL+"/", site)
	chapters, errs := w.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if got := wcLabels(chapters); got != "Chapter 1 - One|Chapter 2 - Two|Chapter 3 - Three" {
		t.Errorf("chapters = %q", got)
	}
	ch, err := w.FetchChapter(chapters[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Pages) != 1 || ch.Pages[0].URL != "http://imgs.test/two.png" {
		t.Errorf("pages = %+v", ch.Pages)
	}
}

func TestWebComicPagedArchive(t *testing.T) {
	// commitstrip.com's shape: a year's archive, newest first, over pages
	srv := wcTestServer(t, map[string]string{
		"/2017/":        `<div class="excerpt"><a href="/2017/12/d/">D</a></div><div class="excerpt"><a href="/2017/11/c/">C</a></div><a class="nextpostslink" href="/2017/page/2/">»</a>`,
		"/2017/page/2/": `<div class="excerpt"><a href="/2017/02/b/">B</a></div><div class="excerpt"><a href="/2017/01/a/">A</a></div><a class="nextpostslink" href="/2017/">«</a>`,
	})
	site := &webComicSite{Links: ".excerpt a[href]", Next: "a.nextpostslink", NewestFirst: true}
	chapters, errs := newTestWebComic(srv.URL+"/2017/", site).FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if got := wcLabels(chapters); got != "Chapter 1 - A|Chapter 2 - B|Chapter 3 - C|Chapter 4 - D" {
		t.Errorf("chapters = %q", got)
	}
}

func TestGunnerkriggChapters(t *testing.T) {
	doc := docFromHTML(t, `<div class="chapters">
		<a class="chapter_button" href="../?p=1"><strong>Chapter 1: The Shadow and the Robot</strong></a>
		<select name="page"><option value='1'>Page 1</option><option value='2'>Page 2</option></select>
		<a class="chapter_button" href="../?p=17"><strong>Chapter 2: Schoolwork</strong></a>
		<select name="page"><option value='17'>Page 1</option></select>
	</div>`)
	entries, _ := gunnerkriggChapters(doc)
	if len(entries) != 2 || entries[1].Number != 2 || entries[1].Pictures[0] != "https://www.gunnerkrigg.com/comics/00000017.jpg" || len(entries[0].Pictures) != 2 {
		t.Errorf("entries = %+v", entries)
	}
}

func TestDuckChapters(t *testing.T) {
	doc := docFromHTML(t, `<select id="page_dropdown"><option value="5847853">Finale - page 679</option><option value="5847852"> - page 678</option></select>`)
	doc.Url, _ = url.Parse("https://www.theduckwebcomics.com/Golden_Spiral/5847853/")
	entries, _ := duckChapters(doc)
	if len(entries) != 2 || entries[0].Number != 679 || entries[0].Title != "Finale" || entries[1].Strips[0] != "https://www.theduckwebcomics.com/Golden_Spiral/5847852/" {
		t.Errorf("entries = %+v", entries)
	}
}

func TestComicFuryChapters(t *testing.T) {
	srv := wcTestServer(t, map[string]string{
		"/read/hypoxia/archive": `<a href="/comicprofile.php?url=hypoxia">Hypoxia</a>
			<a href="/read/hypoxia/archive/chapter/45866">Chapter 1</a><a href="/read/hypoxia/archive/chapter/45867">Chapter 2</a>`,
		"/read/hypoxia/archive/chapter/45867": `<a href="/read/hypoxia/comics/first">first</a><a href="/read/hypoxia/comics/9">9</a><a href="/read/hypoxia/comics/10">10</a>`,
		"/read/hypoxia/comics/9":              `<img src="https://img.comicfury.com/comics/1/9.png">`,
		"/read/hypoxia/comics/10":             `<img src="https://img.comicfury.com/comics/1/10.png">`,
	})
	site := webComicSites[webComicSiteIndex(t, "comicfury.com")]
	site.ArchiveURL = func(series *url.URL) string { return srv.URL + "/read/hypoxia/archive" }
	w := newTestWebComic(srv.URL+"/read/hypoxia/comics/9", &site)
	if title, _ := w.FetchTitle(); title != "Hypoxia" {
		t.Errorf("title = %q", title)
	}
	chapters, errs := w.FetchChapters()
	if len(errs) > 0 || wcLabels(chapters) != "Chapter 1|Chapter 2" {
		t.Fatalf("chapters = %q, %v", wcLabels(chapters), errs)
	}
	ch, err := w.FetchChapter(chapters[1])
	if err != nil || len(ch.Pages) != 2 || ch.Pages[1].URL != "https://img.comicfury.com/comics/1/10.png" {
		t.Errorf("chapter = %+v, %v", ch, err)
	}
}

// webComicSiteIndex finds the webComicSites entry of a host
func webComicSiteIndex(t *testing.T, host string) int {
	t.Helper()
	for i, s := range webComicSites {
		for _, h := range s.Hosts {
			if h == host {
				return i
			}
		}
	}
	t.Fatalf("no site for %s", host)
	return -1
}

func TestGalleryList(t *testing.T) {
	// kiutaku.com's pager shows a few pages and links the post itself
	doc := docFromHTML(t, `<a class="pagination-link" href="/7207">1</a><a class="pagination-link" href="/7207?page=2">2</a><a class="pagination-link" href="/7207?page=4">4</a><a class="pagination-link" href="/tag/x?page=9">9</a>`)
	doc.Url, _ = url.Parse("https://kiutaku.com/7207")
	entries, _ := galleryList("a.pagination-link")(doc)
	want := "https://kiutaku.com/7207 https://kiutaku.com/7207?page=2 https://kiutaku.com/7207?page=3 https://kiutaku.com/7207?page=4"
	if len(entries) != 1 || strings.Join(entries[0].Strips, " ") != want {
		t.Errorf("entries = %+v", entries)
	}
}

func TestHoloearthChapters(t *testing.T) {
	doc := docFromHTML(t, `<a href="/en/alt/holonometria/manga/vestadecooking/ep1/">Next</a>
		<a href="/en/alt/holonometria/manga/vestadecooking/ep0/">【Chapter 0】</a>
		<a href="/en/alt/holonometria/manga/vestadecooking/ep1/">【Chapter 1】</a>
		<a href="/en/alt/holonometria/manga/other/ep3/">other</a>`)
	doc.Url, _ = url.Parse("https://holoearth.com/en/alt/holonometria/manga/vestadecooking/ep0/")
	entries, _ := holoearthChapters(doc)
	if len(entries) != 2 || entries[0].Number != 0 || !entries[0].HasNumber || entries[1].Number != 1 {
		t.Errorf("entries = %+v", entries)
	}
	if n := wcFileNumber("https://holoearth.com/comic/vestadecooking/0/en/img_4.png"); n != 4 {
		t.Errorf("wcFileNumber = %d", n)
	}
}

func TestSchlockChapters(t *testing.T) {
	doc := docFromHTML(t, `
		<div class="archive-book"><h4><a href="/2000-06-12">Book 1: The Tub of Happiness</a></h4>
			<ul class="chapters"><li><a href="/2000-06-12">Meet The Mercenaries</a></li><li><a href="/2000-06-14">Diplomatic Pouch</a></li></ul></div>
		<div class="archive-book"><h4><a href="/2000-06-16">Book 2: The Teraport Wars</a></h4>
			<ul class="chapters"><li><a href="/2000-06-16">Part I</a></li></ul></div>`)
	doc.Url, _ = url.Parse("https://www.schlockmercenary.com/archives/")
	series, _ := url.Parse("https://www.schlockmercenary.com/2000-06-13")
	entries, err := schlockChapters(series, doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || strings.Join(entries[1].Strips, " ") != "https://www.schlockmercenary.com/2000-06-14 https://www.schlockmercenary.com/2000-06-15" {
		t.Errorf("entries = %+v", entries)
	}
	if got := schlockTitle(series, doc); got != "Schlock Mercenary - Book 1: The Tub of Happiness" {
		t.Errorf("title = %q", got)
	}
}
