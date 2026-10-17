// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Inkr is a grabber for INKR Comics (comics.inkr.com): the web pages no
// longer show the reader, so everything comes from the app's content API, a
// batch POST of object ids (ik-title-N, ik-chapter-N) and the fields wanted.
// Free chapters (revenueType "ad" or "free") are readable logged out; the
// others are locked. Pictures are AES-256-CBC encrypted .ikc files, undone
// by each page's Transform.
type Inkr struct {
	*Grabber
	title *inkrTitle
}

// InkrChapter is a chapter of the title
type InkrChapter struct {
	Chapter
	OID string
}

// inkrTitle is the part of a title the API gives
type inkrTitle struct {
	Name     string   `json:"name"`
	Chapters []string `json:"chapterList"`
}

var (
	// inkrAPI is the content API, a var so tests can point it elsewhere
	inkrAPI = "https://icd-api.inkr.com/v1/content_json/batch"
	// inkrTitleRe finds a title's object id in a URL
	inkrTitleRe = regexp.MustCompile(`ik-title-\d+`)
	// inkrKey is the .ikc key: the bytes the app ships XORed with 0x5A
	inkrKey = []byte("EMQKcwYqQtlH29KzSZsDobHL1mHvzotl")
)

// inkrBatchSize is how many chapters one request asks about
const inkrBatchSize = 50

// inkrImageVariant is the picture size asked for; the plain URL answers 404
const inkrImageVariant = "/w1600.ikc"

func NewInkr(g *Grabber) *Inkr {
	return &Inkr{Grabber: g}
}

// Test returns true for a comics.inkr.com title URL
func (i *Inkr) Test() (bool, error) {
	return hostMatchesDomain(i.URL, "inkr.com") && inkrTitleRe.MatchString(i.URL), nil
}

// batch asks the content API for the fields of the objects, decoding each
// object's answer into data
func (i Inkr) batch(query map[string]any, data any) error {
	body, err := json.Marshal([]map[string]any{query})
	if err != nil {
		return err
	}
	text, err := http.PostText(http.RequestParams{
		URL:    inkrAPI,
		Body:   string(body),
		Accept: "application/json",
		Headers: map[string]string{
			"Content-Type": "application/json",
			"ikc-platform": "android",
			"cf-ipcountry": "en-GB",
		},
	})
	if err != nil {
		return err
	}
	var feed struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &feed); err != nil {
		return err
	}
	if feed.Code != 0 {
		return fmt.Errorf("inkr: code %d %s", feed.Code, feed.Message)
	}
	return json.Unmarshal(feed.Data, data)
}

// fetchTitle fetches the title's name and chapter ids once
func (i *Inkr) fetchTitle() (*inkrTitle, error) {
	if i.title != nil {
		return i.title, nil
	}
	oid := inkrTitleRe.FindString(i.URL)
	titles := map[string]inkrTitle{}
	if err := i.batch(map[string]any{"fields": []string{"oid", "name", "chapterList"}, "oids": []string{oid}}, &titles); err != nil {
		return nil, err
	}
	title, ok := titles[oid]
	if !ok {
		return nil, fmt.Errorf("inkr: no title %s", oid)
	}
	i.title = &title
	return i.title, nil
}

// FetchTitle returns the title's name
func (i *Inkr) FetchTitle() (string, error) {
	title, err := i.fetchTitle()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(title.Name), nil
}

// FetchChapters returns the title's chapters, numbered by their order, the
// ones that need coins or a subscription locked
func (i *Inkr) FetchChapters() (Filterables, []error) {
	title, err := i.fetchTitle()
	if err != nil {
		return nil, []error{err}
	}
	chapters := Filterables{}
	for start := 0; start < len(title.Chapters); start += inkrBatchSize {
		oids := title.Chapters[start:min(start+inkrBatchSize, len(title.Chapters))]
		details := map[string]struct {
			Name        string  `json:"name"`
			Order       float64 `json:"order"`
			RevenueType string  `json:"revenueType"`
		}{}
		query := map[string]any{"fields": []string{"oid", "name", "order", "revenueType"}, "oids": oids}
		if err := i.batch(query, &details); err != nil {
			return chapters, []error{err}
		}
		for _, oid := range oids {
			d, ok := details[oid]
			if !ok {
				continue
			}
			revenue := strings.ToLower(d.RevenueType)
			chapters = append(chapters, &InkrChapter{
				Chapter: Chapter{
					Number: d.Order,
					Title:  d.Name,
					ID:     oid,
					Locked: revenue != "ad" && revenue != "free",
					Label:  Label{Style: LabelNumbered},
				},
				OID: oid,
			})
		}
	}
	return chapters, nil
}

// FetchChapter returns the chapter's encrypted pictures, each decrypted by
// its Transform
func (i *Inkr) FetchChapter(f Filterable) (*Chapter, error) {
	ichap := f.(*InkrChapter)
	pages := map[string]struct {
		Pages []struct {
			URL string `json:"url"`
		} `json:"chapterPages"`
	}{}
	query := map[string]any{
		"fields": []string{"chapterPages"},
		"oids":   []string{ichap.OID},
		"includes": map[string]any{"chapterPages": map[string]any{
			"fields": []string{"oid", "width", "height", "type"}, "includes": map[string]any{}, "includeKey": "page",
		}},
	}
	if err := i.batch(query, &pages); err != nil {
		return nil, err
	}
	list := pages[ichap.OID].Pages
	if len(list) == 0 {
		return nil, fmt.Errorf("inkr: no pages for %s (a paid chapter?)", ichap.OID)
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   "en",
		ID:         ichap.ID,
		PagesCount: int64(len(list)),
	}
	for n, p := range list {
		chapter.Pages = append(chapter.Pages, Page{
			Number:    int64(n + 1),
			URL:       strings.TrimSuffix(p.URL, "/") + inkrImageVariant,
			Transform: inkrDecrypt,
		})
	}
	return chapter, nil
}

// inkrDecrypt decrypts an .ikc picture: a little-endian uint32 plaintext
// size, a 16-byte IV, then AES-256-CBC without padding
func inkrDecrypt(data []byte) ([]byte, error) {
	if len(data) < 20 || (len(data)-20)%aes.BlockSize != 0 {
		return nil, errors.New("inkr: not an .ikc picture")
	}
	size := binary.LittleEndian.Uint32(data[:4])
	block, err := aes.NewCipher(inkrKey)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data)-20)
	cipher.NewCBCDecrypter(block, data[4:20]).CryptBlocks(out, data[20:])
	if int(size) > len(out) {
		return nil, errors.New("inkr: truncated picture")
	}
	return out[:size], nil
}
