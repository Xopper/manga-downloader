// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Hennojin is a grabber for hennojin.com, a doujinshi site: each work
// (/home/manga/{slug}/) is one chapter, read at
// /home/manga-reader/?manga={name}&view=page, which renders every page.
// The pictures sit in a folder named after the work, spaces and brackets
// included, so their URLs are escaped before they're fetched.
type Hennojin struct {
	*Grabber
	title  string
	reader string
}

// HennojinChapter is a hennojin work
type HennojinChapter struct {
	Chapter
}

func NewHennojin(g *Grabber) *Hennojin {
	return &Hennojin{Grabber: g}
}

// Test matches hennojin.com work and reader URLs
func (h *Hennojin) Test() (bool, error) {
	if !hostMatchesDomain(h.URL, "hennojin.com") {
		return false, nil
	}
	u, err := url.Parse(h.URL)
	if err != nil {
		return false, nil
	}
	return strings.HasPrefix(u.Path, "/home/manga/") || strings.HasPrefix(u.Path, "/home/manga-reader/"), nil
}

// load finds the work's title and reader, from the work page or the reader URL
func (h *Hennojin) load() error {
	if h.reader != "" {
		return nil
	}
	u, err := url.Parse(h.URL)
	if err != nil {
		return err
	}
	if strings.HasPrefix(u.Path, "/home/manga-reader/") {
		h.title = u.Query().Get("manga")
		h.reader = hennojinReaderURL(u)
		return nil
	}

	body, err := http.Get(http.RequestParams{URL: h.URL})
	if err != nil {
		return err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}
	href, ok := doc.Find(`a[href*="/manga-reader/"]`).First().Attr("href")
	if !ok {
		return errors.New("could not find the work's reader")
	}
	ref, err := u.Parse(href)
	if err != nil {
		return err
	}
	h.title = ref.Query().Get("manga")
	if h.title == "" {
		h.title = doc.Find("h3").First().Text()
	}
	h.reader = hennojinReaderURL(ref)
	return nil
}

// hennojinReaderURL re-encodes a reader link, whose query the site writes
// unescaped, and asks for the full page view
func hennojinReaderURL(u *url.URL) string {
	q := u.Query()
	q.Set("view", "page")
	r := *u
	r.RawQuery = q.Encode()
	r.Fragment = ""
	return r.String()
}

// FetchTitle returns the work's title
func (h *Hennojin) FetchTitle() (string, error) {
	if err := h.load(); err != nil {
		return "", err
	}
	return sanitizeTitle(h.title), nil
}

// FetchChapters returns the work as its only chapter
func (h *Hennojin) FetchChapters() (Filterables, []error) {
	if err := h.load(); err != nil {
		return nil, []error{err}
	}
	return Filterables{&HennojinChapter{Chapter{
		Number: 1,
		ID:     chapterURLID(h.reader),
		Label:  Label{Style: LabelNumbered},
	}}}, nil
}

// FetchChapter reads the pages off the reader
func (h *Hennojin) FetchChapter(f Filterable) (*Chapter, error) {
	if err := h.load(); err != nil {
		return nil, err
	}
	body, err := http.Get(http.RequestParams{URL: h.reader})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(h.reader)

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		ID:       chapterURLID(h.reader),
		Language: "en",
	}
	doc.Find("#readmanga img, .mySlides img").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		u, err := base.Parse(src)
		if err != nil {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    u.String(),
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))
	if chapter.PagesCount == 0 {
		return nil, errors.New("no pages found in the reader")
	}
	return chapter, nil
}
