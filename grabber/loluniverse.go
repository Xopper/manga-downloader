// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

const (
	lolUniverseDomain = "universe.leagueoflegends.com"
	lolUniverseOrigin = "https://universe.leagueoflegends.com"
	// lolUniverseMeeps serves the comics hub and each series' issue list
	lolUniverseMeeps = "https://universe-meeps.leagueoflegends.com/v1"
	// lolUniverseComics serves each issue's (or one-shot's) page list
	lolUniverseComics = "https://universe-comics.leagueoflegends.com/comics"
)

// lolUniverseComicRe splits a comic URL into its locale, its first slug (a
// series, or a champion for a one-shot) and an optional second slug (an
// issue of the series, or the one-shot itself)
var lolUniverseComicRe = regexp.MustCompile(`(?i)^/([a-z]{2}_[a-z]{2})/comic/([^/]+)(?:/([^/]+))?/?$`)

// LolUniverse is a grabber for League of Legends' comics on
// universe.leagueoflegends.com. The site is a JS app whose comic URLs answer
// 404 to a plain GET, so everything comes from the JSON its app reads:
// a series' issues from universe-meeps, an issue's pages from
// universe-comics. Series (/comic/zed, issues /comic/zed/issue-1) and
// one-shots (/comic/nami/intotheabyss) share one URL shape, so a two-slug
// URL is a series' issue only when the first slug has an issue list that
// names it; otherwise it's a one-shot, downloaded as a single chapter.
type LolUniverse struct {
	*Grabber
	locale   string
	title    string
	chapters Filterables
	resolved bool
}

// LolUniverseChapter is an issue (or a one-shot): its comics id, "zed/issue-1"
type LolUniverseChapter struct {
	Chapter
	ComicID string
}

// lolUniverseIssues is a series' meeps index.json
type lolUniverseIssues struct {
	Name   string `json:"name"`
	Issues []struct {
		Title string   `json:"title"`
		Index *float64 `json:"index"`
		URL   string   `json:"url"`
	} `json:"issues"`
}

// lolUniversePages is an issue's comics index.json: pages come in spreads
// (one or two images each), read in order
type lolUniversePages struct {
	Title string `json:"title"`
	Pages [][]struct {
		URI   string `json:"uri"`
		URI2x string `json:"2x"`
	} `json:"desktop-pages"`
}

// NewLolUniverse returns a new LolUniverse grabber
func NewLolUniverse(g *Grabber) *LolUniverse {
	return &LolUniverse{Grabber: g}
}

// Test matches the site's domain
func (l *LolUniverse) Test() (bool, error) {
	return hostMatchesDomain(l.URL, lolUniverseDomain), nil
}

// lolUniverseParseURL returns the locale and slugs of a comic URL
func lolUniverseParseURL(rawURL string) (locale, first, second string, ok bool) {
	_, path, ok := chapterPath(rawURL)
	if !ok {
		return "", "", "", false
	}
	m := lolUniverseComicRe.FindStringSubmatch(path)
	if m == nil {
		return "", "", "", false
	}
	return strings.ToLower(m[1]), m[2], m[3], true
}

// getJSON fetches one of the app's JSON files with the headers its CDN wants
func (l *LolUniverse) getJSON(uri string, v any) error {
	body, err := http.Get(http.RequestParams{
		URL:          uri,
		Referer:      lolUniverseOrigin + "/",
		Origin:       lolUniverseOrigin,
		Accept:       "application/json",
		DisableRetry: true,
	})
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(v)
}

// resolve works out whether the URL is a series, an issue (then its series)
// or a one-shot, and lists the chapters
func (l *LolUniverse) resolve() error {
	if l.resolved {
		return nil
	}
	locale, first, second, ok := lolUniverseParseURL(l.URL)
	if !ok {
		return errSeriesNotFound(l.URL)
	}
	l.locale = locale
	lang, _, _ := strings.Cut(locale, "_")

	issues := &lolUniverseIssues{}
	err := l.getJSON(fmt.Sprintf("%s/%s/comics/%s/index.json", lolUniverseMeeps, locale, first), issues)
	isSeries := err == nil && len(issues.Issues) > 0
	if second != "" && isSeries {
		isSeries = lolUniverseHasIssue(issues, first+"/"+second)
	}
	if isSeries {
		l.title = sanitizeTitle(issues.Name)
		l.chapters = lolUniverseChapters(issues, lang)
		l.resolved = true
		return nil
	}
	if second == "" {
		if err == nil {
			err = fmt.Errorf("no issues found for %s", l.URL)
		}
		return err
	}

	pages := &lolUniversePages{}
	id := first + "/" + second
	if err := l.getJSON(fmt.Sprintf("%s/%s/%s/index.json", lolUniverseComics, locale, id), pages); err != nil {
		return err
	}
	l.title = sanitizeTitle(pages.Title)
	l.chapters = Filterables{&LolUniverseChapter{
		Chapter{Number: 1, Language: lang, ID: id, Label: Label{Style: LabelNumbered}},
		id,
	}}
	l.resolved = true
	return nil
}

// lolUniverseHasIssue reports whether a series' issue list names the comic id
func lolUniverseHasIssue(issues *lolUniverseIssues, id string) bool {
	for _, is := range issues.Issues {
		if strings.EqualFold(lolUniverseComicID(is.URL), id) {
			return true
		}
	}
	return false
}

// lolUniverseComicID is the part of a comic URL after /comic/
func lolUniverseComicID(u string) string {
	_, id, ok := strings.Cut(u, "/comic/")
	if !ok {
		return ""
	}
	return strings.Trim(id, "/")
}

// lolUniverseChapters turns a series' issues into chapters, numbered by the
// issue's index (by position when it has none)
func lolUniverseChapters(issues *lolUniverseIssues, lang string) Filterables {
	var chapters Filterables
	for i, is := range issues.Issues {
		id := lolUniverseComicID(is.URL)
		if id == "" {
			continue
		}
		num := float64(i + 1)
		if is.Index != nil {
			num = *is.Index
		}
		chapters = append(chapters, &LolUniverseChapter{
			Chapter{
				Number:   num,
				Title:    sanitizeTitle(is.Title),
				Language: lang,
				ID:       id,
				Label:    Label{Style: LabelNumbered},
			},
			id,
		})
	}
	return chapters
}

// FetchTitle returns the series name or the one-shot's title
func (l *LolUniverse) FetchTitle() (string, error) {
	if err := l.resolve(); err != nil {
		return "", err
	}
	return l.title, nil
}

// FetchChapters returns the series' issues, or the one-shot alone
func (l *LolUniverse) FetchChapters() (Filterables, []error) {
	if err := l.resolve(); err != nil {
		return nil, []error{err}
	}
	return l.chapters, nil
}

// FetchChapter reads an issue's spreads, preferring each image's 2x version
func (l *LolUniverse) FetchChapter(f Filterable) (*Chapter, error) {
	lchap := f.(*LolUniverseChapter)
	if err := l.resolve(); err != nil {
		return nil, err
	}
	data := &lolUniversePages{}
	if err := l.getJSON(fmt.Sprintf("%s/%s/%s/index.json", lolUniverseComics, l.locale, lchap.ComicID), data); err != nil {
		return nil, err
	}
	pages := lolUniversePageList(data)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   lchap.Language,
		ID:         lchap.ID,
		Pages:      pages,
	}, nil
}

// lolUniversePageList flattens the spreads into pages
func lolUniversePageList(data *lolUniversePages) []Page {
	var pages []Page
	for _, spread := range data.Pages {
		for _, img := range spread {
			u := img.URI2x
			if u == "" {
				u = img.URI
			}
			if u == "" {
				continue
			}
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: u})
		}
	}
	return pages
}
