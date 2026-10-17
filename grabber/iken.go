// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Iken is a grabber for the Iken comic CMS (kencomics.com, orion-scans.com,
// nyxscans.com): series at /series/{slug}, readers at
// /series/{slug}/{chapter-slug}. Its sites run one of two front ends (Astro
// islands or TanStack Start), and both print the CMS API in the page's config
// ("NEXT_PUBLIC_API_URL":"https://api.{host}"). The series page renders only
// the newest chapters (20 on kencomics) while the API lists them all, with the
// site's own lock flags. Readers are server rendered, every page an
// img[data-reader-page-image].
type Iken struct {
	*Grabber
	api      string // the CMS API base, https://api.{host}
	slug     string
	post     ikenPost
	fetched  bool
	chapters Filterables
}

// IkenChapter is a chapter of an Iken site
type IkenChapter struct {
	Chapter
	Slug string
}

type ikenPost struct {
	ID    int    `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"postTitle"`
}

type ikenChapter struct {
	Number            float64 `json:"number"`
	Slug              string  `json:"slug"`
	Title             string  `json:"title"`
	IsLocked          bool    `json:"isLocked"`
	IsShortLinkLocked bool    `json:"isShortLinkLocked"`
}

func NewIken(g *Grabber) *Iken {
	return &Iken{Grabber: g}
}

var (
	ikenSeriesRe = regexp.MustCompile(`^/series/([^/?#]+)`)
	// the config is a JSON object, escaped once more inside a script string
	// on some front ends
	ikenAPIRe = regexp.MustCompile(`NEXT_PUBLIC_API_URL\\?":\\?"(https?://[^"\\]+)`)
)

// Test takes series and reader URLs (/series/...): it fetches the series page
// and accepts it when it names the CMS API and that API knows the series
func (k *Iken) Test() (bool, error) {
	u, err := url.Parse(k.URL)
	if err != nil {
		return false, nil
	}
	m := ikenSeriesRe.FindStringSubmatch(u.EscapedPath())
	if m == nil {
		return false, nil
	}
	slug, err := url.PathUnescape(m[1])
	if err != nil {
		return false, nil
	}

	// the series page, also for a reader URL: some readers (nyxscans) don't
	// print the config
	body, err := http.GetText(http.RequestParams{
		URL:          k.BaseUrl() + "/series/" + m[1],
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		return false, err
	}
	api := ikenAPIRe.FindStringSubmatch(body)
	if api == nil {
		return false, nil
	}
	k.api = strings.TrimRight(api[1], "/")

	post, err := http.GetText(http.RequestParams{
		URL:          k.api + "/api/post?postSlug=" + url.QueryEscape(slug),
		Referer:      k.BaseUrl() + "/",
		DisableRetry: true,
	})
	if err != nil {
		return false, nil
	}
	var res struct {
		Post ikenPost `json:"post"`
	}
	if json.Unmarshal([]byte(post), &res) != nil || res.Post.ID == 0 {
		return false, nil
	}
	k.post, k.slug = res.Post, slug
	return true, nil
}

// FetchTitle returns the series title the API gives
func (k *Iken) FetchTitle() (string, error) {
	return sanitizeTitle(k.post.Title), nil
}

// ikenPageSize is how many chapters one API request asks for
const ikenPageSize = 500

// ikenChapterPage is one page of the API's chapter list
type ikenChapterPage struct {
	Post struct {
		Chapters []ikenChapter `json:"chapters"`
	} `json:"post"`
	TotalChapterCount int `json:"totalChapterCount"`
}

// FetchChapters lists every chapter through the API, oldest first
func (k *Iken) FetchChapters() (Filterables, []error) {
	if k.fetched {
		return k.chapters, nil
	}
	var chapters Filterables
	for skip := 0; ; skip += ikenPageSize {
		body, err := http.GetText(http.RequestParams{
			URL: fmt.Sprintf("%s/api/chapters?postId=%d&skip=%d&take=%d&order=asc",
				k.api, k.post.ID, skip, ikenPageSize),
			Referer: k.BaseUrl() + "/",
		})
		if err != nil {
			return chapters, []error{err}
		}
		var page ikenChapterPage
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			return chapters, []error{fmt.Errorf("could not read the chapter list: %w", err)}
		}
		for _, c := range page.Post.Chapters {
			chapters = append(chapters, &IkenChapter{
				Chapter{
					Number: c.Number,
					Title:  strings.TrimSpace(c.Title),
					ID:     c.Slug,
					Label:  Label{Style: LabelNumbered},
					// a short-link lock may still open to a reader, as in
					// tanstackComic, so only the other locks count
					Locked: c.IsLocked && !c.IsShortLinkLocked,
				},
				c.Slug,
			})
		}
		if len(page.Post.Chapters) < ikenPageSize || len(chapters) >= page.TotalChapterCount {
			break
		}
	}
	if len(chapters) == 0 {
		return nil, []error{errors.New("the series has no chapters")}
	}
	k.chapters, k.fetched = chapters, true
	return chapters, nil
}

// FetchChapter fetches a reader page and returns its pages
func (k *Iken) FetchChapter(f Filterable) (*Chapter, error) {
	kchap := f.(*IkenChapter)
	uri, err := url.JoinPath(k.BaseUrl(), "series", k.slug, kchap.Slug)
	if err != nil {
		return nil, err
	}
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: k.BaseUrl() + "/",
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "en",
		ID:       kchap.Slug,
		Locked:   kchap.Locked,
	}
	doc.Find("img[data-reader-page-image]").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))
	if chapter.PagesCount == 0 {
		return nil, fmt.Errorf("no pages found for %s (it might be locked behind coins or early access)", ChapterLabel(f))
	}
	return chapter, nil
}
