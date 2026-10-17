// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// gallerySeriesEngines are the engines whose items list chapters: adult
// sites whose chapter lists or pages sit in a script, an API or one reader
// page per picture, out of PlainHTML's reach
var gallerySeriesEngines = []galleryEngine{
	{
		// hentaihere.com: the chapters are a <select>, each reader's pages
		// a JSON script
		name: "hentaihere",
		match: func(doc *goquery.Document) bool {
			return doc.Find("h1#book-title").Length() > 0 && doc.Find("select#preview-chapter").Length() > 0
		},
		title:    "h1#book-title",
		chapters: hentaiHereChapters,
		pages:    hentaiHerePages,
	},
	{
		// hentaitnt.net (WordPress, yupmanga theme): chapters come from
		// admin-ajax.php, pages are plain <img>s
		name: "yupmanga",
		match: func(doc *goquery.Document) bool {
			return doc.Find("input#post_manga_id").Length() > 0 &&
				doc.Find(`link[href*="themes/yupmanga"], script[src*="themes/yupmanga"]`).Length() > 0
		},
		title:    "h1",
		chapters: yupMangaChapters,
		pages: func(g *Gallery, c *GalleryChapter) ([]string, error) {
			doc, err := g.docOf(c.URL)
			if err != nil {
				return nil, err
			}
			return g.images(doc, "main#webtoonContainer img.page-image", "src", "data-original-src"), nil
		},
	},
	{
		// hentaikun.com: a gallery or a few chapters as rows, each reader
		// showing one page per URL
		name: "hentaikun",
		match: func(doc *goquery.Document) bool {
			return doc.Find("a.readchap").Length() > 0
		},
		title:    "div.single_title h1",
		chapters: hentaiKunChapters,
		pages:    hentaiKunPages,
	},
	{
		// read.oppai.stream: the reader's script counts the pages through an
		// API and tries each as jpg, then png
		name: "oppai",
		match: func(doc *goquery.Document) bool {
			return doc.Find("section.chapters a[ch-num]").Length() > 0
		},
		titleOf: func(g *Gallery) string {
			h1 := g.doc.Find("h1.bebas").First().Clone()
			h1.Find("a").Remove()
			return strings.TrimSuffix(strings.TrimSpace(h1.Text()), " By")
		},
		chapters: oppaiChapters,
		pages:    oppaiPages,
	},
	{
		// megatokyo.com: a webcomic whose archive lists every strip, one
		// picture each
		name: "megatokyo",
		match: func(doc *goquery.Document) bool {
			return doc.Find(`div.content ul li a[href^="./strip/"]`).Length() > 0
		},
		titleOf: func(g *Gallery) string {
			title, _, _ := strings.Cut(g.doc.Find("title").Text(), " - ")
			return title
		},
		chapters: megaTokyoChapters,
		pages:    megaTokyoPages,
	},
	{
		// toonx.to (Next.js): the chapters and each reader's pages are in
		// the flight data, the DOM only shows some of them
		name: "toonx",
		match: func(doc *goquery.Document) bool {
			return toonxChapterRe.MatchString(doc.Find("script").Text())
		},
		title:    "h1",
		chapters: toonxChapters,
		pages:    toonxPages,
	},
	{
		// xcomic.me (Qwik): a title gathers several sources' uploads, each
		// source's page lists its chapters, and the reader's pages are in
		// the Qwik state
		name: "xcomic",
		match: func(doc *goquery.Document) bool {
			return doc.Find(`script[type="qwik/state"]`).Length() > 0 && doc.Find(`a[href^="/source/"]`).Length() > 0
		},
		title:    "h1 a",
		chapters: xcomicChapters,
		pages:    xcomicPages,
	},
	{
		// manhwa18.net (Laravel, Inertia): the series and each chapter come
		// as the page's Inertia props
		name: "inertia",
		match: func(doc *goquery.Document) bool {
			var page inertiaPage
			return inertiaProps(doc, &page) == nil && page.Props.Manga.Slug != "" && len(page.Props.Chapters) > 0
		},
		titleOf: func(g *Gallery) string {
			var page inertiaPage
			_ = inertiaProps(g.doc, &page)
			return page.Props.Manga.Name
		},
		chapters: inertiaChapters,
		pages:    inertiaPages,
	},
	{
		// 8muses: albums nest; an album of albums lists them as chapters,
		// an album of pictures is one. The page data is a ROT47 JSON blob.
		name: "8muses",
		match: func(doc *goquery.Document) bool {
			return doc.Find("script#ractive-public").Length() > 0
		},
		titleOf: func(g *Gallery) string {
			album, _ := eightMusesAlbum(g.doc)
			return album.Album.Name
		},
		chapters: eightMusesChapters,
		pages:    eightMusesPages,
	},
}

func init() {
	galleryEngines = append(galleryEngines, gallerySeriesEngines...)
}

// hentaiHereChapters reads the chapter <select>: "Chapter 9 - Title (latest)"
func hentaiHereChapters(g *Gallery) ([]*GalleryChapter, error) {
	var chapters []*GalleryChapter
	options := g.doc.Find("select#preview-chapter option[data-read]")
	options.Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s.Text()), "(latest)"))
		c := galleryNumbered(g.resolve(s.AttrOr("data-read", "")), text, float64(options.Length()-i))
		if _, title, ok := strings.Cut(text, " - "); ok {
			c.Title = strings.TrimSpace(title)
			c.Label.Style = LabelPrefixed
		}
		c.ID = s.AttrOr("value", "")
		chapters = append(chapters, c)
	})
	return chapters, nil
}

// hentaiHerePages reads the reader's JSON
func hentaiHerePages(g *Gallery, c *GalleryChapter) ([]string, error) {
	doc, err := g.docOf(c.URL)
	if err != nil {
		return nil, err
	}
	var data struct {
		Images []string `json:"images"`
	}
	if err := json.Unmarshal([]byte(doc.Find("script[data-reader-data]").Text()), &data); err != nil {
		return nil, fmt.Errorf("reading the reader's page list: %w", err)
	}
	return data.Images, nil
}

// yupMangaChapters asks admin-ajax.php for the chapter list, oldest first,
// page by page
func yupMangaChapters(g *Gallery) ([]*GalleryChapter, error) {
	id := g.doc.Find("input#post_manga_id").AttrOr("value", "")
	var chapters []*GalleryChapter
	for page, pages := 1, 1; page <= pages && page <= 100; page++ {
		text, err := http.PostText(http.RequestParams{
			URL:     g.BaseUrl() + "/wp-admin/admin-ajax.php",
			Referer: g.URL,
			Headers: map[string]string{"X-Requested-With": "XMLHttpRequest"},
			Form: url.Values{
				"action":    {"baka_ajax"},
				"type":      {"load_chapters_paginated"},
				"parent_id": {id},
				"page":      {strconv.Itoa(page)},
				"order":     {"oldest_first"},
				"per_page":  {"50"},
			},
		})
		if err != nil {
			return nil, err
		}
		var resp struct {
			Data struct {
				HTML       string `json:"html"`
				TotalPages int    `json:"total_pages"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(text), &resp); err != nil {
			return nil, fmt.Errorf("reading the chapter list: %w", err)
		}
		pages = resp.Data.TotalPages
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(resp.Data.HTML))
		if err != nil {
			return nil, err
		}
		doc.Find("div.comic-card[data-chap-id] > a[href]").Each(func(_ int, s *goquery.Selection) {
			title := strings.TrimSpace(s.AttrOr("title", ""))
			c := galleryNumbered(g.resolve(s.AttrOr("href", "")), title, float64(len(chapters)+1))
			if _, ok := parseChapterNumber(title); !ok {
				// "One Shot": a title, numbered by its place in the list
				c.Title = title
			}
			chapters = append(chapters, c)
		})
	}
	return chapters, nil
}

// hentaiKunChapters reads the rows: one "Read" row for a gallery, a
// "Chapter N" row per chapter otherwise
func hentaiKunChapters(g *Gallery) ([]*GalleryChapter, error) {
	var chapters []*GalleryChapter
	rows := g.doc.Find("a.readchap")
	rows.Each(func(i int, s *goquery.Selection) {
		chapters = append(chapters, galleryNumbered(g.resolve(s.AttrOr("href", "")), s.Text(), float64(rows.Length()-i)))
	})
	return chapters, nil
}

// hentaiKunPages reads the reader's page <select> and fetches every page
// for its picture, the first from the reader itself
func hentaiKunPages(g *Gallery, c *GalleryChapter) ([]string, error) {
	reader, err := g.docOf(c.URL)
	if err != nil {
		return nil, err
	}
	var pageURLs []string
	reader.Find("select option").Each(func(_ int, s *goquery.Selection) {
		if strings.HasPrefix(strings.TrimSpace(s.Text()), "Page ") {
			pageURLs = append(pageURLs, g.resolve(s.AttrOr("value", "")))
		}
	})
	picture := func(doc *goquery.Document) string {
		src := strings.TrimSpace(doc.Find("img.image_rin").First().AttrOr("src", ""))
		return strings.ReplaceAll(src, " ", "%20")
	}

	urls := []string{picture(reader)}
	for _, pageURL := range pageURLs[min(1, len(pageURLs)):] {
		doc, err := galleryDoc(pageURL, c.URL)
		if err != nil {
			return nil, err
		}
		urls = append(urls, picture(doc))
	}
	if urls[0] == "" {
		return nil, nil
	}
	return urls, nil
}

// oppaiChapters reads the rows, numbered by their ch-num
func oppaiChapters(g *Gallery) ([]*GalleryChapter, error) {
	var chapters []*GalleryChapter
	g.doc.Find("section.chapters a[ch-num]").Each(func(_ int, s *goquery.Selection) {
		n, err := strconv.ParseFloat(s.AttrOr("ch-num", ""), 64)
		if err != nil {
			return
		}
		chapters = append(chapters, &GalleryChapter{
			Chapter: Chapter{Number: n, Label: Label{Style: LabelNumbered}},
			URL:     g.resolve(s.AttrOr("href", "")),
		})
	})
	return chapters, nil
}

var (
	// oppaiCountRe is the reader's request for its page count
	oppaiCountRe = regexp.MustCompile(`url:\s*'([^']*images\.php[^']*)'`)
	// oppaiFirstRe is the reader's first page, whose folder holds the rest
	oppaiFirstRe = regexp.MustCompile(`loadImage\("([^"]+/)1\."`)
)

// oppaiPages asks for the page count and picks the file type the first page
// has (jpg, else png, as the reader tries them)
func oppaiPages(g *Gallery, c *GalleryChapter) ([]string, error) {
	reader, err := g.docOf(c.URL)
	if err != nil {
		return nil, err
	}
	script := reader.Find("script").Text()
	countURL, folder := oppaiCountRe.FindStringSubmatch(script), oppaiFirstRe.FindStringSubmatch(script)
	if countURL == nil || folder == nil {
		return nil, errors.New("the reader's page list wasn't found")
	}
	text, err := http.GetText(http.RequestParams{URL: strings.ReplaceAll(countURL[1], " ", "%20"), Referer: c.URL})
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || count <= 0 {
		return nil, fmt.Errorf("the chapter's page count is %q", strings.TrimSpace(text))
	}

	ext := "jpg"
	body, err := http.Get(http.RequestParams{URL: folder[1] + "1.jpg", Referer: c.URL, Accept: http.ImageAccept, DisableRetry: true})
	if err != nil {
		ext = "png"
	} else {
		body.Close()
	}
	urls := make([]string, count)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s%d.%s", folder[1], i+1, ext)
	}
	return urls, nil
}

// megaTokyoStripRe is a strip's number in its link
var megaTokyoStripRe = regexp.MustCompile(`strip/(\d+)`)

// megaTokyoChapters reads the archive: "0001 - E3 Nightmare Begins"
func megaTokyoChapters(g *Gallery) ([]*GalleryChapter, error) {
	var chapters []*GalleryChapter
	g.doc.Find(`div.content ul li a[href^="./strip/"]`).Each(func(_ int, s *goquery.Selection) {
		m := megaTokyoStripRe.FindStringSubmatch(s.AttrOr("href", ""))
		if m == nil {
			return
		}
		n, _ := strconv.ParseFloat(m[1], 64)
		_, title, _ := strings.Cut(strings.TrimSpace(s.Text()), " - ")
		chapters = append(chapters, &GalleryChapter{
			Chapter: Chapter{Number: n, Title: strings.TrimSpace(title), Label: Label{Style: LabelPrefixed}},
			URL:     g.resolve(s.AttrOr("href", "")),
		})
	})
	return chapters, nil
}

// megaTokyoPages reads a strip's picture, relative to the page's <base>
func megaTokyoPages(g *Gallery, c *GalleryChapter) ([]string, error) {
	doc, err := g.docOf(c.URL)
	if err != nil {
		return nil, err
	}
	src := doc.Find("#strip img").First().AttrOr("src", "")
	if src == "" {
		return nil, nil
	}
	base, err := url.Parse(c.URL)
	if err != nil {
		return nil, err
	}
	if href, ok := doc.Find("base[href]").Attr("href"); ok {
		if b, err := base.Parse(href); err == nil {
			base = b
		}
	}
	u, err := base.Parse(strings.TrimSpace(src))
	if err != nil {
		return nil, err
	}
	return []string{u.String()}, nil
}

var (
	// toonxChapterRe is a chapter of the series in the flight data, its
	// quotes escaped: {\"id\":1,\"chapterNumber\":\"7.20\",\"title\":null,\"slug\":\"chapter-7.2\"
	toonxChapterRe = regexp.MustCompile(`\\"id\\":(\d+),\\"chapterNumber\\":\\"([\d.]+)\\",\\"title\\":(?:null|\\"((?:[^"\\]|\\\\.)*)\\"),\\"slug\\":\\"([^"\\]+)\\"`)
	// toonxImagesRe is the reader's page list in the flight data
	toonxImagesRe = regexp.MustCompile(`\\"images\\":\[(.*?)\]`)
	// toonxImageRe is one page of that list
	toonxImageRe = regexp.MustCompile(`\\"filename\\":\\"([^"\\]+)\\".*?\\"sortOrder\\":(\d+)`)
)

// toonxChapters reads the chapters of the flight data, once each
func toonxChapters(g *Gallery) ([]*GalleryChapter, error) {
	series := strings.TrimSuffix(strings.SplitN(g.URL, "?", 2)[0], "/")
	var chapters []*GalleryChapter
	seen := map[string]bool{}
	for _, m := range toonxChapterRe.FindAllStringSubmatch(g.doc.Find("script").Text(), -1) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		n, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		chapters = append(chapters, &GalleryChapter{
			Chapter: Chapter{
				Number: n,
				Title:  strings.ReplaceAll(m[3], `\\`, ``),
				ID:     m[1],
				Label:  Label{Style: LabelPrefixed},
			},
			URL: series + "/chapter/" + strings.TrimPrefix(m[4], "chapter-"),
		})
	}
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].Number > chapters[j].Number })
	return chapters, nil
}

// toonxPages reads the reader's page list, in its sort order
func toonxPages(g *Gallery, c *GalleryChapter) ([]string, error) {
	doc, err := g.docOf(c.URL)
	if err != nil {
		return nil, err
	}
	list := toonxImagesRe.FindStringSubmatch(doc.Find("script").Text())
	if list == nil {
		return nil, errors.New("the reader's page list wasn't found")
	}
	type page struct {
		url   string
		order int
	}
	var pages []page
	for _, m := range toonxImageRe.FindAllStringSubmatch(list[1], -1) {
		order, _ := strconv.Atoi(m[2])
		pages = append(pages, page{g.BaseUrl() + "/uploads/" + m[1], order})
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].order < pages[j].order })
	urls := make([]string, len(pages))
	for i, p := range pages {
		urls[i] = p.url
	}
	return urls, nil
}

// xcomicSourceRe is a source's id in its link
var xcomicSourceRe = regexp.MustCompile(`^/source/([^/?#]+)`)

// xcomicChapters reads every source's chapter list; a source is the
// chapter's group, as "source #id" (how the site names one)
func xcomicChapters(g *Gallery) ([]*GalleryChapter, error) {
	var chapters []*GalleryChapter
	seen := map[string]bool{}
	var errs []string
	g.doc.Find(`a[href^="/source/"]`).Each(func(_ int, s *goquery.Selection) {
		m := xcomicSourceRe.FindStringSubmatch(s.AttrOr("href", ""))
		if m == nil || seen[m[1]] {
			return
		}
		seen[m[1]] = true
		doc, err := galleryDoc(g.resolve(m[0]), g.URL)
		if err != nil {
			errs = append(errs, err.Error())
			return
		}
		doc.Find(`a[href^="/chapter/"]`).Each(func(_ int, row *goquery.Selection) {
			n, ok := parseChapterNumber(row.Text())
			if !ok {
				if d := galleryNumberRe.FindString(row.Text()); d != "" {
					n, _ = strconv.ParseFloat(d, 64)
				} else {
					return
				}
			}
			c := &GalleryChapter{
				Chapter: Chapter{Number: n, Label: Label{Style: LabelNumbered}},
				URL:     g.resolve(row.AttrOr("href", "")),
			}
			c.SetGroups("source #" + m[1])
			chapters = append(chapters, c)
		})
	})
	if len(chapters) == 0 && len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return chapters, nil
}

// galleryNumberRe is the first number of a text ("Ch-38")
var galleryNumberRe = regexp.MustCompile(`\d+(?:\.\d+)?`)

// xcomicURLRe is a page in the Qwik state's imageUrls list
var xcomicURLRe = regexp.MustCompile(`"(https?://[^"]+)"`)

// xcomicPages reads the imageUrls list of the reader's Qwik state
func xcomicPages(g *Gallery, c *GalleryChapter) ([]string, error) {
	doc, err := g.docOf(c.URL)
	if err != nil {
		return nil, err
	}
	state := doc.Find(`script[type="qwik/state"]`).Text()
	i := strings.Index(state, `"imageUrls"`)
	if i < 0 {
		return nil, errors.New("the reader's page list wasn't found")
	}
	list := state[i:]
	start := strings.Index(list, "[")
	end := strings.Index(list, "]")
	if start < 0 || end < start {
		return nil, errors.New("the reader's page list wasn't found")
	}
	var urls []string
	for _, m := range xcomicURLRe.FindAllStringSubmatch(list[start:end], -1) {
		urls = append(urls, m[1])
	}
	return urls, nil
}

// inertiaPage is the part of an Inertia page's props the engine reads
type inertiaPage struct {
	Props struct {
		Manga struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"manga"`
		Chapters []struct {
			ID   any    `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"chapters"`
		ChapterImages []struct {
			Src string `json:"src"`
		} `json:"chapterImages"`
	} `json:"props"`
}

// inertiaProps decodes the page's Inertia data into v
func inertiaProps(doc *goquery.Document, v any) error {
	data, ok := doc.Find("#app[data-page]").Attr("data-page")
	if !ok {
		return errors.New("no Inertia data")
	}
	return json.Unmarshal([]byte(data), v)
}

// inertiaChapters reads the series' chapter list
func inertiaChapters(g *Gallery) ([]*GalleryChapter, error) {
	var page inertiaPage
	if err := inertiaProps(g.doc, &page); err != nil {
		return nil, err
	}
	var chapters []*GalleryChapter
	for i, ch := range page.Props.Chapters {
		c := galleryNumbered(g.BaseUrl()+"/manga/"+page.Props.Manga.Slug+"/"+ch.Slug, ch.Name, float64(len(page.Props.Chapters)-i))
		if _, title, ok := strings.Cut(ch.Name, " - "); ok {
			c.Title = strings.TrimSpace(title)
			c.Label.Style = LabelPrefixed
		}
		if ch.ID != nil {
			c.ID = fmt.Sprint(ch.ID)
		}
		chapters = append(chapters, c)
	}
	return chapters, nil
}

// inertiaPages reads the chapter's images from its page's props
func inertiaPages(g *Gallery, c *GalleryChapter) ([]string, error) {
	doc, err := g.docOf(c.URL)
	if err != nil {
		return nil, err
	}
	var page inertiaPage
	if err := inertiaProps(doc, &page); err != nil {
		return nil, err
	}
	var urls []string
	for _, img := range page.Props.ChapterImages {
		if img.Src != "" {
			urls = append(urls, g.resolve(img.Src))
		}
	}
	return urls, nil
}

// eightMusesPage is the part of an 8muses album page's data the engine reads
type eightMusesPage struct {
	Album struct {
		Name      string `json:"name"`
		Permalink string `json:"permalink"`
	} `json:"album"`
	Albums []struct {
		Permalink   string `json:"permalink"`
		DisplayName string `json:"displayName"`
	} `json:"albums"`
	Pictures []struct {
		PublicURI string `json:"publicUri"`
	} `json:"pictures"`
	Pages int `json:"pages"`
}

// eightMusesAlbum decodes the page's data: HTML-escaped, ROT47, then JSON
// after a stray prefix
func eightMusesAlbum(doc *goquery.Document) (eightMusesPage, error) {
	var page eightMusesPage
	text := strings.TrimSpace(html.UnescapeString(doc.Find("script#ractive-public").Text()))
	decoded := []byte(text)
	for i, c := range decoded {
		if c >= '!' && c <= '~' {
			decoded[i] = '!' + (c-'!'+47)%94
		}
	}
	start := strings.IndexByte(string(decoded), '{')
	if start < 0 {
		return page, errors.New("the album's data wasn't found")
	}
	if err := json.Unmarshal(decoded[start:], &page); err != nil {
		return page, fmt.Errorf("reading the album's data: %w", err)
	}
	return page, nil
}

// eightMusesPaged reads the album at albumURL and its further pages (/2, /3…)
func eightMusesPaged(g *Gallery, albumURL string) ([]eightMusesPage, error) {
	doc, err := g.docOf(albumURL)
	if err != nil {
		return nil, err
	}
	first, err := eightMusesAlbum(doc)
	if err != nil {
		return nil, err
	}
	pages := []eightMusesPage{first}
	for p := 2; p <= first.Pages && p <= 50; p++ {
		doc, err := galleryDoc(g.BaseUrl()+"/porn/album/"+first.Album.Permalink+"/"+strconv.Itoa(p), g.URL)
		if err != nil {
			return nil, err
		}
		page, err := eightMusesAlbum(doc)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, nil
}

// eightMusesChapters lists an album's albums, or the album as one chapter
// when it holds pictures
func eightMusesChapters(g *Gallery) ([]*GalleryChapter, error) {
	pages, err := eightMusesPaged(g, g.URL)
	if err != nil {
		return nil, err
	}
	var chapters []*GalleryChapter
	for _, page := range pages {
		for _, a := range page.Albums {
			chapters = append(chapters, &GalleryChapter{
				Chapter: Chapter{Title: a.DisplayName, Number: float64(len(chapters) + 1), Label: Label{Style: LabelNumbered}},
				URL:     g.BaseUrl() + "/porn/album/" + a.Permalink,
			})
		}
	}
	if len(chapters) == 0 {
		chapters = append(chapters, &GalleryChapter{
			Chapter: Chapter{Number: 1, Label: Label{Style: LabelNumbered}},
			URL:     g.URL,
		})
	}
	return chapters, nil
}

// eightMusesPages lists an album's pictures at full size
func eightMusesPages(g *Gallery, c *GalleryChapter) ([]string, error) {
	pages, err := eightMusesPaged(g, c.URL)
	if err != nil {
		return nil, err
	}
	var urls []string
	for _, page := range pages {
		for _, p := range page.Pictures {
			urls = append(urls, g.BaseUrl()+"/image/fl/"+p.PublicURI+".jpg")
		}
	}
	if len(urls) == 0 && len(pages[0].Albums) > 0 {
		return nil, fmt.Errorf("%s is an album of albums: open one of them", c.URL)
	}
	return urls, nil
}
