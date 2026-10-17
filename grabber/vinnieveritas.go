// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// VinnieVeritas is a grabber for Vinnie Veritas' CCC webcomic
// (ccc.vinnieveritas.com): archiveIndex.php lists every strip id, and each
// strip's page (/{id}.php) holds its picture in English and in Spanish
// (--language es picks the Spanish one)
type VinnieVeritas struct {
	*Grabber
}

// VinnieVeritasChapter is a strip
type VinnieVeritasChapter struct {
	Chapter
	Strip string
}

var (
	// vinnieStripRe reads a strip id from an archive entry's onclick
	vinnieStripRe = regexp.MustCompile(`changeToComic\("([A-Za-z]+(\d+))"\)`)
)

func NewVinnieVeritas(g *Grabber) *VinnieVeritas {
	return &VinnieVeritas{Grabber: g}
}

// Test returns true for any ccc.vinnieveritas.com URL: the site is one comic
func (v *VinnieVeritas) Test() (bool, error) {
	return hostMatchesDomain(v.URL, "ccc.vinnieveritas.com"), nil
}

// spanish reports whether the Spanish edition is wanted
func (v VinnieVeritas) spanish() bool {
	return strings.EqualFold(v.GetPreferredLanguage(), "es")
}

// FetchTitle returns the comic's name in the edition's language
func (v *VinnieVeritas) FetchTitle() (string, error) {
	if v.spanish() {
		return "CCC: La ciudad de las oportunidades", nil
	}
	return "CCC: The city of opportunities", nil
}

// FetchChapters returns the strips the archive lists, oldest first
func (v *VinnieVeritas) FetchChapters() (Filterables, []error) {
	body, err := http.Get(http.RequestParams{URL: v.BaseUrl() + "/archiveIndex.php"})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}
	seen := map[string]bool{}
	chapters := Filterables{}
	doc.Find(".cccArchiveEntry[onclick]").Each(func(_ int, s *goquery.Selection) {
		m := vinnieStripRe.FindStringSubmatch(s.AttrOr("onclick", ""))
		if m == nil || seen[m[1]] {
			return
		}
		seen[m[1]] = true
		number, _ := strconv.ParseFloat(m[2], 64)
		chapters = append(chapters, &VinnieVeritasChapter{
			Chapter: Chapter{Number: number, Title: m[1], ID: m[1], Label: Label{Style: LabelNumbered}},
			Strip:   m[1],
		})
	})
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].GetNumber() < chapters[j].GetNumber() })
	return chapters, nil
}

// FetchChapter reads the strip's picture in the edition's language
func (v *VinnieVeritas) FetchChapter(f Filterable) (*Chapter, error) {
	vchap := f.(*VinnieVeritasChapter)
	page := v.BaseUrl() + "/" + vchap.Strip + ".php"
	body, err := http.Get(http.RequestParams{URL: page, Referer: v.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	lang := "en"
	if v.spanish() {
		lang = "es"
	}
	base, _ := url.Parse(page)
	chapter := &Chapter{Title: f.GetTitle(), Label: labelOf(f), Number: f.GetNumber(), Language: lang, ID: vchap.ID}
	doc.Find("img.cccComic.crazylan-" + lang).Each(func(_ int, img *goquery.Selection) {
		ref, err := url.Parse(strings.TrimSpace(img.AttrOr("src", "")))
		if err != nil {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: base.ResolveReference(ref).String()})
	})
	chapter.PagesCount = int64(len(chapter.Pages))
	return chapter, nil
}
