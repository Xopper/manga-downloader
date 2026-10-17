// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// NamiComi is a grabber for namicomi.com, which serves everything from its
// public API: a title (/title/{id}), its chapters (/chapter?titleId=, 100 a
// page, in every language unless --language picks one) and a chapter's
// pictures (/images/chapter/{id}, a base URL, a hash and the file names,
// served as {base}/chapter/{id}/{hash}/source/{file}).
type NamiComi struct {
	*Grabber
	id string
}

// NamiComiChapter is a chapter of a NamiComi title
type NamiComiChapter struct {
	Chapter
}

// NewNamiComi returns a new NamiComi grabber
func NewNamiComi(g *Grabber) *NamiComi {
	return &NamiComi{Grabber: g}
}

// namiComiAPI is the API's root
const namiComiAPI = "https://api.namicomi.com"

// namiComiTitleRe reads a title's ID from its page's URL (/en/title/TatWWTNm/greek-wine)
var namiComiTitleRe = regexp.MustCompile(`/title/([A-Za-z0-9]+)`)

// Test tells whether the URL is a namicomi.com title
func (n *NamiComi) Test() (bool, error) {
	if !hostMatchesDomain(n.URL, "namicomi.com") {
		return false, nil
	}
	m := namiComiTitleRe.FindStringSubmatch(n.URL)
	if m == nil {
		return false, nil
	}
	n.id = m[1]
	return true, nil
}

// get asks the API for path and decodes its data into v
func (n *NamiComi) get(path string, v any) error {
	body, err := http.GetText(http.RequestParams{URL: namiComiAPI + path, Referer: "https://namicomi.com/", Accept: "application/json"})
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), v)
}

// FetchTitle returns the title's name, in English when it has one
func (n *NamiComi) FetchTitle() (string, error) {
	var res struct {
		Data struct {
			Attributes struct {
				Title map[string]string `json:"title"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := n.get("/title/"+n.id, &res); err != nil {
		return "", err
	}
	titles := res.Data.Attributes.Title
	if t := titles["en"]; t != "" {
		return t, nil
	}
	for _, t := range titles {
		return t, nil
	}
	return "", fmt.Errorf("title %s has no name", n.id)
}

// namiComiPage is the chapter list's page size
const namiComiPage = 100

// FetchChapters returns the title's chapters, in the language asked for, or all
func (n *NamiComi) FetchChapters() (Filterables, []error) {
	var chapters Filterables
	lang := n.Settings.Language
	for offset := 0; ; offset += namiComiPage {
		q := url.Values{
			"titleId":        {n.id},
			"limit":          {strconv.Itoa(namiComiPage)},
			"offset":         {strconv.Itoa(offset)},
			"order[volume]":  {"asc"},
			"order[chapter]": {"asc"},
			"includes[]":     {"organization"},
		}
		if lang != "" && lang != "all" {
			q.Set("translatedLanguages[]", lang)
		}
		var res struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Volume   *string `json:"volume"`
					Chapter  string  `json:"chapter"`
					Name     string  `json:"name"`
					Language string  `json:"translatedLanguage"`
				} `json:"attributes"`
			} `json:"data"`
			Meta struct {
				Total int `json:"total"`
			} `json:"meta"`
		}
		if err := n.get("/chapter?"+q.Encode(), &res); err != nil {
			return chapters, []error{err}
		}
		for _, c := range res.Data {
			num, err := strconv.ParseFloat(c.Attributes.Chapter, 64)
			if err != nil {
				num = float64(len(chapters) + 1)
			}
			ch := Chapter{
				Title:    sanitizeTitle(c.Attributes.Name),
				Number:   num,
				ID:       c.ID,
				Language: c.Attributes.Language,
				Label:    Label{Style: LabelPrefixed},
			}
			if c.Attributes.Volume != nil {
				ch.Volume, _ = strconv.Atoi(*c.Attributes.Volume)
			}
			chapters = append(chapters, &NamiComiChapter{Chapter: ch})
		}
		if len(res.Data) < namiComiPage || offset+namiComiPage >= res.Meta.Total {
			break
		}
	}
	return chapters, nil
}

// FetchChapter fetches a chapter's pictures, in their source quality
func (n *NamiComi) FetchChapter(f Filterable) (*Chapter, error) {
	ch := f.(*NamiComiChapter)
	var res struct {
		Data struct {
			BaseURL string `json:"baseUrl"`
			Hash    string `json:"hash"`
			Source  []struct {
				Filename string `json:"filename"`
			} `json:"source"`
		} `json:"data"`
	}
	if err := n.get("/images/chapter/"+ch.ID+"?newQualities=true", &res); err != nil {
		return nil, err
	}
	var pages []Page
	for _, s := range res.Data.Source {
		pages = append(pages, Page{
			Number: int64(len(pages) + 1),
			URL:    fmt.Sprintf("%s/chapter/%s/%s/source/%s", res.Data.BaseURL, ch.ID, res.Data.Hash, s.Filename),
		})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      ch.Title,
		Number:     ch.Number,
		Label:      ch.Label,
		ID:         ch.ID,
		Volume:     ch.Volume,
		PagesCount: int64(len(pages)),
		Language:   ch.Language,
		Pages:      pages,
	}, nil
}
