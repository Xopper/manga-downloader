// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// madara crawls the Madara WordPress theme family: the /manga/page/N/
// archive, one grid page at a time, until a page is missing (404) or empty.
// Rows are .post-title a (inside .page-item-detail); each row's
// .list-chapter holds the newest chapters, the first of which is the latest.
type madara struct{}

func init() {
	crawlers["madara"] = madara{}
}

func (madara) Crawl(ctx context.Context, site SiteInfo, limit int) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list := site.List
	if list == "" {
		list = strings.TrimSuffix(site.Home, "/") + "/manga/page/{page}/"
	}

	return walkPages(ctx, list, limit, madaraPage)
}

// madaraPage reads one archive page
func madaraPage(doc *goquery.Document, base string) []Entry {
	var entries []Entry
	doc.Find(".post-title a").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		entry := Entry{Title: strings.TrimSpace(a.Text()), URL: absURL(base, href)}
		newest := a.Closest(".page-item-detail").Find(".list-chapter .chapter a").First()
		newestHref, _ := newest.Attr("href")
		entry.Latest = chapterNumber(newest.Text(), newestHref)
		entries = append(entries, entry)
	})

	return entries
}

// chapterWord is the number after the chapter word in a chapter's label:
// "Chapter 1120", "Ch. 26.5", "Capítulo 12", "Bölüm 7", "Chap 3"
var chapterWord = regexp.MustCompile(`(?i)(?:chapter|chap|ch|cap[ií]tulo|cap|bölüm|глава)\.?\s*(\d+(?:\.\d+)?)`)

// anyNumber is the first number of a text
var anyNumber = regexp.MustCompile(`\d+(?:\.\d+)?`)

// chapterHref is the number of Madara's chapter slug: /chapter-26/, /chapter-26-5/
var chapterHref = regexp.MustCompile(`/chapter-(\d+)(?:-(\d+))?/?$`)

// chapterNumber reads a chapter's number from its label ("Chapter 1120"),
// else from its link's slug (chapter-1120-5 is 1120.5); 0 when neither says
func chapterNumber(label, href string) float64 {
	label = strings.TrimSpace(label)
	if m := chapterWord.FindStringSubmatch(label); m != nil {
		if n, err := strconv.ParseFloat(m[1], 64); err == nil {
			return n
		}
	}
	if m := anyNumber.FindString(label); m != "" {
		if n, err := strconv.ParseFloat(m, 64); err == nil {
			return n
		}
	}
	if m := chapterHref.FindStringSubmatch(strings.TrimSpace(href)); m != nil {
		text := m[1]
		if m[2] != "" {
			text += "." + m[2]
		}
		if n, err := strconv.ParseFloat(text, 64); err == nil {
			return n
		}
	}

	return 0
}
