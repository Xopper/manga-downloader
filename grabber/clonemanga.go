// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// CloneManga is a grabber for Clone Manga (manga.clone-army.org): a series
// is one chapter read in viewer.php, one picture per viewer page, and the
// page <select> says how many there are
type CloneManga struct {
	*Grabber
	doc *goquery.Document
}

func NewCloneManga(g *Grabber) *CloneManga {
	return &CloneManga{Grabber: g}
}

// Test returns true for a clone-army.org viewer URL that names a series
func (c *CloneManga) Test() (bool, error) {
	return hostMatchesDomain(c.URL, "clone-army.org") && c.series() != "", nil
}

// series returns the URL's series
func (c CloneManga) series() string {
	u, err := url.Parse(c.URL)
	if err != nil {
		return ""
	}
	return u.Query().Get("series")
}

// viewerURL returns the URL of the series' nth page
func (c CloneManga) viewerURL(page int) string {
	return fmt.Sprintf("%s/viewer.php?series=%s&page=%d&lang=english", c.BaseUrl(), url.QueryEscape(c.series()), page)
}

// fetchViewer fetches one viewer page
func (c CloneManga) fetchViewer(page int) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: c.viewerURL(page), Referer: c.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

// firstViewer fetches the series' first page once
func (c *CloneManga) firstViewer() (*goquery.Document, error) {
	if c.doc == nil {
		doc, err := c.fetchViewer(1)
		if err != nil {
			return nil, err
		}
		c.doc = doc
	}
	return c.doc, nil
}

// FetchTitle returns the series name its <select> shows
func (c *CloneManga) FetchTitle() (string, error) {
	doc, err := c.firstViewer()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find("select.series_select option[selected]").First().Text()), nil
}

// FetchChapters returns the series as its one chapter
func (c *CloneManga) FetchChapters() (Filterables, []error) {
	title, err := c.FetchTitle()
	if err != nil {
		return nil, []error{err}
	}
	return Filterables{&Chapter{Number: 1, Title: title, ID: c.series(), Label: Label{Style: LabelUnnumbered}}}, nil
}

// viewerImage returns the picture of a viewer page
func viewerImage(doc *goquery.Document, base string) string {
	src := strings.TrimSpace(doc.Find(".subsectionContainer img[src]").First().AttrOr("src", ""))
	if src == "" {
		return ""
	}
	b, _ := url.Parse(base)
	ref, err := url.Parse(src)
	if err != nil {
		return ""
	}
	return b.ResolveReference(ref).String()
}

// FetchChapter reads every viewer page for its picture
func (c *CloneManga) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := c.firstViewer()
	if err != nil {
		return nil, err
	}
	count := 1
	doc.Find("select.page_select option").Each(func(_ int, o *goquery.Selection) {
		if n, err := strconv.Atoi(o.AttrOr("value", "")); err == nil && n > count {
			count = n
		}
	})

	chapter := &Chapter{Title: f.GetTitle(), Label: labelOf(f), Number: f.GetNumber(), ID: c.series()}
	for n := 1; n <= count; n++ {
		page := doc
		if n > 1 {
			if page, err = c.fetchViewer(n); err != nil {
				return nil, err
			}
		}
		if img := viewerImage(page, c.BaseUrl()+"/"); img != "" {
			chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: img})
		}
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}
