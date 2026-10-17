// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// JolleyComics is a grabber for jolleycomics.com (The Property of Hate and
// its sister comics, one per first path segment): every page's jump box
// lists the chapters in bold and the current chapter's pages, the latest
// chapter only by its pages. A chapter's pages are each read from their own
// page (/{comic}/{chapter}/{n}).
type JolleyComics struct {
	*Grabber
}

// JolleyComicsChapter is a chapter, its URL showing its first page
type JolleyComicsChapter struct {
	Chapter
	URL string
}

// jolleyTitles names the comics the site doesn't name in its pages
var jolleyTitles = map[string]string{"TPoH": "The Property of Hate"}

func NewJolleyComics(g *Grabber) *JolleyComics {
	return &JolleyComics{Grabber: g}
}

// Test returns true for a jolleycomics.com URL naming a comic
func (j *JolleyComics) Test() (bool, error) {
	return hostMatchesDomain(j.URL, "jolleycomics.com") && j.comic() != "", nil
}

// comic returns the URL's comic, its first path segment; the home page is
// The Property of Hate's
func (j JolleyComics) comic() string {
	u, err := url.Parse(j.URL)
	if err != nil {
		return ""
	}
	comic, _, _ := strings.Cut(strings.Trim(u.Path, "/"), "/")
	if comic == "" {
		return "TPoH"
	}
	return comic
}

// fetchDoc fetches a page of the site
func (j JolleyComics) fetchDoc(uri string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: j.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

// FetchTitle returns the comic's name
func (j *JolleyComics) FetchTitle() (string, error) {
	if title, ok := jolleyTitles[j.comic()]; ok {
		return title, nil
	}
	return sanitizeTitle(strings.ReplaceAll(j.comic(), "_", " ")), nil
}

// FetchChapters reads the chapters off the latest page's jump box: the bold
// options, then the chapter the latest pages belong to when it isn't one
func (j *JolleyComics) FetchChapters() (Filterables, []error) {
	doc, err := j.fetchDoc(j.BaseUrl() + "/" + j.comic() + "/")
	if err != nil {
		return nil, []error{err}
	}
	prefix := "/" + j.comic() + "/"
	chapters := Filterables{}
	seen := map[string]bool{}
	add := func(path, title string) {
		if seen[path] {
			return
		}
		seen[path] = true
		chapters = append(chapters, &JolleyComicsChapter{
			Chapter: Chapter{Number: float64(len(chapters) + 1), Title: title, ID: path, Label: Label{Style: LabelNumbered}},
			URL:     j.BaseUrl() + path,
		})
	}
	doc.Find("select.jumpbox option").Each(func(_ int, o *goquery.Selection) {
		value := o.AttrOr("value", "")
		if !strings.HasPrefix(value, prefix) {
			return
		}
		if strings.Contains(o.AttrOr("style", ""), "bold") {
			add(value, strings.TrimSpace(o.Text()))
			return
		}
		// "Script : Page 652" at /TPoH/Script/652 is a page of "Script"
		name, _, _ := strings.Cut(o.Text(), " : ")
		chapter, _, _ := strings.Cut(strings.TrimPrefix(value, prefix), "/")
		add(prefix+chapter+"/", strings.TrimSpace(name))
	})
	return chapters, nil
}

// FetchChapter reads the chapter's pages off its jump box, then each page's
// picture
func (j *JolleyComics) FetchChapter(f Filterable) (*Chapter, error) {
	jchap := f.(*JolleyComicsChapter)
	first, err := j.fetchDoc(jchap.URL)
	if err != nil {
		return nil, err
	}
	var pages []string
	first.Find("select.jumpbox option").Each(func(_ int, o *goquery.Selection) {
		value := o.AttrOr("value", "")
		if strings.HasPrefix(value, jchap.ID) && value != jchap.ID && !strings.Contains(o.AttrOr("style", ""), "bold") {
			pages = append(pages, value)
		}
	})

	chapter := &Chapter{Title: f.GetTitle(), Label: labelOf(f), Number: f.GetNumber(), Language: "en", ID: jchap.ID}
	base, _ := url.Parse(j.BaseUrl() + "/")
	for i, path := range pages {
		doc := first
		if i > 0 {
			if doc, err = j.fetchDoc(j.BaseUrl() + path); err != nil {
				return nil, err
			}
		}
		src := strings.TrimSpace(doc.Find(".comic_comic img").First().AttrOr("src", ""))
		ref, err := url.Parse(src)
		if src == "" || err != nil {
			continue
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: base.ResolveReference(ref).String()})
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}
