// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Themes of the single-series fan sites (one manga per domain, often with a
// few spin-offs), all plain HTML with every page an <img> in the reader.
func init() {
	plainHTMLSelectors = append(plainHTMLSelectors,
		// the Tailwind/Vite theme of ww9.readfairytail.com, ww6.readchainsawman.com
		// and ww6.readkingdom.com: one card per chapter, holding the title link
		// and a "Read" button to the same chapter
		SiteSelector{
			Title:        "h1.my-3",
			Rows:         `div.bg-bg-secondary:has(a.mb-1[href*="/chapter/"])`,
			Link:         `a.mb-1[href*="/chapter/"]`,
			Chapter:      `a.mb-1[href*="/chapter/"]`,
			ChapterTitle: `a.mb-1[href*="/chapter/"]`,
			Image:        "img.js-page",
		},
		// the Bootstrap theme of readberserk.com: a table row per chapter, the
		// name in the first cell and a "Read" button
		SiteSelector{
			Title:        "h2.mb-0 span",
			Rows:         `.card-table tbody tr:has(a[href*="/chapter/"])`,
			Link:         `a[href*="/chapter/"]`,
			Chapter:      "td:first-child",
			ChapterTitle: "td:first-child",
			Image:        "img.pages__img",
		},
	)
}

// isLazyPlaceholder reports whether src is a lazy-load plugin's shared
// placeholder (a3 Lazy Load's lazy_placeholder.gif) with the real image in
// data-src
func isLazyPlaceholder(src string, s *goquery.Selection) bool {
	base := strings.ToLower(path.Base(src))
	return s.AttrOr("data-src", "") != "" && strings.Contains(base, "placeholder")
}
