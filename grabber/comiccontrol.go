// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// ComicControl is a grabber for webcomics run on the ComicControl CMS
// (egscomics.com, snafu-comics.com): a site holds several comics, one per
// first path segment, and each comic's /{comic}/archive page lists every
// strip in a <select>. A strip is a chapter, its page read when downloaded.
type ComicControl struct {
	*Grabber
	archive *goquery.Document
}

// ComicControlChapter is one strip
type ComicControlChapter struct {
	Chapter
	URL string
}

func NewComicControl(g *Grabber) *ComicControl {
	return &ComicControl{Grabber: g}
}

// Test fetches the comic's archive page and accepts it when it's a
// ComicControl one (its stylesheet and strip <select>), so every site works
// without a domain list
func (c *ComicControl) Test() (bool, error) {
	if c.section() == "" {
		return false, nil
	}
	doc, err := c.fetchArchive(true)
	if err != nil {
		return false, nil
	}
	isCC := doc.Find(`link[href*="comiccontrol/"]`).Length() > 0
	return isCC && doc.Find(`select[name="comic"] option[value^="`+c.section()+`/"]`).Length() > 0, nil
}

// section returns the comic's path segment, the first one of the URL
func (c ComicControl) section() string {
	u, err := url.Parse(c.URL)
	if err != nil {
		return ""
	}
	section, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	return section
}

// fetchArchive fetches the comic's archive page once; probe skips the
// retries, for identifying a site
func (c *ComicControl) fetchArchive(probe bool) (*goquery.Document, error) {
	if c.archive != nil {
		return c.archive, nil
	}
	body, err := http.Get(http.RequestParams{
		URL:          c.BaseUrl() + "/" + c.section() + "/archive",
		DisableRetry: probe,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	c.archive = doc
	return doc, nil
}

// FetchTitle returns the site and comic names the archive's <title> gives
// ("El Goonish Shive - EGS:NP - Archive" is "El Goonish Shive - EGS:NP")
func (c *ComicControl) FetchTitle() (string, error) {
	doc, err := c.fetchArchive(false)
	if err != nil {
		return "", err
	}
	var parts []string
	for _, part := range strings.Split(doc.Find("title").First().Text(), " - ") {
		if part = strings.TrimSpace(part); part != "" && part != "Archive" {
			parts = append(parts, part)
		}
	}
	return sanitizeTitle(strings.Join(parts, " - ")), nil
}

// FetchChapters returns the comic's strips in the archive's order (oldest
// first), numbered by position, titled by the option's text after the date
func (c *ComicControl) FetchChapters() (Filterables, []error) {
	doc, err := c.fetchArchive(false)
	if err != nil {
		return nil, []error{err}
	}
	chapters := Filterables{}
	doc.Find(`select[name="comic"] option[value^="` + c.section() + `/"]`).Each(func(_ int, o *goquery.Selection) {
		value := o.AttrOr("value", "")
		title := strings.TrimSpace(o.Text())
		if _, after, ok := strings.Cut(title, " - "); ok {
			title = strings.TrimSpace(after)
		}
		chapters = append(chapters, &ComicControlChapter{
			Chapter: Chapter{
				Number: float64(len(chapters) + 1),
				Title:  title,
				ID:     value,
				Label:  Label{Style: LabelNumbered},
			},
			URL: c.BaseUrl() + "/" + value,
		})
	})

	return chapters, nil
}

// FetchChapter reads the strip's page for its picture(s)
func (c *ComicControl) FetchChapter(f Filterable) (*Chapter, error) {
	cchap := f.(*ComicControlChapter)
	body, err := http.Get(http.RequestParams{URL: cchap.URL, Referer: c.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(cchap.URL)

	chapter := &Chapter{
		Title:  f.GetTitle(),
		Label:  labelOf(f),
		Number: f.GetNumber(),
		ID:     cchap.ID,
	}
	seen := map[string]bool{}
	doc.Find(`#cc-comicbody img#cc-comic, #cc-comicbody img[src*="/comics/"]`).Each(func(_ int, img *goquery.Selection) {
		src, err := base.Parse(strings.TrimSpace(img.AttrOr("src", "")))
		if err != nil || seen[src.String()] {
			return
		}
		seen[src.String()] = true
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: src.String()})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
