// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// webnexImage is the reader page selector of webnex.cc, a Next.js reader of
// MangaDex and scanlator uploads. Its series page lists one row per chapter
// (the group's upload it shows first; the others sit in a menu with no link
// for the shown one, so they aren't listed). The reader renders only its
// first pages; all of them are in the flight data's "pages" list.
const webnexImage = "img.page-image"

func init() {
	// first: the list sits in section#chapters, which the meowing.org
	// entry's "#chapters a" would claim
	plainHTMLSelectors = append([]SiteSelector{{
		Title:        "h1",
		Rows:         "li.ch-row:has(a.ch-link)",
		Link:         "a.ch-link",
		Chapter:      ".ch-num",
		ChapterTitle: ".ch-title:not(.is-plain)",
		UploadGroup:  ".ch-source, .ch-source-trigger .ch-source-name",
		Image:        webnexImage,
	}}, plainHTMLSelectors...)
}

// webnexPages returns the page list of a webnex.cc reader's flight data, nil
// for any other reader or when the list isn't found once
func webnexPages(selector string, doc *goquery.Document) []string {
	if selector != webnexImage {
		return nil
	}
	html, _ := doc.Html()
	stream, err := nextFlightStream(html)
	if err != nil || strings.Count(stream, `"pages":[`) != 1 {
		return nil
	}
	raw, err := extractBalancedJSON(stream, `"pages":`)
	if err != nil {
		return nil
	}
	var pages []string
	if json.Unmarshal([]byte(raw), &pages) != nil {
		return nil
	}
	return pages
}
