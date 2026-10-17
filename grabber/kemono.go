// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

// Kemono is a grabber for the Kemono archive software (kemono.cr, coomer.st,
// pawchive.pw): a creator's page (/{service}/user/{id}) is the series and
// each of their posts with pictures a chapter. The JSON API lists the posts
// 50 at a time with their files, so the pages are known without a request
// per post
type Kemono struct {
	*Grabber
	title string
	posts []kemonoPost
}

// KemonoChapter is a post with pictures
type KemonoChapter struct {
	Chapter
	Files []string
}

// kemonoHost is how one Kemono instance serves its files
type kemonoHost struct {
	// files is the origin serving /data/{hash path}, "" for the site itself
	// (which redirects to its file servers)
	files string
	// userAgent replaces the browser one for the files: pawchive's file
	// server refuses browser-looking clients and asks for one that names
	// itself
	userAgent string
	// pacer spaces the file requests to the instance's stated limit
	pacer *pacer
}

// kemonoHosts are the known instances, by domain
var kemonoHosts = map[string]kemonoHost{
	"kemono.cr":    {pacer: &pacer{interval: time.Second / 4}},
	"kemono.su":    {pacer: &pacer{interval: time.Second / 4}},
	"kemono.party": {pacer: &pacer{interval: time.Second / 4}},
	"coomer.st":    {pacer: &pacer{interval: time.Second / 4}},
	"coomer.su":    {pacer: &pacer{interval: time.Second / 4}},
	"coomer.party": {pacer: &pacer{interval: time.Second / 4}},
	// pawchive asks for at most one request a second and 5 downloads at once
	"pawchive.pw": {
		files:     "https://file.pawchive.pw",
		userAgent: "manga-downloader (+https://github.com/elboletaire/manga-downloader)",
		pacer:     &pacer{interval: time.Second},
	},
}

// kemonoMaxConcurrency keeps within the instances' download limits
var kemonoMaxConcurrency = MaxConcurrency{Chapters: 1, Pages: 4}

// kemonoPageSize is how many posts the API lists per request
const kemonoPageSize = 50

// kemonoMaxListPages bounds the listing walk (25000 posts)
const kemonoMaxListPages = 500

// kemonoCreatorRe captures the service and creator id of a creator or post
// path (/fanbox/user/123, /onlyfans/user/name/post/456)
var kemonoCreatorRe = regexp.MustCompile(`^/([a-z0-9]+)/user/([^/]+)`)

// kemonoImageExts are the file extensions kept as pages; videos and archives
// are left out
var kemonoImageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".avif": true, ".bmp": true}

func NewKemono(g *Grabber) *Kemono {
	return &Kemono{Grabber: g}
}

// Test returns true for a creator or post URL of a known instance
func (k *Kemono) Test() (bool, error) {
	_, ok := k.host()
	if !ok {
		return false, nil
	}
	service, _ := k.creator()

	return service != "", nil
}

// host returns the URL's instance
func (k Kemono) host() (kemonoHost, bool) {
	for domain, host := range kemonoHosts {
		if hostMatchesDomain(k.URL, domain) {
			return host, true
		}
	}
	return kemonoHost{}, false
}

// creator returns the service and creator id of the URL, both "" when it
// names no creator
func (k Kemono) creator() (service, id string) {
	u, err := url.Parse(k.URL)
	if err != nil {
		return "", ""
	}
	matches := kemonoCreatorRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return "", ""
	}
	return matches[1], matches[2]
}

// apiURL returns the creator's API URL with the given suffix
func (k Kemono) apiURL(suffix string) string {
	service, id := k.creator()
	return fmt.Sprintf("%s/api/v1/%s/user/%s%s", k.BaseUrl(), service, url.PathEscape(id), suffix)
}

// getJSON fetches an API URL and decodes it into v. The API answers
// "text/css" only: coomer refuses the default Accept. kemono.cr gzips its
// answers whether asked or not, and the shared transport doesn't decompress.
func (k Kemono) getJSON(uri string, v any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: k.URL,
		Accept:  "text/css",
	})
	if err != nil {
		return err
	}
	data := []byte(body)
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if data, err = io.ReadAll(zr); err != nil {
			return err
		}
	}
	return json.Unmarshal(data, v)
}

// FetchTitle returns the creator's name
func (k *Kemono) FetchTitle() (string, error) {
	if k.title != "" {
		return k.title, nil
	}
	profile := struct {
		Name string `json:"name"`
	}{}
	if err := k.getJSON(k.apiURL("/profile"), &profile); err != nil {
		return "", err
	}
	k.title = sanitizeTitle(profile.Name)

	return k.title, nil
}

// kemonoPost is a post as the API lists it
type kemonoPost struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	File        kemonoFile   `json:"file"`
	Attachments []kemonoFile `json:"attachments"`
}

// kemonoFile is a post's file or attachment
type kemonoFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// images returns the paths of the post's pictures, the main file first and
// each once (the main file is often an attachment too)
func (p kemonoPost) images() []string {
	var paths []string
	seen := map[string]bool{}
	for _, f := range append([]kemonoFile{p.File}, p.Attachments...) {
		if f.Path == "" || seen[f.Path] || !kemonoImageExts[strings.ToLower(path.Ext(f.Path))] {
			continue
		}
		seen[f.Path] = true
		paths = append(paths, f.Path)
	}
	return paths
}

// listPosts walks the creator's posts, newest first, once
func (k *Kemono) listPosts() ([]kemonoPost, error) {
	if k.posts != nil {
		return k.posts, nil
	}
	posts := []kemonoPost{}
	for page := 0; page < kemonoMaxListPages; page++ {
		var batch []kemonoPost
		if err := k.getJSON(k.apiURL(fmt.Sprintf("/posts?o=%d", page*kemonoPageSize)), &batch); err != nil {
			return nil, err
		}
		posts = append(posts, batch...)
		if len(batch) < kemonoPageSize {
			break
		}
	}
	k.posts = posts

	return posts, nil
}

// FetchChapters returns the creator's posts that have pictures, numbered from
// the oldest, so a new post doesn't renumber the others
func (k *Kemono) FetchChapters() (Filterables, []error) {
	posts, err := k.listPosts()
	if err != nil {
		return nil, []error{err}
	}

	chapters := Filterables{}
	for i := len(posts) - 1; i >= 0; i-- {
		images := posts[i].images()
		if len(images) == 0 {
			continue
		}
		chapters = append(chapters, &KemonoChapter{
			Chapter: Chapter{
				Number: float64(len(chapters) + 1),
				Title:  posts[i].Title,
				ID:     posts[i].ID,
				Label:  Label{Style: LabelNumbered},
			},
			Files: images,
		})
	}

	return chapters, nil
}

// FetchChapter returns the post's pictures, already listed with it
func (k Kemono) FetchChapter(f Filterable) (*Chapter, error) {
	kchap := f.(*KemonoChapter)
	host, _ := k.host()
	files := host.files
	if files == "" {
		files = k.BaseUrl()
	}
	var headers map[string]string
	if host.userAgent != "" {
		headers = map[string]string{"User-Agent": host.userAgent}
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(kchap.Files)),
		ID:         kchap.ID,
	}
	for i, p := range kchap.Files {
		chapter.Pages = append(chapter.Pages, Page{
			Number:  int64(i + 1),
			URL:     files + "/data" + p,
			Headers: headers,
		})
	}

	return chapter, nil
}

// GetMaxConcurrency caps the configured concurrency at the instances' limits
func (k *Kemono) GetMaxConcurrency() MaxConcurrency {
	mc := k.Grabber.GetMaxConcurrency()
	mc.Chapters = min(mc.Chapters, kemonoMaxConcurrency.Chapters)
	mc.Pages = min(mc.Pages, kemonoMaxConcurrency.Pages)
	return mc
}

// WaitPage paces file downloads to the instance's limit (PagePacer)
func (k *Kemono) WaitPage() {
	if host, ok := k.host(); ok && host.pacer != nil {
		host.pacer.wait()
	}
}
