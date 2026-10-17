// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"regexp"

	"github.com/elboletaire/manga-downloader/http"
)

// NHentaiCom is a grabber for nhentai.com (not nhentai.net), whose comics are
// one gallery each, listed as a single chapter: its JSON API answers the
// title and every page's picture for a comic's slug.
type NHentaiCom struct {
	*Grabber
	slug  string
	comic *nhentaiComImages
}

// NHentaiComChapter is the comic, as the only chapter
type NHentaiComChapter struct {
	Chapter
}

type nhentaiComImages struct {
	Comic struct {
		Title string `json:"title"`
		Pages int    `json:"pages"`
	} `json:"comic"`
	Images []struct {
		Page      int    `json:"page"`
		SourceURL string `json:"source_url"`
	} `json:"images"`
}

// the comic and its reader: /{lang}/comic/{slug} and …/reader
var nhentaiComURLRe = regexp.MustCompile(`^https?://(?:www\.)?nhentai\.com/(?:[a-z]{2}/)?comic/([^/?#]+)`)

func NewNHentaiCom(g *Grabber) *NHentaiCom {
	return &NHentaiCom{Grabber: g}
}

// Test returns true for a comic or reader URL of nhentai.com
func (n *NHentaiCom) Test() (bool, error) {
	m := nhentaiComURLRe.FindStringSubmatch(n.URL)
	if m == nil {
		return false, nil
	}
	n.slug = m[1]
	return true, nil
}

func (n *NHentaiCom) images() (*nhentaiComImages, error) {
	if n.comic != nil {
		return n.comic, nil
	}
	body, err := http.Get(http.RequestParams{URL: n.BaseUrl() + "/api/comics/" + n.slug + "/images", Referer: n.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var comic nhentaiComImages
	if err := json.NewDecoder(body).Decode(&comic); err != nil {
		return nil, err
	}
	n.comic = &comic
	return n.comic, nil
}

// FetchTitle returns the comic's title
func (n *NHentaiCom) FetchTitle() (string, error) {
	comic, err := n.images()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(comic.Comic.Title), nil
}

// FetchChapters returns the comic as its only chapter
func (n *NHentaiCom) FetchChapters() (Filterables, []error) {
	if _, err := n.images(); err != nil {
		return nil, []error{err}
	}
	return Filterables{&NHentaiComChapter{Chapter{Number: 1, Label: Label{Style: LabelNumbered}, ID: n.slug}}}, nil
}

// FetchChapter lists the comic's pages
func (n *NHentaiCom) FetchChapter(f Filterable) (*Chapter, error) {
	comic, err := n.images()
	if err != nil {
		return nil, err
	}
	if len(comic.Images) == 0 {
		return nil, errors.New("the comic has no pages")
	}
	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Label:      labelOf(f),
		ID:         n.slug,
		PagesCount: int64(len(comic.Images)),
	}
	for _, img := range comic.Images {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(img.Page), URL: img.SourceURL})
	}
	return chapter, nil
}
