// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// ceFlatDropdown is the plugin's widget as killsixbilliondemons.com has it:
// flat, with counts. Book 1 only groups its chapters (17+20 strips), Book 2
// has a cover of its own (1+5+6 < 13).
const ceFlatDropdown = `<div class="widget ceo_comic_archive_dropdown_widget"><select name="chapter" id="chapter">
	<option value="0">Select Chapter</option>
	<option class="level-0" value="https://c.test/chapter/book-1/">Book 1: Demons&nbsp;&nbsp;(37)</option>
	<option class="level-0" value="https://c.test/chapter/1/">1&nbsp;&nbsp;(17)</option>
	<option class="level-0" value="https://c.test/chapter/2/">2&nbsp;&nbsp;(20)</option>
	<option class="level-0" value="https://c.test/chapter/book-2/">Book 2: Names&nbsp;&nbsp;(13)</option>
	<option class="level-0" value="https://c.test/chapter/1-names/">1&nbsp;&nbsp;(5)</option>
	<option class="level-0" value="https://c.test/chapter/2-names/">2&nbsp;&nbsp;(6)</option>
</select></div>`

func TestComicEaselFlatDropdown(t *testing.T) {
	options := ceDropdown(docFromHTML(t, ceFlatDropdown))
	if len(options) != 6 || options[0].name != "Book 1: Demons" || options[0].count != 37 {
		t.Fatalf("ceDropdown = %+v", options)
	}

	chapters := ceOptionChapters(options)
	want := []struct {
		label  string
		volume int
		source string
	}{
		{"Chapter 1 - Book 1: Demons - 1", 1, "https://c.test/chapter/1/"},
		{"Chapter 2 - Book 1: Demons - 2", 1, "https://c.test/chapter/2/"},
		{"Chapter 3 - Book 2: Names", 2, "https://c.test/chapter/book-2/"},
		{"Chapter 4 - Book 2: Names - 1", 2, "https://c.test/chapter/1-names/"},
		{"Chapter 5 - Book 2: Names - 2", 2, "https://c.test/chapter/2-names/"},
	}
	if len(chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d (Book 1 only groups its chapters)", len(chapters), len(want))
	}
	for i, w := range want {
		ch := chapters[i].(*ComicEaselChapter)
		if ChapterLabel(ch) != w.label || ch.Volume != w.volume || ch.Archive != w.source {
			t.Errorf("chapter %d = %q vol %d %q, want %q vol %d %q", i, ChapterLabel(ch), ch.Volume, ch.Archive, w.label, w.volume, w.source)
		}
	}
}

func TestComicEaselBlockDropdown(t *testing.T) {
	// dumbingofage.com: the categories block for the chapters taxonomy, by
	// level, whose values are slugs
	doc := docFromHTML(t, `<div class="wp-block-categories-taxonomy-chapters"><select name="chapters">
		<option value="-1">Select Chapter</option>
		<option class="level-0" value="book-1">Book 01</option>
		<option class="level-1" value="01-move-in-day">&nbsp;&nbsp;&nbsp;01 &#8211; Move-In Day</option>
		<option class="level-1" value="02-uphill">&nbsp;&nbsp;&nbsp;02 &#8211; Uphill</option>
		<option class="level-0" value="book-2">Book 02</option>
		<option class="level-1" value="01-pajama-jeans">&nbsp;&nbsp;&nbsp;01 &#8211; Pajama Jeans</option>
	</select></div>`)
	chapters := ceOptionChapters(ceDropdown(doc))
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want the 3 leaves", len(chapters))
	}
	last := chapters[2].(*ComicEaselChapter)
	if last.Slug != "01-pajama-jeans" || last.Volume != 2 || last.Number != 3 || ChapterLabel(last) != "Chapter 3 - 01 – Pajama Jeans" {
		t.Errorf("last chapter = %+v %q", last, ChapterLabel(last))
	}
}

func TestComicEaselFirstStripDropdown(t *testing.T) {
	// buttsmithy.com: options lead to each chapter's first strip, in no order
	doc := docFromHTML(t, `<select name="chapter" id="chapter">
		<option class="level-0" value="https://b.test/archives/comic/p-1009">Chapter 15.5</option>
		<option class="level-0" value="https://b.test/archives/comic/misc">Misc</option>
		<option class="level-0" value="https://b.test/archives/comic/p-1">Chapter 1</option>
	</select>`)
	chapters := ceOptionChapters(ceDropdown(doc))
	got := []string{}
	for _, c := range chapters {
		ch := c.(*ComicEaselChapter)
		got = append(got, fmt.Sprintf("%v %s %s", ch.Number, ChapterLabel(ch), ch.First))
	}
	want := "15.5 Chapter 15.5 https://b.test/archives/comic/p-1009|0 Misc https://b.test/archives/comic/misc|1 Chapter 1 https://b.test/archives/comic/p-1"
	if strings.Join(got, "|") != want {
		t.Errorf("chapters = %q", got)
	}
}

func TestComicEaselArchiveShortcode(t *testing.T) {
	doc := docFromHTML(t, `<div class="entry-content">
		<div class="comic-archive-chapter-wrap"><h3 class="comic-archive-chapter">14. Zam's Amporiam</h3>
			<div class="comic-archive-list-wrap">
				<div class="comic-list"><span class="comic-archive-title"><a href="https://i.test/comic/p-64/">p.64</a></span></div>
				<div class="comic-list"><span class="comic-archive-title"><a href="https://i.test/comic/p-65/">p.65</a></span></div>
			</div></div>
		<div class="comic-archive-chapter-wrap"><h3 class="comic-archive-chapter">15. Trunkajunk</h3>
			<div class="comic-list"><span class="comic-archive-title"><a href="https://i.test/comic/p-68/">p.68</a></span></div></div>
	</div>`)
	chapters := ceArchiveChapters([]*goquery.Document{doc})
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters", len(chapters))
	}
	first := chapters[0].(*ComicEaselChapter)
	if first.Number != 14 || ChapterLabel(first) != "Chapter 14 - Zam's Amporiam" || strings.Join(first.Strips, " ") != "https://i.test/comic/p-64/ https://i.test/comic/p-65/" {
		t.Errorf("first chapter = %+v %q", first, ChapterLabel(first))
	}
}

func TestComicEaselTest(t *testing.T) {
	srv := wcTestServer(t, map[string]string{
		"/ce/":   `<html><body class="home">` + ceFlatDropdown + `</body></html>`,
		"/cp/":   `<html><head><link rel="stylesheet" href="/wp-content/themes/comicpress-3c/style.css"></head><body><table><tr><td class="archive-title"><a href="/2000/01/01/a/">A</a></td></tr></table></body></html>`,
		"/blog/": `<html><body class="home"><div id="comic"></div><td class="archive-title"><a href="/x">x</a></td></body></html>`,
	})
	for path, want := range map[string]bool{"/ce/": true, "/cp/": true, "/blog/": false} {
		c := NewComicEasel(&Grabber{URL: srv.URL + path, Settings: &Settings{}})
		if ok, err := c.Test(); ok != want || err != nil {
			t.Errorf("Test(%s) = %v, %v; want %v", path, ok, err, want)
		}
	}
}

func TestComicEaselWalkArchive(t *testing.T) {
	post := func(id int, slug, link, picture string) string {
		thumb := ""
		if picture != "" {
			thumb = `<p class="comic-thumbnail-in-archive"><a href="` + link + `"><img src="` + picture + `-500x773.jpg" srcset="` + picture + `.jpg 800w, ` + picture + `-194x300.jpg 194w"></a></p>`
		}
		return fmt.Sprintf(`<article id="post-%d" class="post-%d comic type-comic chapters-%s"><h2 class="post-title"><a href="%s">t</a></h2>%s</article>`, id, id, slug, link, thumb)
	}
	// newest first, and the book lists its chapter's strips with its own cover
	srv := wcTestServer(t, map[string]string{
		"/chapter/book-3/":        "<html><body>" + post(30, "1-book-3", "/comic/c/", "/up/c") + post(20, "book-3", "/comic/cover/", "/up/cover") + "</body></html>",
		"/chapter/book-3/page/2/": "<html><body>" + post(10, "1-book-3", "/comic/a/", "/up/a") + "</body></html>",
		"/chapter/1/":             "<html><body>" + post(11, "1", "/comic/x/", "/up/x") + post(12, "1", "/comic/y/", "") + "</body></html>",
		"/comic/x/":               `<html><body><div id="comic"><img src="/up/x.png"></div></body></html>`,
		"/comic/y/":               `<html><body><div id="comic"><img src="/up/y.png"></div></body></html>`,
	})
	c := NewComicEasel(&Grabber{URL: srv.URL + "/", Settings: &Settings{}})

	pages, err := c.walkArchive(srv.URL+"/chapter/book-3/", "book-3")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].URL != srv.URL+"/up/cover.jpg" {
		t.Errorf("book pages = %+v, want its cover only", pages)
	}

	// a strip without its picture in the archive is read from its page
	pages, err = c.walkArchive(srv.URL+"/chapter/1/", "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[1].URL != srv.URL+"/up/y.png" {
		t.Errorf("chapter pages = %+v", pages)
	}
}

func TestComicEaselComicPressArchive(t *testing.T) {
	year := func(y, strips string) string {
		return `<html><head><link rel="stylesheet" href="/wp-content/themes/comicpress/style.css"></head><body>
			<h3 class="year-title">` + y + `</h3><div class="archive-yearlist"><a href="/archive/?archive_year=2010">2010</a> | <a href="/archive/?archive_year=2011">2011</a></div>
			<table class="month-table">` + strips + `</table></body></html>`
	}
	row := func(link, title string) string {
		return `<tr><td class="archive-date">Jan 1</td><td class="archive-title"><a href="` + link + `">` + title + `</a></td></tr>`
	}
	srv := wcTestServer(t, map[string]string{
		"/archive/":                   year("2011", row("/comic/3/", "Three")),
		"/archive/?archive_year=2010": year("2010", row("/comic/1/", "One")+row("/comic/2/", "Two")),
		"/archive/?archive_year=2011": year("2011", row("/comic/3/", "Three")),
	})
	c := NewComicEasel(&Grabber{URL: srv.URL + "/archive/", Settings: &Settings{}})
	chapters, errs := c.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	var got []string
	for _, ch := range chapters {
		got = append(got, ChapterLabel(ch))
	}
	if strings.Join(got, "|") != "Chapter 1 - One|Chapter 2 - Two|Chapter 3 - Three" {
		t.Errorf("chapters = %q", got)
	}
}
