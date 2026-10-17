// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

var (
	// oglafHostRe matches oglaf.com URLs
	oglafHostRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?oglaf\.com(?:[/?#]|$)`)
	// oglafStoryRe matches a story's link on the archive, `/slug/`
	oglafStoryRe = regexp.MustCompile(`^/([^/?#]+)/$`)
	// oglafPageRe matches a story's later page, `/slug/2/`
	oglafPageRe = regexp.MustCompile(`^/([^/?#]+)/(\d+)/$`)
)

const (
	// oglafArchiveURL is the whole webcomic: every story, newest first
	oglafArchiveURL = "https://www.oglaf.com/archive/"
	// oglafMaxPages bounds the walk over a story's pages
	oglafMaxPages = 200
)

// Oglaf is a grabber for oglaf.com, a single adult webcomic: the series is the
// archive and each story a chapter. The archive lists stories newest first, as
// banner links (400px wide images), and gives no page count, so a story's
// pages are found by following each page's `rel="next"` link while it stays
// within the story (`/slug/2/`, `/slug/3/`…); the last page links to the next
// story's `/slug/` instead. Any oglaf.com link loads the whole archive.
type Oglaf struct {
	*Grabber
}

// NewOglaf returns an oglaf grabber
func NewOglaf(g *Grabber) *Oglaf {
	return &Oglaf{Grabber: g}
}

// OglafChapter is one story, keeping its path for FetchChapter
type OglafChapter struct {
	Chapter
	Path string
}

// Test matches oglaf.com URLs by domain
func (o *Oglaf) Test() (bool, error) {
	return oglafHostRe.MatchString(o.URL), nil
}

// FetchTitle returns the comic's name: the site has only the one
func (o *Oglaf) FetchTitle() (string, error) {
	return "Oglaf", nil
}

// FetchChapters lists the archive's stories, numbered oldest first
func (o *Oglaf) FetchChapters() (Filterables, []error) {
	body, err := http.Get(http.RequestParams{URL: oglafArchiveURL})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}
	paths := oglafStories(doc)
	if len(paths) == 0 {
		return nil, []error{fmt.Errorf("no stories found in %s", oglafArchiveURL)}
	}
	chapters := make(Filterables, 0, len(paths))
	for i, p := range paths {
		slug := oglafStoryRe.FindStringSubmatch(p)[1]
		chapters = append(chapters, &OglafChapter{
			Chapter: Chapter{
				Number: float64(len(paths) - i),
				Title:  slug,
				ID:     slug,
				Label:  Label{Style: LabelPrefixed},
			},
			Path: p,
		})
	}
	return chapters, nil
}

// oglafStories returns the archive's story paths in its order, once each
func oglafStories(doc *goquery.Document) []string {
	var paths []string
	seen := map[string]bool{}
	doc.Find(`a:has(img[width="400"])`).Each(func(_ int, a *goquery.Selection) {
		href := a.AttrOr("href", "")
		if u, err := url.Parse(href); err == nil {
			href = u.Path
		}
		if !oglafStoryRe.MatchString(href) || href == "/archive/" || seen[href] {
			return
		}
		seen[href] = true
		paths = append(paths, href)
	})
	return paths
}

// FetchChapter walks the story's pages, one strip each
func (o *Oglaf) FetchChapter(f Filterable) (*Chapter, error) {
	oc := f.(*OglafChapter)
	slug := oglafStoryRe.FindStringSubmatch(oc.Path)[1]
	base, _ := url.Parse(oglafArchiveURL)

	chapter := &Chapter{
		Title:  oc.Title,
		Number: oc.Number,
		Label:  labelOf(f),
		ID:     oc.ID,
	}
	next := oc.Path
	for range oglafMaxPages {
		ref, err := url.Parse(next)
		if err != nil {
			return nil, err
		}
		body, err := http.Get(http.RequestParams{URL: base.ResolveReference(ref).String()})
		if err != nil {
			return nil, err
		}
		doc, err := goquery.NewDocumentFromReader(body)
		body.Close()
		if err != nil {
			return nil, err
		}
		src := strings.TrimSpace(doc.Find("img#strip").AttrOr("src", ""))
		if src == "" {
			break
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: src})
		next = oglafNextInStory(doc, slug)
		if next == "" {
			break
		}
	}
	if len(chapter.Pages) == 0 {
		return nil, fmt.Errorf("no strips found for %s", oc.Path)
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}

// oglafNextInStory returns the page's next link when it's a later page of the
// same story, "" when the story ends there
func oglafNextInStory(doc *goquery.Document, slug string) string {
	href := doc.Find(`a[rel="next"]`).First().AttrOr("href", "")
	if u, err := url.Parse(href); err == nil {
		href = u.Path
	}
	m := oglafPageRe.FindStringSubmatch(href)
	if m == nil || m[1] != slug {
		return ""
	}
	return href
}
