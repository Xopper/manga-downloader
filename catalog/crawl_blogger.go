// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// blogger is for the manga blogs on Blogger, where a series is no page but
// the blog's search for its name (/search?q=One+Piece), which the Blogger
// grabber loads: the crawl walks the blog's post feed (List, a
// /feeds/posts/summary URL, /-/<label> keeping one label's posts), 150 posts
// a page, and each chapter post's title ("Blue Lock Manga 364 [Español]")
// names its series. The title is read as grabber/blogger.go reads it, so the
// search finds the same posts. limit caps the feed pages.
type blogger struct{}

func init() {
	crawlers["blogger"] = blogger{}
}

const bloggerPerPage = 150

func (blogger) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	feed := site.List
	if feed == "" {
		feed = strings.TrimSuffix(site.Home, "/") + "/feeds/posts/summary"
	}
	home := homeOf(feed)
	if home == "" {
		return nil, fmt.Errorf("%s: bad feed %q", site.Site, feed)
	}
	sep := "?"
	if strings.Contains(feed, "?") {
		sep = "&"
	}
	var entries []Entry
	seen := map[string]bool{}
	for page := 1; limit <= 0 || page <= limit; page++ {
		if page > 1 {
			if err := pause(ctx); err != nil {
				return entries, err
			}
		}
		start := 1 + (page-1)*bloggerPerPage
		u := feed + sep + "alt=json&max-results=" + strconv.Itoa(bloggerPerPage) + "&start-index=" + strconv.Itoa(start)
		body, err := fetchBody(ctx, bodyRequest{URL: u})
		if err != nil {
			return entries, err
		}
		titles, err := bloggerFeedTitles(body)
		if err != nil {
			return entries, fmt.Errorf("%s: %w", u, err)
		}
		if len(titles) == 0 {
			break
		}
		for _, title := range titles {
			series, ok := bloggerSeries(title)
			key := bloggerKey(series)
			if !ok || key == "" || seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, Entry{
				Title: series,
				URL:   strings.TrimSuffix(home, "/") + "/search?q=" + url.QueryEscape(series),
			})
		}
		if len(titles) < bloggerPerPage {
			break
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s: no chapter posts in %s", site.Site, feed)
	}

	return entries, nil
}

// bloggerFeedTitles reads the post titles of a Blogger JSON feed page
func bloggerFeedTitles(body string) ([]string, error) {
	var feed struct {
		Feed struct {
			Entries []struct {
				Title struct {
					Value string `json:"$t"`
				} `json:"title"`
			} `json:"entry"`
		} `json:"feed"`
	}
	if err := json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	titles := make([]string, 0, len(feed.Feed.Entries))
	for _, e := range feed.Feed.Entries {
		titles = append(titles, e.Title.Value)
	}

	return titles, nil
}

// These follow grabber/blogger.go (parseBloggerTitle, stripKindWord,
// bloggerKey): a catalog name must be what the grabber's search matches.
var (
	bloggerBracketRe = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)`)
	bloggerPartRe    = regexp.MustCompile(`(?i)\b(?:parte|part)\s+(\d+)\b`)
	bloggerNumberRe  = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
	bloggerLeerRe    = regexp.MustCompile(`(?i)^\s*leer\s+`)
	bloggerKindRe    = regexp.MustCompile(`(?i)(?:\s*[|:,]?\s*\b(?:manga|webcomic|manhwa|cap[ií]tulo|chapter)\b)*\s*[|:,]?\s*$`)
	bloggerAccents   = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")
)

// bloggerSeries is the series a chapter post's title names: what comes
// before its last number ("Kenichi 2 | Manga 27" is "Kenichi 2"), without
// brackets, "Parte N" or the kind word; false for a title with no number
func bloggerSeries(title string) (string, bool) {
	title = bloggerLeerRe.ReplaceAllString(title, "")
	title = bloggerBracketRe.ReplaceAllString(title, " ")
	title = bloggerPartRe.ReplaceAllString(title, " ")
	locs := bloggerNumberRe.FindAllStringIndex(title, -1)
	if len(locs) == 0 {
		return "", false
	}
	series := strings.TrimSpace(bloggerKindRe.ReplaceAllString(strings.TrimSpace(title[:locs[len(locs)-1][0]]), ""))

	return squash(series), series != ""
}

// bloggerKey folds a series name for comparison: lowercase, Spanish accents
// dropped, punctuation and spacing reduced to single spaces
func bloggerKey(name string) string {
	name = bloggerAccents.Replace(strings.ToLower(name))

	return strings.Join(strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}
