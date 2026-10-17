// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Explosm is a grabber for Cyanide & Happiness (explosm.net): the /comics
// page embeds the whole archive in its __NEXT_DATA__, one strip (one
// picture) per chapter, numbered from the first strip
type Explosm struct {
	*Grabber
}

// ExplosmChapter is one strip
type ExplosmChapter struct {
	Chapter
	Image string
}

// explosmFiles is where strips with only a file name are served
const explosmFiles = "https://files.explosm.net/comics/"

func NewExplosm(g *Grabber) *Explosm {
	return &Explosm{Grabber: g}
}

// Test returns true for any explosm.net URL: the site is one series
func (e *Explosm) Test() (bool, error) {
	return hostMatchesDomain(e.URL, "explosm.net"), nil
}

// FetchTitle returns the comic's name
func (e *Explosm) FetchTitle() (string, error) {
	return "Cyanide & Happiness", nil
}

// explosmStrip is a strip as the archive lists it
type explosmStrip struct {
	File       *string `json:"file"`
	FileStatic *string `json:"file_static"`
	PublishAt  string  `json:"publish_at"`
	Slug       string  `json:"slug"`
	AuthorName string  `json:"author_name"`
}

// image returns the strip's picture URL, "" for the old Flash animations
func (s explosmStrip) image() string {
	if s.FileStatic != nil && *s.FileStatic != "" {
		return *s.FileStatic
	}
	if s.File == nil || *s.File == "" || strings.HasPrefix(*s.File, "http") {
		return ""
	}
	return explosmFiles + *s.File
}

// FetchChapters returns every strip with a picture, oldest first
func (e *Explosm) FetchChapters() (Filterables, []error) {
	body, err := http.Get(http.RequestParams{URL: e.BaseUrl() + "/comics"})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}
	data := struct {
		Props struct {
			PageProps struct {
				// year → month → strips
				Archive map[string]map[string][]explosmStrip `json:"comicArchiveData"`
			} `json:"pageProps"`
		} `json:"props"`
	}{}
	if err := json.Unmarshal([]byte(doc.Find("script#__NEXT_DATA__").Text()), &data); err != nil {
		return nil, []error{fmt.Errorf("explosm archive: %w", err)}
	}

	var strips []explosmStrip
	for _, months := range data.Props.PageProps.Archive {
		for _, month := range months {
			strips = append(strips, month...)
		}
	}
	sort.SliceStable(strips, func(i, j int) bool { return strips[i].PublishAt < strips[j].PublishAt })

	chapters := Filterables{}
	for _, s := range strips {
		image := s.image()
		if image == "" {
			continue
		}
		chapters = append(chapters, &ExplosmChapter{
			Chapter: Chapter{
				Number: float64(len(chapters) + 1),
				Title:  s.Slug,
				ID:     s.Slug,
				Label:  Label{Style: LabelNumbered},
			},
			Image: image,
		})
	}

	return chapters, nil
}

// FetchChapter returns the strip's one picture, already listed with it
func (e *Explosm) FetchChapter(f Filterable) (*Chapter, error) {
	echap := f.(*ExplosmChapter)
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   "en",
		ID:         echap.ID,
		PagesCount: 1,
		Pages:      []Page{{Number: 1, URL: echap.Image}},
	}, nil
}
