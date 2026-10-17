// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Mangadraft is a grabber for mangadraft.com: the series page states its
// project in `window.project`, the reader API lists the project's chapters
// (categories, with their price) and, from any page id, every page of the
// project with its category
type Mangadraft struct {
	*Grabber
	project *mangadraftProject
}

// MangadraftChapter is a category of the project
type MangadraftChapter struct {
	Chapter
	CategoryID int64
}

// mangadraftProject is the part of `window.project` used
type mangadraftProject struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// mangadraftCategory is a chapter (or, for a one-shot, the root category)
type mangadraftCategory struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Order  int    `json:"order"`
	IsFree bool   `json:"is_free"`
	Pages  int    `json:"nb_pages"`
}

var (
	// mangadraftProjectRe and mangadraftFirstPageRe find the page's JSON
	// objects (decoded from there, as they nest)
	mangadraftProjectRe   = regexp.MustCompile(`window\.project\s*=\s*`)
	mangadraftFirstPageRe = regexp.MustCompile(`window\.firstPage\s*=\s*`)
)

func NewMangadraft(g *Grabber) *Mangadraft {
	return &Mangadraft{Grabber: g}
}

// Test returns true for mangadraft.com URLs
func (m *Mangadraft) Test() (bool, error) {
	return hostMatchesDomain(m.URL, "mangadraft.com"), nil
}

// jsObject decodes the JSON object that follows re's match in page into v
func jsObject(page string, re *regexp.Regexp, v any) error {
	loc := re.FindStringIndex(page)
	if loc == nil {
		return fmt.Errorf("no %s in the page", re.String())
	}
	return json.NewDecoder(strings.NewReader(page[loc[1]:])).Decode(v)
}

// fetchProject reads `window.project` off the URL's page once
func (m *Mangadraft) fetchProject() (*mangadraftProject, error) {
	if m.project != nil {
		return m.project, nil
	}
	page, err := http.GetText(http.RequestParams{URL: m.URL})
	if err != nil {
		return nil, err
	}
	project := &mangadraftProject{}
	if err := jsObject(page, mangadraftProjectRe, project); err != nil {
		return nil, err
	}
	if project.ID == 0 {
		return nil, errors.New("mangadraft: the page names no project")
	}
	m.project = project
	return project, nil
}

// FetchTitle returns the project's name
func (m *Mangadraft) FetchTitle() (string, error) {
	project, err := m.fetchProject()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(project.Name), nil
}

// FetchChapters returns the project's chapters in order, the paid ones
// locked; a one-shot (no chapters) is its root category
func (m *Mangadraft) FetchChapters() (Filterables, []error) {
	project, err := m.fetchProject()
	if err != nil {
		return nil, []error{err}
	}
	body, err := http.GetText(http.RequestParams{
		URL:     fmt.Sprintf("%s/api/reader/%d/categories?number=200", m.BaseUrl(), project.ID),
		Referer: m.URL,
		Accept:  "application/json",
	})
	if err != nil {
		return nil, []error{err}
	}
	var feed struct {
		// root category id → its chapters; an empty list when there are none
		Chapter json.RawMessage      `json:"CHAPTER"`
		Root    []mangadraftCategory `json:"ROOT"`
	}
	if err := json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, []error{err}
	}
	byRoot := map[string][]mangadraftCategory{}
	if len(feed.Chapter) > 0 && feed.Chapter[0] == '{' {
		if err := json.Unmarshal(feed.Chapter, &byRoot); err != nil {
			return nil, []error{err}
		}
	}

	sort.SliceStable(feed.Root, func(i, j int) bool { return feed.Root[i].Order < feed.Root[j].Order })
	var categories []mangadraftCategory
	for _, root := range feed.Root {
		children := byRoot[fmt.Sprint(root.ID)]
		sort.SliceStable(children, func(i, j int) bool { return children[i].Order < children[j].Order })
		if len(children) == 0 && root.Pages > 0 {
			// a one-shot's pages sit in the root itself, which is free
			root.IsFree = true
			children = []mangadraftCategory{root}
		}
		categories = append(categories, children...)
	}

	chapters := make(Filterables, 0, len(categories))
	for _, c := range categories {
		chapters = append(chapters, &MangadraftChapter{
			Chapter: Chapter{
				Number: float64(len(chapters) + 1),
				Title:  c.Name,
				ID:     fmt.Sprint(c.ID),
				Locked: !c.IsFree,
				Label:  Label{Style: LabelNumbered, Lang: "fr"},
			},
			CategoryID: c.ID,
		})
	}

	return chapters, nil
}

// FetchChapter finds the chapter's first page in its reader, then lists the
// project's pages from there and keeps the chapter's own
func (m *Mangadraft) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangadraftChapter)
	project, err := m.fetchProject()
	if err != nil {
		return nil, err
	}
	reader := fmt.Sprintf("%s/reader/%s/c.%d", m.BaseUrl(), project.Slug, mchap.CategoryID)
	page, err := http.GetText(http.RequestParams{URL: reader, Referer: m.URL})
	if err != nil {
		return nil, err
	}
	var first struct {
		ID int64 `json:"id"`
	}
	if err := jsObject(page, mangadraftFirstPageRe, &first); err != nil {
		return nil, err
	}

	body, err := http.GetText(http.RequestParams{
		URL:     fmt.Sprintf("%s/api/reader/listPages?first_page=%d", m.BaseUrl(), first.ID),
		Referer: reader,
		Accept:  "application/json",
	})
	if err != nil {
		return nil, err
	}
	var list struct {
		Data []struct {
			URL      string `json:"url"`
			Category int64  `json:"cat"`
			Type     string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "fr",
		ID:       mchap.ID,
	}
	for _, p := range list.Data {
		if p.Category != mchap.CategoryID || p.URL == "" {
			continue
		}
		// without the size the image URL answers 404
		sep := "?"
		if strings.Contains(p.URL, "?") {
			sep = "&"
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    p.URL + sep + "size=full",
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
