// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"html"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// gallerySeries identifies the series at pageURL and returns its title, its
// chapters and the first chapter's page URLs
func gallerySeries(t *testing.T, pageURL string) (string, Filterables, []string) {
	t.Helper()
	g := NewGallery(&Grabber{URL: pageURL, Settings: &Settings{}})
	if ok, err := g.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want true", ok, err)
	}
	title, err := g.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle: %v", err)
	}
	chapters, errs := g.FetchChapters()
	if len(errs) > 0 || len(chapters) == 0 {
		t.Fatalf("FetchChapters = %d chapters, %v", len(chapters), errs)
	}
	chapter, err := g.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	var urls []string
	for _, p := range chapter.Pages {
		urls = append(urls, p.URL)
	}
	return title, chapters, urls
}

// galleryLabels returns the chapters' numbers and labels
func galleryLabels(chapters Filterables) ([]float64, []string) {
	var numbers []float64
	var labels []string
	for _, c := range chapters {
		numbers = append(numbers, c.GetNumber())
		labels = append(labels, ChapterLabel(c))
	}
	return numbers, labels
}

func TestGallerySeries(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		pages   map[string]func(string) string
		title   string
		numbers []float64
		labels  []string
		want    func(base string) []string
	}{
		{
			name: "hentaihere",
			path: "/original/25844-shunned",
			pages: map[string]func(string) string{
				"/original/25844-shunned": func(base string) string {
					return `<h1 id="book-title" title="Shunned">Shunned</h1><select id="preview-chapter">
<option value="57028" data-read="` + base + `/reader/25844-shunned/57028/page-1" selected>Chapter 9 - Shunned 9 (latest)</option>
<option value="57001" data-read="` + base + `/reader/25844-shunned/57001/page-1">Chapter 8 - Shunned 8</option></select>`
				},
				"/reader/25844-shunned/57028/page-1": func(string) string {
					return `<script type="application/json" data-reader-data>{"images":["https://cdn.example.com/hentai/25844/9/hcdn0001.jpg","https://cdn.example.com/hentai/25844/9/hcdn0002.jpg"],"book":{"id":25844}}</script>`
				},
			},
			title:   "Shunned",
			numbers: []float64{9, 8},
			labels:  []string{"Chapter 9 - Shunned 9", "Chapter 8 - Shunned 8"},
			want: func(string) []string {
				return []string{"https://cdn.example.com/hentai/25844/9/hcdn0001.jpg", "https://cdn.example.com/hentai/25844/9/hcdn0002.jpg"}
			},
		},
		{
			name: "hentaitnt (yupmanga)",
			path: "/secret.html",
			pages: map[string]func(string) string{
				"/secret.html": func(string) string {
					return `<link rel="stylesheet" href="/wp-content/themes/yupmanga/style.css"><h1>Secret &#8230; ♡</h1><input id="post_manga_id" name="post_manga_id" type="hidden" value="94785">`
				},
				"/wp-admin/admin-ajax.php": func(base string) string {
					return `{"success":true,"data":{"html":"<div class=\"comic-card group\" data-chap-id=\"94787\"><a href=\"` + base + `/secret/one-shot.html\" title=\"One Shot\">x</a></div>","current_page":1,"total_pages":1}}`
				},
				"/secret/one-shot.html": func(string) string {
					return `<main id="webtoonContainer"><img class="page-image" src="https://cdn.example.com/secret/one-shot/0.jpg" data-page="1"><img class="page-image" src="https://cdn.example.com/secret/one-shot/1.jpg" data-page="2"></main>`
				},
			},
			title:   "Secret … ♡",
			numbers: []float64{1},
			labels:  []string{"One Shot"},
			want: func(string) []string {
				return []string{"https://cdn.example.com/secret/one-shot/0.jpg", "https://cdn.example.com/secret/one-shot/1.jpg"}
			},
		},
		{
			name: "hentaikun",
			path: "/manga/hentai-manga/mature-wife-17457/",
			pages: map[string]func(string) string{
				"/manga/hentai-manga/mature-wife-17457/": func(base string) string {
					return `<div class="single_title"><h1>Mature Wife</h1></div><table>
<tr><td><a class='readchap' href=" ` + base + `/manga/hentai-manga/mature-wife-17457/chapter-6/">Chapter 6</a></td></tr>
<tr><td><a class='readchap' href=" ` + base + `/manga/hentai-manga/mature-wife-17457/chapter-1-3/">Chapter 1-3</a></td></tr></table>`
				},
				"/manga/hentai-manga/mature-wife-17457/chapter-6/": func(base string) string {
					return `<select><option value='` + base + `/manga/hentai-manga/mature-wife-17457/chapter-6/'>Chapter 6</option></select>
<select><option selected value='` + base + `/manga/hentai-manga/mature-wife-17457/chapter-6/1/'>Page 1</option><option value='` + base + `/manga/hentai-manga/mature-wife-17457/chapter-6/2/'>Page 2</option></select>
<div class="image-show"><img class= "img-responsive center-block image_rin" src="https://s1.example.com/images/english/m/mature wife/166.jpg
" /></div>`
				},
				"/manga/hentai-manga/mature-wife-17457/chapter-6/2/": func(string) string {
					return `<div class="image-show"><img class="image_rin" src=" https://s1.example.com/images/english/m/mature wife/167.jpg" /></div>`
				},
			},
			title:   "Mature Wife",
			numbers: []float64{6, 1},
			want: func(string) []string {
				return []string{"https://s1.example.com/images/english/m/mature%20wife/166.jpg", "https://s1.example.com/images/english/m/mature%20wife/167.jpg"}
			},
		},
		{
			name: "oppai",
			path: "/manhwa",
			pages: map[string]func(string) string{
				"/manhwa": func(base string) string {
					return `<h1 class="white bebas line-3">The Mother By <a href='' class='red'>Cloud Light</a></h1>
<section class="chapters"><a href='` + base + `/page?m=the-mother&c=2' ch-num='2'>2</a><a href='` + base + `/page?m=the-mother&c=1' ch-num='1'>1</a></section>`
				},
				"/page": func(base string) string {
					return `<script>loadImage("` + base + `/img/the-mother/2/1."+filetype+"?v=", document.querySelector(".image-wrap"));
$.ajax({ url: '` + base + `/img/images.php?f-m=the-mother&c=2&v=2026-10-17 03:30:42&upsc=', success: function(data) {}});</script>`
				},
				"/img/images.php": func(string) string { return "3" },
			},
			title:   "The Mother",
			numbers: []float64{2, 1},
			// no 1.jpg on the server, so the chapter is png
			want: func(base string) []string {
				return []string{base + "/img/the-mother/2/1.png", base + "/img/the-mother/2/2.png", base + "/img/the-mother/2/3.png"}
			},
		},
		{
			name: "megatokyo",
			path: "/archive.php",
			pages: map[string]func(string) string{
				"/archive.php": func(string) string {
					return `<title>MegaTokyo - Archives</title><div class="content"><ul>
<li><a title="August 14th, 2000" href="./strip/1">0001 - E3 Nightmare Begins</a></li>
<li><a title="August 16th, 2000" href="./strip/2">0002 - Hello Kitty</a></li></ul></div>`
				},
				"/strip/1": func(base string) string {
					return `<base href="` + base + `/"><span id="strip"><img src="strips/0001.gif"></span>`
				},
			},
			title:   "MegaTokyo",
			numbers: []float64{1, 2},
			labels:  []string{"Chapter 1 - E3 Nightmare Begins", "Chapter 2 - Hello Kitty"},
			want: func(base string) []string {
				return []string{base + "/strips/0001.gif"}
			},
		},
		{
			name: "toonx",
			path: "/adult/therapy",
			pages: map[string]func(string) string{
				"/adult/therapy": func(string) string {
					return `<h1>Adicktion Therapy</h1><script>self.__next_f.push([1,"{\"first\":{\"id\":10010713,\"chapterNumber\":\"1.00\",\"slug\":\"chapter-1\"},\"latest\":{\"id\":10021623,\"chapterNumber\":\"7.20\",\"title\":null,\"slug\":\"chapter-7.2\",\"pageCount\":22},\"list\":[{\"id\":10021623,\"chapterNumber\":\"7.20\",\"title\":null,\"slug\":\"chapter-7.2\",\"pageCount\":22},{\"id\":10010713,\"chapterNumber\":\"1.00\",\"title\":\"The Start\",\"slug\":\"chapter-1\",\"pageCount\":6}]}"])</script>`
				},
				"/adult/therapy/chapter/7.2": func(string) string {
					return `<script>self.__next_f.push([1,"{\"id\":10021623,\"slug\":\"chapter-7.2\",\"images\":[{\"id\":2,\"filename\":\"chapters/1/7/002.jpg\",\"width\":1,\"height\":1,\"sortOrder\":2},{\"id\":1,\"filename\":\"chapters/1/7/001.jpg\",\"width\":1,\"height\":1,\"sortOrder\":1}]}"])</script>`
				},
			},
			title:   "Adicktion Therapy",
			numbers: []float64{7.2, 1},
			labels:  []string{"Chapter 7.2", "Chapter 1 - The Start"},
			want: func(base string) []string {
				return []string{base + "/uploads/chapters/1/7/001.jpg", base + "/uploads/chapters/1/7/002.jpg"}
			},
		},
		{
			name: "xcomic",
			path: "/title/4brda",
			pages: map[string]func(string) string{
				"/title/4brda": func(string) string {
					return `<h1><a href="/title/4brda">Martial God</a></h1><a href="/source/lk2ang">Martial God</a><a href="/source/lk2ang">Chapter 40</a><script type="qwik/state">{}</script>`
				},
				"/source/lk2ang": func(string) string {
					return `<a href="/chapter/6ex4nw4">Chapter 40</a><a href="/chapter/xd7apjd">Ch-39</a>`
				},
				"/chapter/6ex4nw4": func(string) string {
					return `<script type="qwik/state">{"objs":[0,"sser_follow",3,1],0,"imageUrls",4,[0,"https://i02.example.org/_f/a.webp",0,"https://i03.example.org/_f/b.webp"],0,"next","https://other.example.org/x"}</script>`
				},
			},
			title:   "Martial God",
			numbers: []float64{40, 39},
			labels:  []string{"Chapter 40", "Chapter 39"},
			want: func(string) []string {
				return []string{"https://i02.example.org/_f/a.webp", "https://i03.example.org/_f/b.webp"}
			},
		},
		{
			name: "inertia",
			path: "/manga/hypnotist",
			pages: map[string]func(string) string{
				"/manga/hypnotist": func(string) string {
					return `<div id="app" data-page="` + html.EscapeString(`{"component":"Manga","props":{"manga":{"name":"Hypnotist","slug":"hypnotist"},"chapters":[{"id":159,"name":"Chapter 70","slug":"chapter-70-159"},{"id":3,"name":"Chapter 0 - Prologue","slug":"chapter-0-prologue"}]}}`) + `"></div>`
				},
				"/manga/hypnotist/chapter-70-159": func(string) string {
					return `<div id="app" data-page="` + html.EscapeString(`{"component":"Chapter","props":{"chapterImages":[{"src":"https://min.example.net/a/1.jpg"},{"src":"https://min.example.net/a/2.jpg"}]}}`) + `"></div>`
				},
			},
			title:   "Hypnotist",
			numbers: []float64{70, 0},
			labels:  []string{"Chapter 70", "Chapter 0 - Prologue"},
			want: func(string) []string {
				return []string{"https://min.example.net/a/1.jpg", "https://min.example.net/a/2.jpg"}
			},
		},
		{
			name: "8muses",
			path: "/porn/album/English/Oda-Non",
			pages: map[string]func(string) string{
				"/porn/album/English/Oda-Non": func(string) string {
					return `<script id="ractive-public" type="text/plain">` + eightMusesBlob(`{"album":{"name":"Oda Non","permalink":"English/Oda-Non"},"albums":[{"permalink":"English/Oda-Non/A-Secret-Meeting","displayName":"A Secret Meeting"}],"pictures":[],"page":1,"pages":1}`) + `</script>`
				},
				"/porn/album/English/Oda-Non/A-Secret-Meeting": func(string) string {
					return `<script id="ractive-public" type="text/plain">` + eightMusesBlob(`{"album":{"name":"A Secret Meeting","permalink":"English/Oda-Non/A-Secret-Meeting"},"albums":[],"pictures":[{"name":"001","publicUri":"dLe+fB-x"},{"name":"002","publicUri":"aB+c-d"}],"page":1,"pages":0}`) + `</script>`
				},
			},
			title:   "Oda Non",
			numbers: []float64{1},
			labels:  []string{"A Secret Meeting"},
			want: func(base string) []string {
				return []string{base + "/image/fl/dLe+fB-x.jpg", base + "/image/fl/aB+c-d.jpg"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := galleryServe(t, tt.pages)
			title, chapters, urls := gallerySeries(t, srv.URL+tt.path)
			if title != tt.title {
				t.Errorf("title = %q, want %q", title, tt.title)
			}
			numbers, labels := galleryLabels(chapters)
			if !slices.Equal(numbers, tt.numbers) {
				t.Errorf("chapter numbers = %v, want %v", numbers, tt.numbers)
			}
			if tt.labels != nil && !slices.Equal(labels, tt.labels) {
				t.Errorf("chapter labels = %q, want %q", labels, tt.labels)
			}
			if want := tt.want(srv.URL); !slices.Equal(urls, want) {
				t.Errorf("pages = %v, want %v", urls, want)
			}
		})
	}
}

// eightMusesBlob writes JSON as 8muses' page data: a stray prefix, ROT47,
// HTML-escaped
func eightMusesBlob(data string) string {
	b := []byte("P" + data)
	for i, c := range b {
		if c >= '!' && c <= '~' {
			b[i] = '!' + (c-'!'+47)%94
		}
	}
	return html.EscapeString(string(b))
}

// TestGalleryXcomicGroups names each chapter's source as its group
func TestGalleryXcomicGroups(t *testing.T) {
	srv := galleryServe(t, map[string]func(string) string{
		"/title/t": func(string) string {
			return `<h1><a>T</a></h1><a href="/source/aa">x</a><a href="/source/bb">y</a><script type="qwik/state">{}</script>`
		},
		"/source/aa": func(string) string { return `<a href="/chapter/1">Chapter 1</a>` },
		"/source/bb": func(string) string { return `<a href="/chapter/2">Chapter 1</a>` },
	})
	g := NewGallery(&Grabber{URL: srv.URL + "/title/t", Settings: &Settings{}})
	chapters, errs := g.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters = %d chapters, %v; want 2", len(chapters), errs)
	}
	if a, b := chapters[0].(*GalleryChapter).Group, chapters[1].(*GalleryChapter).Group; a != "source #aa" || b != "source #bb" {
		t.Errorf("groups = %q, %q; want source #aa, source #bb", a, b)
	}
}

// TestGalleryOppaiJpg keeps jpg when the chapter's first page is one
func TestGalleryOppaiJpg(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manhwa":
			w.Write([]byte(`<h1 class="white bebas">M By <a>A</a></h1><section class="chapters"><a href='` + srv.URL + `/page?m=m&c=1' ch-num='1'>1</a></section>`))
		case "/page":
			w.Write([]byte(`<script>loadImage("` + srv.URL + `/img/m/1/1."+filetype+"?v=", x); $.ajax({ url: '` + srv.URL + `/img/images.php?f-m=m&c=1', success: 1});</script>`))
		case "/img/images.php":
			w.Write([]byte("1"))
		case "/img/m/1/1.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte(strings.Repeat("x", 10)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, _, urls := gallerySeries(t, srv.URL+"/manhwa")
	if want := []string{srv.URL + "/img/m/1/1.jpg"}; !slices.Equal(urls, want) {
		t.Errorf("pages = %v, want %v", urls, want)
	}
}
