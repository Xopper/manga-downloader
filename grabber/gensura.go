// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Gensura is a grabber for gensura.net, a doujinshi gallery site: a gallery
// is one chapter, and its page shows every picture as a thumbnail whose name
// ends in "-t" (the full picture is the same name without it)
type Gensura struct {
	*Grabber
	doc *goquery.Document
}

var (
	// gensuraGalleryRe captures a gallery's path from a gallery or reader URL
	gensuraGalleryRe = regexp.MustCompile(`^(/manga/[^/]+)`)
	// gensuraThumbRe finds a thumbnail's "-t" suffix
	gensuraThumbRe = regexp.MustCompile(`-t(\.[a-z0-9]+)$`)
)

func NewGensura(g *Grabber) *Gensura {
	return &Gensura{Grabber: g}
}

// Test returns true for a gensura.net gallery URL
func (g *Gensura) Test() (bool, error) {
	return hostMatchesDomain(g.URL, "gensura.net") && g.galleryPath() != "", nil
}

// galleryPath returns the gallery's path, "" when the URL isn't one
func (g Gensura) galleryPath() string {
	u, err := url.Parse(g.URL)
	if err != nil {
		return ""
	}
	matches := gensuraGalleryRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return ""
	}
	return matches[1] + "/"
}

// gallery fetches the gallery's page once
func (g *Gensura) gallery() (*goquery.Document, error) {
	if g.doc == nil {
		body, err := http.Get(http.RequestParams{URL: g.BaseUrl() + g.galleryPath()})
		if err != nil {
			return nil, err
		}
		defer body.Close()
		if g.doc, err = goquery.NewDocumentFromReader(body); err != nil {
			return nil, err
		}
	}
	return g.doc, nil
}

// FetchTitle returns the gallery's title ("English | Japanese")
func (g *Gensura) FetchTitle() (string, error) {
	doc, err := g.gallery()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find("h1.font-semibold").First().Text()), nil
}

// FetchChapters returns the gallery as its one chapter
func (g *Gensura) FetchChapters() (Filterables, []error) {
	title, err := g.FetchTitle()
	if err != nil {
		return nil, []error{err}
	}
	return Filterables{&Chapter{Number: 1, Title: title, ID: g.galleryPath(), Label: Label{Style: LabelUnnumbered}}}, nil
}

// FetchChapter returns the full pictures of the gallery's thumbnails
func (g *Gensura) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := g.gallery()
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(g.BaseUrl())
	chapter := &Chapter{Title: f.GetTitle(), Label: labelOf(f), Number: f.GetNumber(), ID: g.galleryPath()}
	doc.Find("div.img-thumb a img, div.lazy-img-thumb a img").Each(func(_ int, img *goquery.Selection) {
		src := strings.TrimSpace(img.AttrOr("data-src", ""))
		if src == "" {
			src = strings.TrimSpace(img.AttrOr("src", ""))
		}
		ref, err := url.Parse(gensuraThumbRe.ReplaceAllString(src, "$1"))
		if src == "" || err != nil {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: base.ResolveReference(ref).String()})
	})
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}
