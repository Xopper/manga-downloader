// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

const (
	// warForRayubaRaw is where the WarMap repo keeps one JSON per round
	warForRayubaRaw = "https://raw.githubusercontent.com/xrabohrok/WarMap/master/tools/"
	// warForRayubaCubari serves a chapter's imgur album as a page list
	warForRayubaCubari = "https://cubari.moe"
	// warForRayubaMaxRounds bounds the walk over master-list-N.json
	warForRayubaMaxRounds = 50
)

// warForRayubaRoundRe reads the round of a master list file name
var warForRayubaRoundRe = regexp.MustCompile(`master-list-(\d+)\.json$`)

// warForRayubaFileRe matches a round file on GitHub (raw or blob view)
var warForRayubaFileRe = regexp.MustCompile(`(?i)^/xrabohrok/warmap/(?:blob/)?[^/]+/tools/([^/]+\.json)$`)

// WarForRayuba is a grabber for War For Rayuba, a collaborative webcomic
// tournament whose map lives at xrabohrok.github.io/WarMap. Each round is a
// cubari-style JSON (master-list-N.json) in the map's GitHub repo, whose
// chapters point at imgur albums read through cubari's proxy. The map URL
// loads every round as one series, a round per volume; a round's JSON URL
// loads that round alone.
type WarForRayuba struct {
	*Grabber
	title    string
	chapters Filterables
	loaded   bool
}

// WarForRayubaChapter is a chapter of a round: Cubari is its page list path
type WarForRayubaChapter struct {
	Chapter
	Cubari string
}

// warForRayubaRound is a round's JSON (cubari's series format)
type warForRayubaRound struct {
	Title    string `json:"title"`
	Chapters map[string]struct {
		Title  string `json:"title"`
		Groups struct {
			Primary string `json:"primary"`
		} `json:"groups"`
	} `json:"chapters"`
}

// NewWarForRayuba returns a new WarForRayuba grabber
func NewWarForRayuba(g *Grabber) *WarForRayuba {
	return &WarForRayuba{Grabber: g}
}

// Test matches the map and the repo's round files
func (w *WarForRayuba) Test() (bool, error) {
	if w.isMap() {
		return true, nil
	}
	_, ok := w.roundFile()
	return ok, nil
}

// isMap reports whether the URL is the WarMap site
func (w *WarForRayuba) isMap() bool {
	host, path, ok := chapterPath(w.URL)
	return ok && host == "xrabohrok.github.io" && strings.HasPrefix(strings.ToLower(path), "/warmap")
}

// roundFile returns the round file a GitHub URL points at
func (w *WarForRayuba) roundFile() (string, bool) {
	host, path, ok := chapterPath(w.URL)
	if !ok || (host != "raw.githubusercontent.com" && host != "github.com") {
		return "", false
	}
	m := warForRayubaFileRe.FindStringSubmatch(path)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// load reads the rounds: every master-list-N.json for the map (until one is
// missing), or the one file pasted
func (w *WarForRayuba) load() error {
	if w.loaded {
		return nil
	}
	if file, ok := w.roundFile(); ok {
		round, err := warForRayubaFetch(file, false)
		if err != nil {
			return err
		}
		vol := 0
		if m := warForRayubaRoundRe.FindStringSubmatch(file); m != nil {
			vol, _ = strconv.Atoi(m[1])
		}
		w.title = sanitizeTitle(round.Title)
		w.chapters = warForRayubaChapters(round, vol, 0, false)
		w.loaded = true
		return nil
	}
	if !w.isMap() {
		return errSeriesNotFound(w.URL)
	}
	for n := 1; n <= warForRayubaMaxRounds; n++ {
		round, err := warForRayubaFetch(fmt.Sprintf("master-list-%d.json", n), true)
		var se *http.StatusError
		if n > 1 && errors.As(err, &se) && se.Code == 404 {
			break
		}
		if err != nil {
			return err
		}
		w.chapters = append(w.chapters, warForRayubaChapters(round, n, len(w.chapters), true)...)
	}
	w.title = "War For Rayuba"
	w.loaded = true
	return nil
}

// warForRayubaFetch reads a round file of the repo
func warForRayubaFetch(file string, disableRetry bool) (*warForRayubaRound, error) {
	body, err := http.Get(http.RequestParams{URL: warForRayubaRaw + url.PathEscape(file), DisableRetry: disableRetry})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return warForRayubaParse(body)
}

// warForRayubaParse decodes a round file
func warForRayubaParse(r io.Reader) (*warForRayubaRound, error) {
	round := &warForRayubaRound{}
	if err := json.NewDecoder(r).Decode(round); err != nil {
		return nil, err
	}
	return round, nil
}

// warForRayubaChapters lists a round's chapters in their order, the round
// as their volume. Alone they keep the round's own numbers; chained, they're
// numbered on after the offset chapters of the previous rounds, since every
// round starts again at 1.
func warForRayubaChapters(round *warForRayubaRound, vol, offset int, chained bool) Filterables {
	keys := make([]string, 0, len(round.Chapters))
	for k := range round.Chapters {
		if _, err := strconv.ParseFloat(k, 64); err == nil {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, _ := strconv.ParseFloat(keys[i], 64)
		b, _ := strconv.ParseFloat(keys[j], 64)
		return a < b
	})
	var chapters Filterables
	for i, k := range keys {
		c := round.Chapters[k]
		if c.Groups.Primary == "" {
			continue
		}
		num, _ := strconv.ParseFloat(k, 64)
		label := Label{Style: LabelPrefixed, Number: k}
		if chained {
			num = float64(offset + i + 1)
			label.Number = ""
		}
		chapters = append(chapters, &WarForRayubaChapter{
			Chapter{
				Number:   num,
				Title:    sanitizeTitle(c.Title),
				Language: "en",
				Volume:   vol,
				ID:       c.Groups.Primary,
				Label:    label,
			},
			c.Groups.Primary,
		})
	}
	return chapters
}

// FetchTitle returns "War For Rayuba", or the round's title
func (w *WarForRayuba) FetchTitle() (string, error) {
	if err := w.load(); err != nil {
		return "", err
	}
	return w.title, nil
}

// FetchChapters returns the rounds' chapters
func (w *WarForRayuba) FetchChapters() (Filterables, []error) {
	if err := w.load(); err != nil {
		return nil, []error{err}
	}
	return w.chapters, nil
}

// FetchChapter reads the album through cubari (its /proxy path redirects to
// /read, which answers a JSON list of the images)
func (w *WarForRayuba) FetchChapter(f Filterable) (*Chapter, error) {
	wchap := f.(*WarForRayubaChapter)
	body, err := http.Get(http.RequestParams{URL: warForRayubaCubari + wchap.Cubari, Accept: "application/json"})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	pages, err := warForRayubaPages(body)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		Volume:     wchap.Volume,
		ID:         wchap.ID,
		Pages:      pages,
	}, nil
}

// warForRayubaPages decodes cubari's page list
func warForRayubaPages(r io.Reader) ([]Page, error) {
	var rows []struct {
		Src string `json:"src"`
	}
	if err := json.NewDecoder(r).Decode(&rows); err != nil {
		return nil, err
	}
	pages := make([]Page, 0, len(rows))
	for _, row := range rows {
		if row.Src != "" {
			pages = append(pages, Page{Number: int64(len(pages) + 1), URL: row.Src})
		}
	}
	return pages, nil
}
