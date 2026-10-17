// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// e621UserAgent is what the API is asked with: e621 asks scripts for a
// descriptive User-Agent and turns some generic ones away with a 403
const e621UserAgent = "manga-downloader (https://github.com/elboletaire/manga-downloader)"

// e621BatchSize is how many posts one posts.json request asks for (the API
// caps limit at 320)
const e621BatchSize = 200

var (
	e621HostRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?(?:e621|e926)\.net(?:[/?#]|$)`)
	e621PoolRe = regexp.MustCompile(`^/pools/(\d+)`)
)

// E621 is a grabber for e621.net and its filtered mirror e926.net, image
// boards whose pools group posts into comics. A pool is a series with one
// chapter whose pages are its posts, in the pool's order; posts the API
// shows no file for (deleted, or hidden from anonymous visitors) and videos
// are left out.
type E621 struct {
	*Grabber
	pool *e621Pool
}

func NewE621(g *Grabber) *E621 {
	return &E621{Grabber: g}
}

// E621Chapter is the pool, the only chapter of its series
type E621Chapter struct {
	Chapter
	PostIDs []int
}

// e621Pool is the /pools/{id}.json answer
type e621Pool struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	PostIDs []int  `json:"post_ids"`
}

// e621Post is a post of /posts.json
type e621Post struct {
	ID   int `json:"id"`
	File struct {
		Ext string  `json:"ext"`
		URL *string `json:"url"`
	} `json:"file"`
}

// Test returns true for e621.net and e926.net URLs
func (e *E621) Test() (bool, error) {
	return e621HostRe.MatchString(e.URL), nil
}

func (e *E621) getJSON(rawURL string, v any) error {
	text, err := http.GetText(http.RequestParams{URL: rawURL, Headers: map[string]string{"User-Agent": e621UserAgent}, Accept: "application/json"})
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(text), v)
}

// e621PoolID reads the pool id off a /pools/{id} URL
func e621PoolID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	m := e621PoolRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errors.New("not a pool URL (/pools/{id})")
	}

	return m[1], nil
}

func (e *E621) fetchPool() (*e621Pool, error) {
	if e.pool != nil {
		return e.pool, nil
	}
	id, err := e621PoolID(e.URL)
	if err != nil {
		return nil, err
	}
	pool := &e621Pool{}
	if err = e.getJSON(e.BaseUrl()+"/pools/"+id+".json", pool); err != nil {
		return nil, err
	}
	if pool.ID == 0 {
		return nil, fmt.Errorf("pool %s not found", id)
	}
	e.pool = pool

	return pool, nil
}

// FetchTitle returns the pool's name, its underscores spaced out
func (e *E621) FetchTitle() (string, error) {
	pool, err := e.fetchPool()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(strings.ReplaceAll(pool.Name, "_", " ")), nil
}

// FetchChapters returns the pool as the only chapter
func (e *E621) FetchChapters() (Filterables, []error) {
	pool, err := e.fetchPool()
	if err != nil {
		return nil, []error{err}
	}

	return Filterables{&E621Chapter{
		Chapter: Chapter{Number: 1, ID: strconv.Itoa(pool.ID), Label: Label{Style: LabelUnnumbered}},
		PostIDs: pool.PostIDs,
	}}, nil
}

// FetchChapter asks for the pool's posts in batches and lists their files
func (e *E621) FetchChapter(f Filterable) (*Chapter, error) {
	echap := f.(*E621Chapter)
	posts := map[int]e621Post{}
	for start := 0; start < len(echap.PostIDs); start += e621BatchSize {
		batch := echap.PostIDs[start:min(start+e621BatchSize, len(echap.PostIDs))]
		ids := make([]string, len(batch))
		for i, id := range batch {
			ids[i] = strconv.Itoa(id)
		}
		q := url.Values{"tags": {"status:any id:" + strings.Join(ids, ",")}, "limit": {strconv.Itoa(len(batch))}}
		list := struct {
			Posts []e621Post `json:"posts"`
		}{}
		if err := e.getJSON(e.BaseUrl()+"/posts.json?"+q.Encode(), &list); err != nil {
			return nil, err
		}
		for _, p := range list.Posts {
			posts[p.ID] = p
		}
	}

	chapter := &Chapter{Title: echap.GetTitle(), Number: echap.GetNumber(), ID: echap.ID, Label: labelOf(f)}
	for _, u := range e621Pages(echap.PostIDs, posts) {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(len(chapter.Pages) + 1), URL: u, Headers: map[string]string{"User-Agent": e621UserAgent}})
	}
	chapter.PagesCount = int64(len(chapter.Pages))
	if len(chapter.Pages) == 0 {
		return nil, errors.New("none of the pool's posts has a picture anonymous visitors can see")
	}

	return chapter, nil
}

// e621Pages returns the picture of each post, in the pool's order
func e621Pages(order []int, posts map[int]e621Post) (urls []string) {
	for _, id := range order {
		p, ok := posts[id]
		if !ok || p.File.URL == nil || *p.File.URL == "" {
			continue
		}
		switch strings.ToLower(p.File.Ext) {
		case "webm", "mp4", "swf":
			continue
		}
		urls = append(urls, *p.File.URL)
	}

	return urls
}
