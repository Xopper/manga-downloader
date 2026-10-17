// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

// adultSelectors are the PlainHTML entries of adult manhwa sites. Their rows
// are scoped to markup no other entry uses, so they go after the rest of
// plainHTMLSelectors and can't shadow it.
var adultSelectors = []SiteSelector{
	// manhwa18.cc, mangadass.com, yaoiscan.me: one PHP manhwa script (not
	// Madara) listing every chapter on the series page. The <h1> holds an
	// "18+" badge. The reader is followed by related series whose covers
	// are lazy too, so images are taken inside .read-content only.
	{
		Title:      "h1",
		TitleNoise: "span",
		Rows:       "ul.row-content-chapter li a.chapter-name",
		Image:      "div.read-content img[data-src]",
	},
	// manhwa18tx.com: the same script's other theme, with the date in a
	// second span of each row
	{
		Title:   "h1",
		Rows:    ".manga-chapters .list-items li.item > a[href]",
		Chapter: "span:first-child",
		Image:   "div.reading-content img[data-src]",
	},
	// manhwa18.com ("MyManga", Laravel): every chapter on the series page,
	// many named "chap 61", which the text parser doesn't read, so the
	// number comes from the URL (/chapter-75-net-90565, /chap-01-1143)
	{
		Title:    ".series-name a",
		Rows:     "ul.list-chapters.at-series a[href]",
		NumberRe: `/chap(?:ter)?-(\d+)-`,
		Image:    "#chapter-content img[data-src]",
	},
	// hentara.com: a client-rendered app with a server-rendered copy for
	// crawlers. The <h1> is an SEO sentence, the breadcrumb's last item is
	// the title. The reader also shows related covers from the same CDN,
	// which aren't under a chapter folder.
	{
		Title: "nav.ssr-breadcrumb > span:last-child",
		Rows:  "nav.ssr-chapters-grid a.ssr-chapter",
		Image: `img[src*="/chapter-"]`,
	},
	// pornhwa18.com (Qwik, server-rendered): the full list is a scrolling
	// box of links, apart from the first/last chapter buttons above it.
	// Every reader starts with a relative /banner.jpg placeholder.
	{
		Title:   `h1[class*="drop-shadow-solid"]`,
		Rows:    `div.max-h-96 > a[href*="/chapter-"]`,
		Chapter: "p",
		Image:   `img.imgku[data-src^="http"]`,
	},
	// kuramanga.com (Astro): every chapter on the series page
	{
		Title: "h1.manga-title",
		Rows:  "#chapterList .chapter-item a",
		Image: "section.chapter-images img",
	},
	// todaymanga.com: every chapter on the series page as "Ch.374" chips.
	// Its image host redirects to the homepage without the site's Referer.
	{
		Title:   "h1.series-name",
		Rows:    "ul#chapList a.chap-chip",
		Chapter: "strong",
		Image:   ".chapter-content img[data-src]",
	},
}

func init() {
	plainHTMLSelectors = append(plainHTMLSelectors, adultSelectors...)
}
