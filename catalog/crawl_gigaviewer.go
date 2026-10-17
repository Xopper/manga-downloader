// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// gigaviewer is for the hosts of Hatena's GigaViewer (shonenjumpplus.com,
// comic-days.com, …): one page, the site's list (/series or /comics), links
// every series to one of its episodes, and an episode link loads the whole
// series in the downloader. The markup differs between hosts, so a link's
// title is the first title element inside it, else its picture's alt text,
// else its own text. A site's Links narrows the episode links to its series
// cards.
type gigaviewer struct{}

func init() {
	crawlers["gigaviewer"] = gigaviewer{}
}

var gigaviewerEpisodePath = regexp.MustCompile(`^/episode/\d+$`)

func (gigaviewer) Crawl(ctx context.Context, site SiteInfo, _ int) ([]Entry, error) {
	list := site.List
	if list == "" {
		list = strings.TrimSuffix(site.Home, "/") + "/series"
	}
	base, err := url.Parse(list)
	if err != nil {
		return nil, err
	}
	doc, err := fetch(ctx, list)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	seenURL, seenTitle := map[string]bool{}, map[string]bool{}
	links := `a[href*="/episode/"]`
	if site.Links != "" {
		// a home page whose banners and news also link episodes
		links = site.Links
	}
	doc.Find(links).Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		u, err := base.Parse(href)
		if err != nil || u.Host != base.Host || !gigaviewerEpisodePath.MatchString(u.Path) {
			return
		}
		u.RawQuery, u.Fragment = "", ""
		title := gigaviewerTitle(a)
		// the same series can be linked twice (a banner, then the list)
		if title == "" || seenURL[u.String()] || seenTitle[title] {
			return
		}
		seenURL[u.String()], seenTitle[title] = true, true
		entries = append(entries, Entry{Title: title, URL: u.String()})
	})
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s: no series links", list)
	}

	return entries, nil
}

// gigaviewerButton matches a link's own text when it's a button rather than
// a title: "read the latest", "read from the start", "free", "episode 1"
var gigaviewerButton = regexp.MustCompile(`最新話|最初から|読む|無料|^第?[0-9０-９]+話`)

// gigaviewerVolume is a paperback's volume or edition, which a /comics list
// adds to the series' title ("…心中 ３巻", "…の女の子 ４【電子単行本版】")
var gigaviewerVolume = regexp.MustCompile(`\s*(?:[0-9０-９]+巻|[0-9０-９]*\s*【[^】]*】)$`)

// gigaviewerTitle is a series link's title: the series title the link carries
// (data-series-title or data-series-name), a series title element (a card
// can also name its latest episode), any title or heading, the picture's alt
// text, then the link's own text when it's short and not a button's
func gigaviewerTitle(a *goquery.Selection) string {
	if t := squash(a.AttrOr("data-series-title", a.AttrOr("data-series-name", ""))); t != "" {
		return gigaviewerVolume.ReplaceAllString(t, "")
	}
	for _, sel := range []string{`[class*="series"][class*="title"]`, `[class*="title"]`, "h2, h3, h4"} {
		if t := squash(a.Find(sel).First().Text()); t != "" {
			return gigaviewerVolume.ReplaceAllString(t, "")
		}
	}
	if alt := squash(a.Find("img").First().AttrOr("alt", "")); alt != "" {
		return gigaviewerVolume.ReplaceAllString(alt, "")
	}
	if t := squash(a.Text()); len([]rune(t)) <= 60 && !gigaviewerButton.MatchString(t) {
		return t
	}

	return ""
}

// squash trims a text and folds its runs of white space into one space
func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
