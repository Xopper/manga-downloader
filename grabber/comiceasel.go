// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// ComicEasel is a grabber for WordPress web comics run by the Comic Easel
// plugin, the ComicPress theme or the Toocheke theme, which keeps the plugin's
// post type and chapters (killsixbilliondemons.com, dumbingofage.com,
// irovedout.com, grrlpowercomic.com, vixenlogic.com). The comic is the series. Its chapters
// are the plugin's chapters (storylines) when the site files its strips under
// them, else every strip is a chapter. It matches by markup, from any page of
// the comic, and reads the chapters from, in this order:
//
//   - the plugin's archive pages (`[comic-archive]`), which list every
//     chapter with its strips, found through the page's links to "archive" pages
//   - its chapter dropdown (the plugin's widget, or the block theme's
//     categories block for the chapters taxonomy); a chapter's strips come
//     from its archive pages or, when the site has it, the REST API
//   - ComicPress' archive page template (a table of every strip, by year)
//   - the REST API's list of strips
type ComicEasel struct {
	*Grabber
	doc      *goquery.Document
	language string
	title    string
	chapters Filterables
	listed   bool
	// rest is 1 when the site's REST API serves the comic post type, -1 when
	// it doesn't, 0 when not asked yet
	rest int
}

// ComicEaselChapter is a chapter of a ComicEasel comic. Only one of its
// sources is set.
type ComicEaselChapter struct {
	Chapter
	// Strips are the chapter's strip pages, in reading order
	Strips []string
	// Archive is the chapter's archive page, walked page after page
	Archive string
	// Slug is the chapter's term slug, for the REST API and the archive query
	Slug string
	// First is the chapter's first strip, whose page links the chapter archive
	First string
}

// NewComicEasel returns a new ComicEasel grabber
func NewComicEasel(g *Grabber) *ComicEasel {
	return &ComicEasel{Grabber: g}
}

// comicEaselMarkup is markup only Comic Easel and ComicPress pages have: the
// archive shortcode, the chapter dropdowns, a comic post, the comic
// navigation and the theme's archive template
const comicEaselMarkup = `.comic-archive-chapter-wrap, .ceo_comic_archive_dropdown_widget select, ` +
	`.wp-block-categories-taxonomy-chapters select, article.type-comic, .comic_navi, ` +
	`#comic .comic-nav, #comic.single-comic-wrapper, body.single-comic, body.post-type-archive-comic, ` +
	`body.tax-chapters`

// Test fetches the page and tells whether it's a Comic Easel or ComicPress
// comic. The theme alone isn't enough: a ComicPress blog without the archive
// template has nothing to list.
func (c *ComicEasel) Test() (bool, error) {
	doc, err := wcGetDoc(c.URL, "", true)
	if err != nil {
		return false, err
	}
	c.doc = doc
	c.language = pageLanguage(doc)

	if doc.Find(comicEaselMarkup).Length() > 0 {
		return true, nil
	}
	theme := doc.Find(`body[class*="wp-theme-comicpress"], link[href*="/themes/comicpress"], script[src*="/themes/comicpress"]`).Length() > 0
	return theme && doc.Find("td.archive-title a[href]").Length() > 0, nil
}

// FetchTitle returns the comic's name: the site's
func (c *ComicEasel) FetchTitle() (string, error) {
	if c.doc == nil {
		if _, err := c.Test(); err != nil {
			return "", err
		}
	}
	if c.title == "" {
		c.title = wpSiteName(c.doc)
	}
	if c.title == "" {
		c.title = hostName(c.URL)
	}
	return c.title, nil
}

// FetchChapters returns the comic's chapters, see ComicEasel
func (c *ComicEasel) FetchChapters() (Filterables, []error) {
	if err := c.list(); err != nil {
		return nil, []error{err}
	}
	return c.chapters, nil
}

// list finds the chapters once, from the first source that has any
func (c *ComicEasel) list() error {
	if c.listed {
		return nil
	}
	if c.doc == nil {
		if _, err := c.Test(); err != nil {
			return err
		}
	}

	docs := []*goquery.Document{c.doc}
	if c.doc.Find(".comic-archive-chapter-wrap").Length() == 0 {
		docs = append(docs, c.archivePages()...)
	}

	var err error
	if chapters := ceArchiveChapters(docs); len(chapters) > 0 {
		c.chapters = chapters
	} else if options := ceDropdown(c.doc); len(options) > 0 {
		c.chapters = ceOptionChapters(options)
	} else if strips := c.comicPressStrips(docs); len(strips) > 0 {
		c.chapters = wcStripChapters(strips)
	} else if strips, err = c.restStrips(); len(strips) > 0 {
		c.chapters = wcStripChapters(strips)
	}
	if err != nil {
		return err
	}
	if len(c.chapters) == 0 {
		return errors.New("no strips found in the comic's archive")
	}
	for _, ch := range c.chapters {
		ch.(*ComicEaselChapter).Language = c.language
	}
	c.listed = true
	return nil
}

// ceMaxArchivePages bounds the archive pages a listing reads (irovedout.com
// has one per book)
const ceMaxArchivePages = 15

// archivePages fetches the pages the page links to on the same site whose
// path names an archive, skipping those that don't answer
func (c *ComicEasel) archivePages() []*goquery.Document {
	base, err := url.Parse(c.URL)
	if err != nil {
		return nil
	}
	var docs []*goquery.Document
	seen := map[string]bool{strings.TrimSuffix(base.Path, "/"): true}
	c.doc.Find("a[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		link, err := url.Parse(resolveURLRelative(s.AttrOr("href", ""), base))
		if err != nil || !strings.EqualFold(link.Hostname(), base.Hostname()) {
			return true
		}
		path := strings.TrimSuffix(link.Path, "/")
		if !strings.Contains(strings.ToLower(path), "archive") || link.RawQuery != "" || seen[path] {
			return true
		}
		seen[path] = true
		if doc, err := wcGetDoc(link.String(), c.URL, true); err == nil {
			docs = append(docs, doc)
		}
		return len(seen) <= ceMaxArchivePages
	})
	return docs
}

// ceArchiveChapters reads the chapters of the plugin's archive pages, each a
// heading and the links to its strips, in the pages' order
func ceArchiveChapters(docs []*goquery.Document) Filterables {
	var names []string
	var strips [][]string
	seen := map[string]bool{}
	for _, doc := range docs {
		base := docURL(doc)
		doc.Find(".comic-archive-chapter-wrap").Each(func(_ int, wrap *goquery.Selection) {
			// a nested chapter's strips are its own, not its parent's
			links := wrap.Find(".comic-archive-title a[href]").FilterFunction(func(_ int, a *goquery.Selection) bool {
				return a.Closest(".comic-archive-chapter-wrap").IsSelection(wrap)
			})
			var list []string
			links.Each(func(_ int, a *goquery.Selection) {
				if u := resolveURLRelative(a.AttrOr("href", ""), base); u != "" && !seen[u] {
					seen[u] = true
					list = append(list, u)
				}
			})
			if len(list) == 0 {
				return
			}
			heading := wrap.Find(".comic-archive-chapter").First()
			names = append(names, sanitizeTitle(heading.Text()))
			strips = append(strips, list)
		})
	}

	numbers := wcNumbers(names)
	var chapters Filterables
	for i, name := range names {
		chapters = append(chapters, &ComicEaselChapter{
			Chapter: wcChapter(name, numbers[i]),
			Strips:  strips[i],
		})
	}
	return chapters
}

// ceOption is a chapter of a dropdown
type ceOption struct {
	name  string
	value string
	count int
	level int
}

// ceCountRe reads the strip count the plugin's dropdown puts after a name
var ceCountRe = regexp.MustCompile(`^(.*?)\s*\((\d+)\)$`)

// ceLevelRe reads the depth WordPress gives an option of a hierarchical dropdown
var ceLevelRe = regexp.MustCompile(`\blevel-(\d+)\b`)

// ceDropdown reads the page's chapter dropdown: the plugin's widget, whose
// options link to the chapter archive (or its first strip) and count its
// strips, or the categories block, whose options are the chapters' slugs
func ceDropdown(doc *goquery.Document) []ceOption {
	selects := doc.Find(".ceo_comic_archive_dropdown_widget select, .wp-block-categories-taxonomy-chapters select, select#chapter")
	var options []ceOption
	selects.First().Find("option").Each(func(_ int, s *goquery.Selection) {
		value := strings.TrimSpace(s.AttrOr("value", ""))
		if value == "" || value == "0" || value == "-1" {
			return
		}
		name := sanitizeTitle(strings.ReplaceAll(s.Text(), " ", " "))
		o := ceOption{name: name, value: value}
		if m := ceCountRe.FindStringSubmatch(name); m != nil {
			o.name = m[1]
			o.count, _ = strconv.Atoi(m[2])
		}
		if m := ceLevelRe.FindStringSubmatch(s.AttrOr("class", "")); m != nil {
			o.level, _ = strconv.Atoi(m[1])
		}
		options = append(options, o)
	})
	return options
}

// ceIsParent tells whether the option at i only groups the chapters after it:
// a deeper one follows it, or, in a flat dropdown with counts, two or more of
// the following ones add up to its count exactly. A book whose own cover
// counts too isn't, and lists only the cover (see walkArchive).
func ceIsParent(options []ceOption, i int) bool {
	if i+1 < len(options) && options[i+1].level > options[i].level {
		return true
	}
	if options[i].count == 0 {
		return false
	}
	sum := 0
	for j := i + 1; j < len(options) && options[j].level == options[i].level; j++ {
		sum += options[j].count
		if sum >= options[i].count {
			return sum == options[i].count && j >= i+2
		}
	}
	return false
}

// ceBareNumberRe matches a chapter named by its number alone, like "3" under
// "Book 2" (killsixbilliondemons.com)
var ceBareNumberRe = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

// ceOptionChapters turns the dropdown's chapters into chapters, leaving out
// those that only group others. A top-level group (a book) makes a volume of
// its chapters, and a chapter named by a bare number is named after its group
// too. A flat dropdown has no levels, so there a name followed by numbers is
// the group.
func ceOptionChapters(options []ceOption) Filterables {
	hierarchical := slices.ContainsFunc(options, func(o ceOption) bool { return o.level > 0 })
	var names, values []string
	var volumes []int
	group, volume := "", 0
	for i, o := range options {
		bare := ceBareNumberRe.MatchString(o.name)
		switch {
		case hierarchical && o.level == 0:
			group = o.name
			if ceIsParent(options, i) {
				volume++
			}
		case !hierarchical && !bare:
			group = o.name
			if i+1 < len(options) && ceBareNumberRe.MatchString(options[i+1].name) {
				volume++
			}
		}
		if ceIsParent(options, i) {
			continue
		}
		name := o.name
		if bare && group != "" {
			name = group + " - " + o.name
		}
		names = append(names, name)
		values = append(values, o.value)
		if hierarchical && o.level == 0 {
			volumes = append(volumes, 0)
		} else {
			volumes = append(volumes, volume)
		}
	}
	if len(names) == 0 {
		return nil
	}
	if slices.Contains(volumes, 0) {
		// some chapters are in no group: the groups aren't volumes
		volumes = make([]int, len(volumes))
	}

	numbers := wcNumbers(names)
	var chapters Filterables
	for i, name := range names {
		ch := &ComicEaselChapter{Chapter: wcChapter(name, numbers[i])}
		ch.Volume = volumes[i]
		switch u, err := url.Parse(values[i]); {
		case err != nil || u.Host == "":
			ch.Slug = values[i]
		case strings.Contains(u.Path, "/chapter/") || u.Query().Has("chapters"):
			ch.Archive = values[i]
		default:
			ch.First = values[i]
		}
		chapters = append(chapters, ch)
	}
	return chapters
}

// comicPressStrips reads ComicPress' archive template, which lists every
// strip of a year in a table, with links to the other years: the strips of
// every year, oldest first
func (c *ComicEasel) comicPressStrips(docs []*goquery.Document) []wcStrip {
	var page *goquery.Document
	for _, doc := range docs {
		if doc.Find("td.archive-title a[href]").Length() > 0 {
			page = doc
			break
		}
	}
	if page == nil {
		return nil
	}

	base := docURL(page)
	years := map[int]*goquery.Document{}
	current, _ := strconv.Atoi(strings.TrimSpace(page.Find(".year-title").First().Text()))
	years[current] = page
	page.Find(".archive-yearlist a[href]").Each(func(_ int, a *goquery.Selection) {
		link := resolveURLRelative(a.AttrOr("href", ""), base)
		u, err := url.Parse(link)
		if err != nil {
			return
		}
		year, err := strconv.Atoi(u.Query().Get("archive_year"))
		if err != nil || years[year] != nil {
			return
		}
		if doc, err := wcGetDoc(link, c.URL, false); err == nil {
			years[year] = doc
		}
	})

	order := make([]int, 0, len(years))
	for year := range years {
		order = append(order, year)
	}
	slices.Sort(order)

	var strips []wcStrip
	for _, year := range order {
		doc := years[year]
		var list []wcStrip
		doc.Find("td.archive-title a[href]").Each(func(_ int, a *goquery.Selection) {
			if u := resolveURLRelative(a.AttrOr("href", ""), docURL(doc)); u != "" {
				list = append(list, wcStrip{URL: u, Title: sanitizeTitle(a.Text())})
			}
		})
		strips = append(strips, wcOldestFirst(list)...)
	}
	return strips
}

// wcDatedRe reads the date WordPress puts in a post's link (/2008/10/27/)
var wcDatedRe = regexp.MustCompile(`/((?:19|20)\d\d)/(\d\d)/(\d\d)/`)

// wcOldestFirst reverses a list of strips whose links' dates say it's newest
// first (sandraandwoo.com's archive)
func wcOldestFirst(list []wcStrip) []wcStrip {
	if len(list) < 2 {
		return list
	}
	first := wcDatedRe.FindStringSubmatch(list[0].URL)
	last := wcDatedRe.FindStringSubmatch(list[len(list)-1].URL)
	if first != nil && last != nil && strings.Join(first[1:], "") > strings.Join(last[1:], "") {
		slices.Reverse(list)
	}
	return list
}

// ceRESTItem is the part of a REST comic post or media item the grabber reads
type ceRESTItem struct {
	ID    int    `json:"id"`
	Link  string `json:"link"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	FeaturedMedia int    `json:"featured_media"`
	SourceURL     string `json:"source_url"`
}

// hasREST tells whether the site's REST API serves the comic post type
func (c *ComicEasel) hasREST() bool {
	if c.rest == 0 {
		c.rest = -1
		body, err := http.GetText(http.RequestParams{
			URL:          c.BaseUrl() + "/wp-json/wp/v2/comic?per_page=1&_fields=id",
			Referer:      c.URL,
			DisableRetry: true,
		})
		var items []ceRESTItem
		if err == nil && json.Unmarshal([]byte(body), &items) == nil {
			c.rest = 1
		}
	}
	return c.rest == 1
}

// restItems reads every page of a REST list (query without the page), 100 at
// a time
func (c *ComicEasel) restItems(query string) ([]ceRESTItem, error) {
	var items []ceRESTItem
	for page := 1; ; page++ {
		body, err := http.GetText(http.RequestParams{
			URL:     fmt.Sprintf("%s/wp-json/wp/v2/%s&per_page=%d&page=%d", c.BaseUrl(), query, wpPerPage, page),
			Referer: c.URL,
		})
		var statusErr *http.StatusError
		if page > 1 && errors.As(err, &statusErr) && statusErr.Code == 400 {
			break
		}
		if err != nil {
			return nil, err
		}
		var list []ceRESTItem
		if err := json.Unmarshal([]byte(body), &list); err != nil {
			return nil, err
		}
		items = append(items, list...)
		if len(list) < wpPerPage {
			break
		}
	}
	return items, nil
}

// restStrips lists every strip through the REST API, oldest first, for sites
// whose strips have no chapters and no archive page (vixenlogic.com)
func (c *ComicEasel) restStrips() ([]wcStrip, error) {
	if !c.hasREST() {
		return nil, nil
	}
	items, err := c.restItems("comic?orderby=date&order=asc&_fields=link,title")
	if err != nil {
		return nil, err
	}
	strips := make([]wcStrip, 0, len(items))
	for _, it := range items {
		strips = append(strips, wcStrip{URL: it.Link, Title: sanitizeTitle(html.UnescapeString(it.Title.Rendered))})
	}
	return strips, nil
}

// FetchChapter fetches a chapter's pictures, from its strips' pages or from
// its archive
func (c *ComicEasel) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*ComicEaselChapter)

	var pages []Page
	var err error
	switch {
	case len(ch.Strips) > 0:
		pages, err = wcStripPages(ch.Strips, c.BaseUrl())
	case ch.Slug != "" && c.hasREST():
		pages, err = c.restChapterPages(ch.Slug)
	case ch.Slug != "":
		pages, err = c.walkArchive(c.BaseUrl()+"/?chapters="+url.QueryEscape(ch.Slug), ch.Slug)
	case ch.Archive != "":
		pages, err = c.walkArchive(ch.Archive, archiveSlug(ch.Archive))
	case ch.First != "":
		var archive string
		if archive, err = ceStripArchive(ch.First, c.BaseUrl()); err == nil {
			pages, err = c.walkArchive(archive, archiveSlug(archive))
		}
	}
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}

	return &Chapter{
		Title:      ch.Title,
		Number:     ch.Number,
		Label:      ch.Label,
		Volume:     ch.Volume,
		PagesCount: int64(len(pages)),
		Language:   c.language,
		Pages:      pages,
	}, nil
}

// archiveSlug is the chapter slug an archive link names: its last path
// segment, or its chapters query
func archiveSlug(archive string) string {
	u, err := url.Parse(archive)
	if err != nil {
		return ""
	}
	if slug := u.Query().Get("chapters"); slug != "" {
		return slug
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return parts[len(parts)-1]
}

// ceStripArchive returns the archive of the chapter a strip is filed under,
// which the strip's page links to
func ceStripArchive(strip, referer string) (string, error) {
	doc, err := wcGetDoc(strip, referer, false)
	if err != nil {
		return "", err
	}
	link := doc.Find(`.comic-chapter a[href], a[rel="tag"][href*="/chapter/"]`).First()
	if link.Length() == 0 {
		return "", fmt.Errorf("couldn't find the chapter of %s", strip)
	}
	return resolveURLRelative(link.AttrOr("href", ""), docURL(doc)), nil
}

// ceMaxArchiveWalk bounds the pages of one chapter archive
const ceMaxArchiveWalk = 100

// ceArchiveEntry is a strip as a chapter archive lists it
type ceArchiveEntry struct {
	id      int
	link    string
	picture string
	// own is set when the post names slug as its chapter
	own bool
}

// cePostIDRe reads a post's ID from its element's id
var cePostIDRe = regexp.MustCompile(`^post-(\d+)$`)

// walkArchive reads a chapter archive page after page and returns its
// strips' pictures, oldest first. A book whose chapters are filed under it
// lists their strips too, so when some posts name slug as their chapter, only
// those are kept. The archive's own picture of a strip (its featured image,
// the comic) saves a request per strip; without one, each strip's page is read.
func (c *ComicEasel) walkArchive(archive, slug string) ([]Page, error) {
	var entries []ceArchiveEntry
	seen := map[string]bool{}
	for page := 1; page <= ceMaxArchiveWalk; page++ {
		doc, err := wcGetDoc(cePagedURL(archive, page), c.BaseUrl(), page > 1)
		var statusErr *http.StatusError
		if page > 1 && errors.As(err, &statusErr) {
			break
		}
		if err != nil {
			return nil, err
		}
		base := docURL(doc)
		added := 0
		doc.Find(`article[id^="post-"], div[id^="post-"].post`).Each(func(_ int, s *goquery.Selection) {
			a := s.Find(".post-title a[href], .entry-title a[href], .comic-thumbnail-in-archive a[href]").First()
			link := resolveURLRelative(a.AttrOr("href", ""), base)
			if link == "" || seen[link] {
				return
			}
			seen[link] = true
			added++
			entry := ceArchiveEntry{link: link, own: slug != "" && s.HasClass("chapters-"+slug)}
			if m := cePostIDRe.FindStringSubmatch(s.AttrOr("id", "")); m != nil {
				entry.id, _ = strconv.Atoi(m[1])
			}
			if img := s.Find(".comic-thumbnail-in-archive img").First(); img.Length() > 0 {
				entry.picture = wcLargestImage(img, base)
			}
			entries = append(entries, entry)
		})
		if added == 0 {
			break
		}
	}

	if slices.ContainsFunc(entries, func(e ceArchiveEntry) bool { return e.own }) {
		entries = slices.DeleteFunc(entries, func(e ceArchiveEntry) bool { return !e.own })
	}
	if len(entries) > 1 && entries[0].id > entries[len(entries)-1].id {
		slices.Reverse(entries)
	}

	var strips []string
	pictures := true
	for _, e := range entries {
		strips = append(strips, e.link)
		pictures = pictures && e.picture != ""
	}
	if !pictures {
		return wcStripPages(strips, c.BaseUrl())
	}
	pages := make([]Page, 0, len(entries))
	for i, e := range entries {
		pages = append(pages, Page{Number: int64(i + 1), URL: e.picture})
	}
	return pages, nil
}

// cePagedURL is the nth page of an archive: WordPress' /page/n/, or paged=n
// for an archive asked by query
func cePagedURL(archive string, n int) string {
	if n == 1 {
		return archive
	}
	u, err := url.Parse(archive)
	if err != nil {
		return archive
	}
	if u.RawQuery != "" {
		q := u.Query()
		q.Set("paged", strconv.Itoa(n))
		u.RawQuery = q.Encode()
		return u.String()
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/page/" + strconv.Itoa(n) + "/"
	return u.String()
}

// restChapterPages lists a chapter's strips through the REST API, oldest
// first, and returns their featured images, which Comic Easel shows as the comic
func (c *ComicEasel) restChapterPages(slug string) ([]Page, error) {
	terms, err := c.restItems("chapters?slug=" + url.QueryEscape(slug) + "&_fields=id")
	if err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return nil, fmt.Errorf("chapter %q isn't in the site's REST API", slug)
	}
	posts, err := c.restItems(fmt.Sprintf("comic?chapters=%d&orderby=date&order=asc&_fields=link,featured_media", terms[0].ID))
	if err != nil {
		return nil, err
	}

	pictures := map[int]string{}
	var ids []string
	for _, p := range posts {
		if p.FeaturedMedia != 0 {
			ids = append(ids, strconv.Itoa(p.FeaturedMedia))
		}
	}
	for start := 0; start < len(ids); start += wpPerPage {
		end := min(start+wpPerPage, len(ids))
		media, err := c.restItems("media?include=" + strings.Join(ids[start:end], ",") + "&_fields=id,source_url")
		if err != nil {
			return nil, err
		}
		for _, m := range media {
			pictures[m.ID] = m.SourceURL
		}
	}

	var pages []Page
	for _, p := range posts {
		if pic := pictures[p.FeaturedMedia]; pic != "" {
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: pic})
			continue
		}
		// a strip without a featured image shows its comic on its page only
		more, err := wcStripPages([]string{p.Link}, c.BaseUrl())
		if err != nil {
			return nil, err
		}
		for _, m := range more {
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: m.URL})
		}
	}
	return pages, nil
}
