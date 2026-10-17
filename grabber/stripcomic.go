// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// StripComic is a grabber for one-series webcomics whose strips are listed
// in one request and each read from its own page (SUPER MEGA, Patch Friday,
// Loading Artist): a strip is a chapter, and its page's pictures, found with
// the site's selector, are its pages
type StripComic struct {
	*Grabber
	site stripSite
}

// StripComicChapter is one strip
type StripComicChapter struct {
	Chapter
	URL string
}

// stripSite is how one webcomic lists its strips and shows their pictures
type stripSite struct {
	domain string
	title  string
	// list returns the strips, oldest first, numbered and with their pages
	list func(s *StripComic) ([]*StripComicChapter, error)
	// image selects the strip page's pictures
	image string
}

// stripSites are the webcomics StripComic reads
var stripSites = []stripSite{
	{
		// strips are index.php?i=1… up to the one the home page shows
		domain: "supermegacomics.com",
		title:  "SUPER MEGA",
		list:   superMegaStrips,
		image:  `img[border="4"]`,
	},
	{
		// strips are /1/… up to the one the home page shows
		domain: "patchfriday.com",
		title:  "Patch Friday",
		list:   patchFridayStrips,
		image:  "#strip_image img",
	},
	{
		// the site's search index lists every page; comics, games and art
		// are strips, news isn't
		domain: "loadingartist.com",
		title:  "Loading Artist",
		list:   loadingArtistStrips,
		image:  ".main-image-container img",
	},
}

var (
	// superMegaLatestRe reads the latest strip number off its picture
	superMegaLatestRe = regexp.MustCompile(`/images/(\d+)\.`)
	// patchFridayLatestRe reads the latest strip number off its picture
	patchFridayLatestRe = regexp.MustCompile(`/patches/(\d+)\.`)
)

func NewStripComic(g *Grabber) *StripComic {
	return &StripComic{Grabber: g}
}

// Test returns true for a URL of one of the webcomics
func (s *StripComic) Test() (bool, error) {
	for _, site := range stripSites {
		if hostMatchesDomain(s.URL, site.domain) {
			s.site = site
			return true, nil
		}
	}
	return false, nil
}

// FetchTitle returns the webcomic's name
func (s *StripComic) FetchTitle() (string, error) {
	return s.site.title, nil
}

// FetchChapters returns the strips
func (s *StripComic) FetchChapters() (Filterables, []error) {
	if s.site.list == nil {
		return nil, []error{errors.New("not a known webcomic")}
	}
	strips, err := s.site.list(s)
	if err != nil {
		return nil, []error{err}
	}
	chapters := make(Filterables, 0, len(strips))
	for _, strip := range strips {
		chapters = append(chapters, strip)
	}
	return chapters, nil
}

// fetchDoc fetches a page of the site
func (s *StripComic) fetchDoc(uri string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: s.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

// FetchChapter reads the strip's page for its pictures
func (s *StripComic) FetchChapter(f Filterable) (*Chapter, error) {
	schap := f.(*StripComicChapter)
	doc, err := s.fetchDoc(schap.URL)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(schap.URL)
	chapter := &Chapter{Title: f.GetTitle(), Label: labelOf(f), Number: f.GetNumber(), Language: "en", ID: schap.ID}
	doc.Find(s.site.image).Each(func(_ int, img *goquery.Selection) {
		ref, err := url.Parse(strings.TrimSpace(img.AttrOr("src", "")))
		if err != nil || ref.String() == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: base.ResolveReference(ref).String()})
	})
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}

// numberedStrips returns strips 1…latest at the URLs pattern gives
func numberedStrips(latest int, pattern string) []*StripComicChapter {
	strips := make([]*StripComicChapter, 0, latest)
	for n := 1; n <= latest; n++ {
		strips = append(strips, &StripComicChapter{
			Chapter: Chapter{Number: float64(n), ID: strconv.Itoa(n), Label: Label{Style: LabelNumbered}},
			URL:     fmt.Sprintf(pattern, n),
		})
	}
	return strips
}

// latestStrip reads the latest strip number off the home page's picture
func (s *StripComic) latestStrip(image string, re *regexp.Regexp) (int, error) {
	doc, err := s.fetchDoc(s.BaseUrl() + "/")
	if err != nil {
		return 0, err
	}
	m := re.FindStringSubmatch(doc.Find(image).First().AttrOr("src", ""))
	if m == nil {
		return 0, errors.New("couldn't find the latest strip")
	}
	return strconv.Atoi(m[1])
}

// superMegaStrips lists SUPER MEGA's strips
func superMegaStrips(s *StripComic) ([]*StripComicChapter, error) {
	latest, err := s.latestStrip(s.site.image, superMegaLatestRe)
	if err != nil {
		return nil, err
	}
	return numberedStrips(latest, s.BaseUrl()+"/index.php?i=%d"), nil
}

// patchFridayStrips lists Patch Friday's strips
func patchFridayStrips(s *StripComic) ([]*StripComicChapter, error) {
	latest, err := s.latestStrip(s.site.image, patchFridayLatestRe)
	if err != nil {
		return nil, err
	}
	return numberedStrips(latest, s.BaseUrl()+"/%d/"), nil
}

// loadingArtistStrips lists Loading Artist's comics, games and art by date
func loadingArtistStrips(s *StripComic) ([]*StripComicChapter, error) {
	body, err := http.GetText(http.RequestParams{URL: s.BaseUrl() + "/search.json", Referer: s.BaseUrl(), Accept: "application/json"})
	if err != nil {
		return nil, err
	}
	var index []struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Date    string `json:"date"`
		Section string `json:"section"`
	}
	if err := json.Unmarshal([]byte(body), &index); err != nil {
		return nil, err
	}
	sort.SliceStable(index, func(i, j int) bool { return index[i].Date < index[j].Date })
	var strips []*StripComicChapter
	for _, e := range index {
		if e.Section != "comic" && e.Section != "game" && e.Section != "art" {
			continue
		}
		strips = append(strips, &StripComicChapter{
			Chapter: Chapter{Number: float64(len(strips) + 1), Title: e.Title, ID: e.URL, Label: Label{Style: LabelNumbered}},
			URL:     s.BaseUrl() + e.URL,
		})
	}
	return strips, nil
}
