// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Junmeitu is a grabber for the Junmeitu photo galleries (meijuntu.com,
// junmeitu.com): a gallery is one chapter whose pictures are one per page,
// /{category}/{slug}.html then /{category}/{slug}-{n}.html
type Junmeitu struct {
	*Grabber
	doc *goquery.Document
}

// junmeituPathRe splits a gallery page path into the gallery path and the
// page number, if any
var junmeituPathRe = regexp.MustCompile(`^(/[a-z]+/[^/]+?)(?:-(\d+))?\.html$`)

func NewJunmeitu(g *Grabber) *Junmeitu {
	return &Junmeitu{Grabber: g}
}

// Test returns true for a gallery page of a known domain
func (j *Junmeitu) Test() (bool, error) {
	if !hostMatchesDomain(j.URL, "meijuntu.com") && !hostMatchesDomain(j.URL, "junmeitu.com") {
		return false, nil
	}
	return j.galleryPath() != "", nil
}

// galleryPath returns the gallery's path without a page number, "" when the
// URL isn't a gallery page
func (j Junmeitu) galleryPath() string {
	u, err := url.Parse(j.URL)
	if err != nil {
		return ""
	}
	matches := junmeituPathRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return ""
	}
	return matches[1]
}

// pageURL returns the URL of the gallery's nth page
func (j Junmeitu) pageURL(n int) string {
	if n == 1 {
		return j.BaseUrl() + j.galleryPath() + ".html"
	}
	return fmt.Sprintf("%s%s-%d.html", j.BaseUrl(), j.galleryPath(), n)
}

// fetchPage fetches one page of the gallery
func (j Junmeitu) fetchPage(n int) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: j.pageURL(n), Referer: j.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

// firstPage fetches the gallery's first page once
func (j *Junmeitu) firstPage() (*goquery.Document, error) {
	if j.doc != nil {
		return j.doc, nil
	}
	doc, err := j.fetchPage(1)
	if err != nil {
		return nil, err
	}
	j.doc = doc
	return doc, nil
}

// FetchTitle returns the gallery's title
func (j *Junmeitu) FetchTitle() (string, error) {
	doc, err := j.firstPage()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(doc.Find("h1.title").First().Text()), nil
}

// FetchChapters returns the gallery as its one chapter
func (j *Junmeitu) FetchChapters() (Filterables, []error) {
	title, err := j.FetchTitle()
	if err != nil {
		return nil, []error{err}
	}
	return Filterables{&Chapter{Number: 1, Title: title, ID: j.galleryPath(), Label: Label{Style: LabelUnnumbered}}}, nil
}

// FetchChapter fetches every page of the gallery for its picture; the page
// count is the highest page the pager links to
func (j *Junmeitu) FetchChapter(f Filterable) (*Chapter, error) {
	doc, err := j.firstPage()
	if err != nil {
		return nil, err
	}
	count := 1
	doc.Find("div.pages a[href]").Each(func(_ int, a *goquery.Selection) {
		u, err := url.Parse(a.AttrOr("href", ""))
		if err != nil {
			return
		}
		if m := junmeituPathRe.FindStringSubmatch(u.Path); m != nil && m[2] != "" {
			if n, _ := strconv.Atoi(m[2]); n > count {
				count = n
			}
		}
	})

	chapter := &Chapter{
		Title:  f.GetTitle(),
		Label:  labelOf(f),
		Number: f.GetNumber(),
		ID:     j.galleryPath(),
	}
	for n := 1; n <= count; n++ {
		page := doc
		if n > 1 {
			if page, err = j.fetchPage(n); err != nil {
				return nil, err
			}
		}
		page.Find(".pictures img[src]").Each(func(_ int, img *goquery.Selection) {
			chapter.Pages = append(chapter.Pages, Page{
				Number: int64(len(chapter.Pages) + 1),
				URL:    img.AttrOr("src", ""),
			})
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
