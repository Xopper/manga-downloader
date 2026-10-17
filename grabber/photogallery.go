// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// photoGalleryMaxPages bounds how many pages of one album are followed
const photoGalleryMaxPages = 30

// photoGallerySite says how one photo-album site lays out an album
type photoGallerySite struct {
	// Host matches the site's URLs, Album the path of an album page
	Host, Album *regexp.Regexp
	// Title is the album title's element (empty takes <title>), and TitleTrim
	// what is cut out of it
	Title     string
	TitleTrim *regexp.Regexp
	// Image is each picture's element and Attr where its URL is (the first
	// non-empty of the comma separated attributes)
	Image, Attr string
	// Next is the link to the album's next page, for albums split over pages
	Next string
	// Normalize rewrites a picture URL (an image proxy back to the original)
	Normalize func(string) string
}

var (
	photoGalleryWPProxyRe = regexp.MustCompile(`^https://i\d+\.wp\.com/`)

	// photoGallerySites are the photo-album sites the grabber knows
	photoGallerySites = []photoGallerySite{
		{
			// xasiat.com: every picture of an album is on its page, as links
			// to /get_image/ behind lazy-loaded placeholders
			Host:  regexp.MustCompile(`(?i)^https?://(?:www\.)?xasiat\.com(?:[/?#]|$)`),
			Album: regexp.MustCompile(`^/albums/\d+/`),
			Title: "h1",
			Image: "a.item[href*='/get_image/']",
			Attr:  "href",
		},
		{
			// baobua.net: albums may be split over pages linked by "Next", and
			// some pictures go through the i0.wp.com proxy, asked for at 640px
			Host:      regexp.MustCompile(`(?i)^https?://(?:www\.)?baobua\.net(?:[/?#]|$)`),
			Album:     regexp.MustCompile(`^/spot/[^/]+\.html$`),
			TitleTrim: regexp.MustCompile(`^BaoBua\.Net:\s*|\s*\|\s*Page \d+/\d+$`),
			Image:     "div.contentme a[href^='/img.html'] img",
			Attr:      "src",
			Next:      "a.page-numbers:contains('Next')",
			Normalize: func(u string) string {
				if photoGalleryWPProxyRe.MatchString(u) {
					return strings.Replace(photoGalleryWPProxyRe.ReplaceAllString(u, "https://"), "?w=640", "", 1)
				}
				return u
			},
		},
	}
)

// PhotoGallery is a grabber for photo-album sites, where an album is a page
// of pictures: the album is a series with one chapter. The sites differ only
// in where the title and pictures are, which photoGallerySites says.
type PhotoGallery struct {
	*Grabber
	site *photoGallerySite
	doc  *goquery.Document
}

func NewPhotoGallery(g *Grabber) *PhotoGallery {
	return &PhotoGallery{Grabber: g}
}

// PhotoGalleryChapter is the album, the only chapter of its series
type PhotoGalleryChapter struct {
	Chapter
}

// Test returns true for an album page of a known site
func (p *PhotoGallery) Test() (bool, error) {
	u, err := url.Parse(p.URL)
	if err != nil {
		return false, nil
	}
	for i := range photoGallerySites {
		s := &photoGallerySites[i]
		if s.Host.MatchString(p.URL) && s.Album.MatchString(u.Path) {
			p.site = s
			return true, nil
		}
	}

	return false, nil
}

func (p *PhotoGallery) fetch(rawURL string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: rawURL})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

func (p *PhotoGallery) fetchDoc() (*goquery.Document, error) {
	if p.doc != nil {
		return p.doc, nil
	}
	if p.site == nil {
		if ok, _ := p.Test(); !ok {
			return nil, errors.New("not an album page")
		}
	}
	doc, err := p.fetch(p.URL)
	if err != nil {
		return nil, err
	}
	p.doc = doc

	return doc, nil
}

// FetchTitle returns the album title
func (p *PhotoGallery) FetchTitle() (string, error) {
	doc, err := p.fetchDoc()
	if err != nil {
		return "", err
	}

	return photoGalleryTitle(doc, p.site), nil
}

func photoGalleryTitle(doc *goquery.Document, s *photoGallerySite) string {
	var title string
	if s.Title != "" {
		title = doc.Find(s.Title).First().Text()
	} else {
		title = doc.Find("title").First().Text()
	}
	title = sanitizeTitle(title)
	if s.TitleTrim != nil {
		title = s.TitleTrim.ReplaceAllString(title, "")
	}

	return title
}

// FetchChapters returns the album as the only chapter
func (p *PhotoGallery) FetchChapters() (Filterables, []error) {
	if _, err := p.fetchDoc(); err != nil {
		return nil, []error{err}
	}
	u, _ := url.Parse(p.URL)

	return Filterables{&PhotoGalleryChapter{Chapter{Number: 1, ID: u.Path, Label: Label{Style: LabelUnnumbered}}}}, nil
}

// FetchChapter lists the album's pictures, following its pages
func (p *PhotoGallery) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := p.fetchDoc()
	if err != nil {
		return nil, err
	}

	var urls []string
	seen := map[string]bool{}
	pageURL := p.URL
	for n := 0; doc != nil && n < photoGalleryMaxPages; n++ {
		for _, u := range photoGalleryImages(doc, p.site, pageURL) {
			if !seen[u] {
				seen[u] = true
				urls = append(urls, u)
			}
		}
		next := photoGalleryNext(doc, p.site, pageURL)
		if next == "" || seen["page:"+next] {
			break
		}
		seen["page:"+next] = true
		if doc, err = p.fetch(next); err != nil {
			return nil, err
		}
		pageURL = next
	}
	if len(urls) == 0 {
		return nil, errors.New("the album shows no pictures")
	}

	chapter := &Chapter{Title: f.GetTitle(), Number: f.GetNumber(), ID: f.(*PhotoGalleryChapter).ID, Label: labelOf(f)}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// photoGalleryImages returns a page's pictures, resolved against pageURL
func photoGalleryImages(doc *goquery.Document, s *photoGallerySite, pageURL string) (urls []string) {
	base, _ := url.Parse(pageURL)
	doc.Find(s.Image).Each(func(_ int, sel *goquery.Selection) {
		for _, attr := range strings.Split(s.Attr, ",") {
			v := strings.TrimSpace(sel.AttrOr(strings.TrimSpace(attr), ""))
			if v == "" || strings.HasPrefix(v, "data:") {
				continue
			}
			u, err := base.Parse(v)
			if err != nil {
				break
			}
			abs := u.String()
			if s.Normalize != nil {
				abs = s.Normalize(abs)
			}
			urls = append(urls, abs)
			break
		}
	})

	return urls
}

// photoGalleryNext returns the album's next page, "" on its last
func photoGalleryNext(doc *goquery.Document, s *photoGallerySite, pageURL string) string {
	if s.Next == "" {
		return ""
	}
	href := doc.Find(s.Next).First().AttrOr("href", "")
	if href == "" {
		return ""
	}
	base, _ := url.Parse(pageURL)
	u, err := base.Parse(href)
	if err != nil || u.String() == pageURL {
		return ""
	}

	return u.String()
}
