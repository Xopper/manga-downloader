// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

var (
	yabaiHostRe    = regexp.MustCompile(`(?i)^https?://(?:www\.)?yabai\.si(?:[/?#]|$)`)
	yabaiGalleryRe = regexp.MustCompile(`^/g/([A-Za-z0-9]+)`)
)

// Yabai is a grabber for yabai.si, a doujinshi gallery site behind DDoS-Guard
// (see ddosGuardPass) built with Laravel and Inertia: every page carries its
// props as JSON in the #app element's data-page attribute. A gallery is a
// series with one chapter, and its reader page (/g/{slug}/read) holds both
// the gallery and the picture list, given as parallel arrays (page number,
// hash, random part, extension) to compose into file names.
type Yabai struct {
	*Grabber
	reader *yabaiReader
}

func NewYabai(g *Grabber) *Yabai {
	return &Yabai{Grabber: g}
}

// YabaiChapter is the gallery, the only chapter of its series
type YabaiChapter struct {
	Chapter
}

// yabaiReader is the reader page's Inertia props
type yabaiReader struct {
	Post struct {
		Data struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"data"`
	} `json:"post"`
	Pages struct {
		Data struct {
			List yabaiPageList `json:"list"`
		} `json:"data"`
	} `json:"pages"`
}

type yabaiPageList struct {
	Root string   `json:"root"`
	Code int      `json:"code"`
	Head []string `json:"head"`
	Hash []string `json:"hash"`
	Rand []string `json:"rand"`
	Type []string `json:"type"`
}

// Test returns true for yabai.si URLs
func (y *Yabai) Test() (bool, error) {
	return yabaiHostRe.MatchString(y.URL), nil
}

func yabaiSlug(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	m := yabaiGalleryRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errors.New("not a yabai.si gallery URL (/g/{slug})")
	}

	return m[1], nil
}

func (y *Yabai) fetchReader() (*yabaiReader, error) {
	if y.reader != nil {
		return y.reader, nil
	}
	slug, err := yabaiSlug(y.URL)
	if err != nil {
		return nil, err
	}
	ddosGuardPass(y.BaseUrl())
	body, err := http.Get(http.RequestParams{URL: y.BaseUrl() + "/g/" + slug + "/read"})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	raw, ok := doc.Find("[data-page]").First().Attr("data-page")
	if !ok {
		return nil, fmt.Errorf("could not find the gallery data of %s", slug)
	}
	page := struct {
		Props yabaiReader `json:"props"`
	}{}
	if err = json.Unmarshal([]byte(raw), &page); err != nil {
		return nil, err
	}
	y.reader = &page.Props

	return y.reader, nil
}

// FetchTitle returns the gallery title
func (y *Yabai) FetchTitle() (string, error) {
	reader, err := y.fetchReader()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(reader.Post.Data.Name), nil
}

// FetchChapters returns the gallery as the only chapter
func (y *Yabai) FetchChapters() (Filterables, []error) {
	reader, err := y.fetchReader()
	if err != nil {
		return nil, []error{err}
	}

	return Filterables{&YabaiChapter{Chapter{Number: 1, ID: reader.Post.Data.Slug, Language: akumaLanguage(reader.Post.Data.Name), Label: Label{Style: LabelUnnumbered}}}}, nil
}

// FetchChapter composes the gallery's picture URLs
func (y *Yabai) FetchChapter(f Filterable) (*Chapter, error) {
	reader, err := y.fetchReader()
	if err != nil {
		return nil, err
	}
	urls := yabaiPages(reader.Pages.Data.List)
	if len(urls) == 0 {
		return nil, errors.New("the gallery lists no pictures")
	}

	chapter := &Chapter{Title: f.GetTitle(), Number: f.GetNumber(), ID: f.(*YabaiChapter).ID, Language: f.(*YabaiChapter).Language, Label: labelOf(f)}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// yabaiPages composes {root}/{code}/{page:0000}-{hash}-{rand}.{type} for each
// page, in page order
func yabaiPages(l yabaiPageList) []string {
	n := len(l.Head)
	if len(l.Hash) < n || len(l.Rand) < n || len(l.Type) < n || l.Root == "" {
		return nil
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	number := func(i int) int { v, _ := strconv.Atoi(l.Head[i]); return v }
	sort.SliceStable(order, func(a, b int) bool { return number(order[a]) < number(order[b]) })

	urls := make([]string, 0, n)
	for _, i := range order {
		urls = append(urls, fmt.Sprintf("%s/%d/%04d-%s-%s.%s", strings.TrimRight(l.Root, "/"), l.Code, number(i), l.Hash[i], l.Rand[i], l.Type[i]))
	}

	return urls
}
