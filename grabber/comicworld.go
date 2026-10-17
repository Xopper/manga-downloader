// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// ComicWorld is a grabber for sites on the ComicWorld WordPress theme and its
// coins plugin (rinkocomics.com). A series page shows its newest 10 chapters
// and a "load more" button, which asks admin-ajax.php for the next 10
// (load_more_chapters, with the comic's ID, an offset and the page's nonce).
// Paid chapters are rows marked locked-chapter, which a logged-out reader
// can't open. It matches by markup.
type ComicWorld struct {
	*Grabber
	doc *goquery.Document
}

// ComicWorldChapter is a chapter of a ComicWorld series
type ComicWorldChapter struct {
	Chapter
	URL string
}

// NewComicWorld returns a new ComicWorld grabber
func NewComicWorld(g *Grabber) *ComicWorld {
	return &ComicWorld{Grabber: g}
}

// cwMoreButton is the series page's "load more" button, which holds the comic's ID
const cwMoreButton = "#loadMoreChaptersBtn[data-comic-id]"

// cwRows are a chapter list's rows
const cwRows = "li.chapter[data-permalink]"

// Test fetches the page and tells whether it's a ComicWorld series page
func (c *ComicWorld) Test() (bool, error) {
	doc, err := wcGetDoc(c.URL, "", true)
	if err != nil {
		return false, err
	}
	if doc.Find(cwMoreButton+", ul.chapters-list "+cwRows).Length() == 0 ||
		!strings.Contains(doc.Text(), "comicworld_ajax") {
		return false, nil
	}
	c.doc = doc
	return true, nil
}

// FetchTitle returns the series' title
func (c *ComicWorld) FetchTitle() (string, error) {
	if c.doc == nil {
		if _, err := c.Test(); err != nil {
			return "", err
		}
	}
	title := sanitizeTitle(c.doc.Find("h1").First().Text())
	if title == "" {
		title = smPageName(c.doc.Find("title").First().Text())
	}
	return title, nil
}

// cwNonceRe reads the nonce the page gives its scripts
var cwNonceRe = regexp.MustCompile(`comicworld_ajax\s*=\s*\{[^}]*"nonce"\s*:\s*"([^"]+)"`)

// cwMaxPages bounds the "load more" requests of a listing
const cwMaxPages = 200

// FetchChapters returns the series' chapters: the page's rows, then those
// "load more" gives until it gives none
func (c *ComicWorld) FetchChapters() (Filterables, []error) {
	if c.doc == nil {
		if _, err := c.Test(); err != nil {
			return nil, []error{err}
		}
	}
	chapters, seen := Filterables{}, map[string]bool{}
	chapters = cwChapters(c.doc.Find("ul.chapters-list "+cwRows), seen, chapters)

	button := c.doc.Find(cwMoreButton).First()
	html, _ := c.doc.Html()
	nonce := cwNonceRe.FindStringSubmatch(html)
	if button.Length() == 0 || nonce == nil {
		return chapters.SortByNumber(), nil
	}
	offset, _ := strconv.Atoi(button.AttrOr("data-offset", ""))
	if offset == 0 {
		offset = len(chapters)
	}
	for range cwMaxPages {
		body, err := http.PostText(http.RequestParams{
			URL:     c.BaseUrl() + "/wp-admin/admin-ajax.php",
			Referer: c.URL,
			Form: url.Values{
				"action":   {"load_more_chapters"},
				"nonce":    {nonce[1]},
				"comic_id": {button.AttrOr("data-comic-id", "")},
				"offset":   {strconv.Itoa(offset)},
			},
		})
		if err != nil {
			return chapters, []error{err}
		}
		var res struct {
			Success bool `json:"success"`
			Data    struct {
				HTML string `json:"html"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &res); err != nil || !res.Success || strings.TrimSpace(res.Data.HTML) == "" {
			break
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.Data.HTML))
		if err != nil {
			break
		}
		rows := doc.Find(cwRows)
		if rows.Length() == 0 {
			break
		}
		chapters = cwChapters(rows, seen, chapters)
		offset += rows.Length()
	}
	// oldest first, as a reader goes; the page lists the newest first
	return chapters.SortByNumber(), nil
}

// cwChapters adds the rows' chapters to chapters, numbered by the number
// their title ends with ("… Chapter 30"); a row seen before is skipped
func cwChapters(rows *goquery.Selection, seen map[string]bool, chapters Filterables) Filterables {
	rows.Each(func(_ int, row *goquery.Selection) {
		link := strings.TrimSpace(row.AttrOr("data-permalink", ""))
		if link == "" || seen[link] {
			return
		}
		seen[link] = true
		title := sanitizeTitle(row.AttrOr("data-title", ""))
		num, ok := parseChapterNumber(title)
		if !ok {
			m := wpTitleNumberRe.FindStringSubmatch(title)
			if m == nil {
				return
			}
			num, _ = strconv.ParseFloat(m[1], 64)
		}
		chapters = append(chapters, &ComicWorldChapter{
			Chapter: Chapter{
				Number: num,
				ID:     chapterURLID(link),
				Locked: row.HasClass("locked-chapter"),
				Label:  Label{Style: LabelNumbered},
			},
			URL: link,
		})
	})
	return chapters
}

// FetchChapter fetches a chapter's pages
func (c *ComicWorld) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*ComicWorldChapter)
	doc, err := wcGetDoc(ch.URL, c.URL, false)
	if err != nil {
		return nil, err
	}
	var pages []Page
	for _, src := range wcComicImages(doc, "img.chapter-image") {
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      ch.Title,
		Number:     ch.Number,
		Label:      ch.Label,
		ID:         ch.ID,
		PagesCount: int64(len(pages)),
		Language:   "en",
		Pages:      pages,
	}, nil
}
