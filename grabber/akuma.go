// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

var (
	akumaHostRe    = regexp.MustCompile(`(?i)^https?://(?:www\.)?akuma\.moe(?:[/?#]|$)`)
	akumaGalleryRe = regexp.MustCompile(`^/g/([A-Za-z0-9]+)`)
	akumaPrefixRe  = regexp.MustCompile(`img_prt\s*=\s*"([^"]+)"`)
	// akumaPageTitleRe is what the reader's <title> adds around the gallery's
	akumaPageTitleRe = regexp.MustCompile(`^\s*Page \d+ - |\s*—\s*akuma\.moe\s*$`)
	akumaLangRe      = regexp.MustCompile(`(?i)\[(english|japanese|chinese|korean|spanish|french|russian|german|portuguese|italian|vietnamese|thai|indonesian)\]`)
)

// Akuma is a grabber for akuma.moe, a doujinshi gallery site behind
// DDoS-Guard (see ddosGuardPass). A gallery (/g/{id}) is a series with one
// chapter. The reader page names the image host's folder (img_prt) but only
// the first few files; the full list is what the reader then POSTs for to the
// gallery URL, with the page's CSRF token and its session cookie.
type Akuma struct {
	*Grabber
	id     string
	title  string
	prefix string
	csrf   string
}

func NewAkuma(g *Grabber) *Akuma {
	return &Akuma{Grabber: g}
}

// AkumaChapter is the gallery, the only chapter of its series
type AkumaChapter struct {
	Chapter
}

// Test returns true for akuma.moe URLs
func (a *Akuma) Test() (bool, error) {
	return akumaHostRe.MatchString(a.URL), nil
}

// akumaGalleryID reads the gallery id off a gallery or reader URL
func akumaGalleryID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	m := akumaGalleryRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errors.New("not an akuma.moe gallery URL (/g/{id})")
	}

	return m[1], nil
}

// fetchReader reads the title, the image folder and the CSRF token off the
// gallery's reader page, once
func (a *Akuma) fetchReader() error {
	if a.prefix != "" {
		return nil
	}
	id, err := akumaGalleryID(a.URL)
	if err != nil {
		return err
	}
	ddosGuardPass(a.BaseUrl())
	page, err := http.GetText(http.RequestParams{URL: a.BaseUrl() + "/g/" + id + "/1"})
	if err != nil {
		return err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		return err
	}
	m := akumaPrefixRe.FindStringSubmatch(page)
	if m == nil {
		return fmt.Errorf("could not find the pictures of gallery %s", id)
	}
	a.id = id
	a.prefix = strings.TrimRight(m[1], "/")
	a.csrf = doc.Find(`meta[name="csrf-token"]`).AttrOr("content", "")
	a.title = doc.Find(".entry-title").First().Text()
	if a.title == "" {
		a.title = akumaPageTitleRe.ReplaceAllString(doc.Find("title").Text(), "")
	}

	return nil
}

// FetchTitle returns the gallery title
func (a *Akuma) FetchTitle() (string, error) {
	if err := a.fetchReader(); err != nil {
		return "", err
	}

	return sanitizeTitle(a.title), nil
}

// FetchChapters returns the gallery as the only chapter
func (a *Akuma) FetchChapters() (Filterables, []error) {
	if err := a.fetchReader(); err != nil {
		return nil, []error{err}
	}

	return Filterables{&AkumaChapter{Chapter{Number: 1, ID: a.id, Language: akumaLanguage(a.title), Label: Label{Style: LabelUnnumbered}}}}, nil
}

// akumaLanguage reads the language off the title's "[English]" tag
func akumaLanguage(title string) string {
	codes := map[string]string{"english": "en", "japanese": "ja", "chinese": "zh", "korean": "ko", "spanish": "es", "french": "fr", "russian": "ru", "german": "de", "portuguese": "pt", "italian": "it", "vietnamese": "vi", "thai": "th", "indonesian": "id"}
	if m := akumaLangRe.FindStringSubmatch(title); m != nil {
		return codes[strings.ToLower(m[1])]
	}

	return ""
}

// FetchChapter asks for the gallery's full file list
func (a *Akuma) FetchChapter(f Filterable) (*Chapter, error) {
	if err := a.fetchReader(); err != nil {
		return nil, err
	}
	text, err := http.PostText(http.RequestParams{
		URL:     a.BaseUrl() + "/g/" + a.id,
		Referer: a.BaseUrl() + "/g/" + a.id + "/1",
		Accept:  "application/json",
		Headers: map[string]string{"X-CSRF-TOKEN": a.csrf, "X-Requested-With": "XMLHttpRequest"},
	})
	if err != nil {
		return nil, err
	}
	files := []string{}
	if err = json.Unmarshal([]byte(text), &files); err != nil || len(files) == 0 {
		return nil, fmt.Errorf("could not read the pictures of gallery %s", a.id)
	}

	chapter := &Chapter{Title: f.GetTitle(), Number: f.GetNumber(), ID: a.id, Language: f.(*AkumaChapter).Language, Label: labelOf(f)}
	for i, file := range files {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: a.prefix + "/" + file})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
