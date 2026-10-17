// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

// saymanhwa.com: every chapter row is an <a data-chapter-item> inside an
// element with id="chapters", which the meowing.org entry's "#chapters a"
// would claim with a reader selector that finds nothing here, so this entry
// goes first. The reader lists every page as a plain <img> in
// [data-reader-pages].
func init() {
	plainHTMLSelectors = append([]SiteSelector{{
		Title:        "h1",
		Rows:         "a[data-chapter-item][data-chapter-number]",
		Chapter:      ".series-chapter-number-text",
		ChapterTitle: ".series-chapter-number-text",
		Image:        "[data-reader-pages] img",
		SeriesLink:   `.reader-nav-actions a[href*="/series/"]:not([href*="/chapter-"])`,
	}}, plainHTMLSelectors...)
}
