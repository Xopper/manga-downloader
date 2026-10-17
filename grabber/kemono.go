// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

var (
	// kemonoHostRe matches kemono on the domains it has used (its sister
	// coomer runs the same API but archives photo and video sites, not comics)
	kemonoHostRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?kemono\.(?:cr|su|party)(?:[/?#]|$)`)
	// kemonoCreatorRe matches a creator's path, or one of their posts
	kemonoCreatorRe = regexp.MustCompile(`^/([a-z0-9_-]+)/user/([^/?#]+)(?:/post/[^/?#]+)?/?$`)
	// kemonoImageExtRe matches the attachments that are pictures
	kemonoImageExtRe = regexp.MustCompile(`(?i)\.(?:jpe?g|png|gif|webp)$`)
)

const (
	// kemonoAPIAccept is what the API demands: it answers anything else
	// (JSON included) with a 403 telling scrapers to send `Accept: text/css`
	kemonoAPIAccept = "text/css"
	// kemonoPageSize is the posts one listing page holds (fixed by the API)
	kemonoPageSize = 50
	// kemonoMaxListPages bounds the walk over a creator's posts
	kemonoMaxListPages = 400
	// kemonoListPause spaces the listing's requests: the API rate-limits hard
	kemonoListPause = 500 * time.Millisecond
	// kemonoProbeTimeout bounds the check that the full-size file servers
	// answer at all
	kemonoProbeTimeout = 15 * time.Second
)

// Kemono is a grabber for kemono.cr, an archive of creators'
// paywalled posts (Patreon, Fanbox, Fantia…). A creator is a series and each
// of their posts with pictures a chapter, numbered oldest first; posts with
// only text, video or archives are left out. The JSON API lists 50 posts per
// request with their main file and attachments, which is all a chapter needs.
//
// Full-size files (`/data/…`) redirect to numbered file servers (n1…n4) that
// some networks can't reach at all (the connection just hangs), while the
// thumbnail server (`img.` host, `/thumbnail/data/…`, ~800px tall) works, so
// FetchChapter checks once whether a full-size file answers and falls back
// to thumbnails when it doesn't, as Tachiyomi's extension does.
type Kemono struct {
	*Grabber
	profile *kemonoProfile
	// probeOnce guards fullOK, whether the full-size file servers answered
	probeOnce sync.Once
	fullOK    bool
}

// NewKemono returns a kemono grabber
func NewKemono(g *Grabber) *Kemono {
	return &Kemono{Grabber: g}
}

// KemonoChapter is a post, keeping its picture paths for FetchChapter
type KemonoChapter struct {
	Chapter
	Paths []string
}

// kemonoProfile is a creator as /profile returns it
type kemonoProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Service   string `json:"service"`
	PostCount int    `json:"post_count"`
}

// kemonoFile is a post's main file or one of its attachments
type kemonoFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// kemonoPost is a post as the listing returns it
type kemonoPost struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Published   string       `json:"published"`
	File        kemonoFile   `json:"file"`
	Attachments []kemonoFile `json:"attachments"`
}

// Test matches kemono URLs by domain
func (k *Kemono) Test() (bool, error) {
	return kemonoHostRe.MatchString(k.URL), nil
}

// creator returns the service and creator id the URL names, a creator's page
// or one of their posts
func (k *Kemono) creator() (service, id string, err error) {
	u, err := url.Parse(k.URL)
	if err != nil {
		return "", "", err
	}
	m := kemonoCreatorRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", errSeriesNotFound(k.URL)
	}
	return m[1], m[2], nil
}

// get fetches an API path and decodes its JSON. Some answers come gzipped
// although the request never offers gzip (the shared client doesn't, so it
// doesn't unpack them either), hence the check for gzip's magic bytes.
func (k *Kemono) get(path string, v any) error {
	text, err := http.GetText(http.RequestParams{URL: k.BaseUrl() + "/api/v1" + path, Accept: kemonoAPIAccept})
	if err != nil {
		return err
	}
	data, err := kemonoGunzip([]byte(text))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// kemonoGunzip unpacks a gzipped body, passing anything else through
func kemonoGunzip(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// fetchProfile fetches the creator once
func (k *Kemono) fetchProfile() (*kemonoProfile, error) {
	if k.profile != nil {
		return k.profile, nil
	}
	service, id, err := k.creator()
	if err != nil {
		return nil, err
	}
	p := &kemonoProfile{}
	if err := k.get("/"+service+"/user/"+url.PathEscape(id)+"/profile", p); err != nil {
		return nil, err
	}
	if p.Service == "" {
		p.Service = service
	}
	if p.ID == "" {
		p.ID = id
	}
	k.profile = p
	return p, nil
}

// FetchTitle returns the creator's name
func (k *Kemono) FetchTitle() (string, error) {
	p, err := k.fetchProfile()
	if err != nil {
		return "", err
	}
	return sanitizeTitle(p.Name), nil
}

// FetchChapters lists every post that has pictures, oldest first
func (k *Kemono) FetchChapters() (Filterables, []error) {
	p, err := k.fetchProfile()
	if err != nil {
		return nil, []error{err}
	}
	var posts []kemonoPost
	for page := range kemonoMaxListPages {
		if page > 0 {
			time.Sleep(kemonoListPause)
		}
		var batch []kemonoPost
		path := fmt.Sprintf("/%s/user/%s/posts?o=%d", p.Service, url.PathEscape(p.ID), page*kemonoPageSize)
		if err := k.get(path, &batch); err != nil {
			if len(posts) == 0 {
				return nil, []error{err}
			}
			return kemonoChapters(posts), []error{err}
		}
		posts = append(posts, batch...)
		if len(batch) < kemonoPageSize || (p.PostCount > 0 && len(posts) >= p.PostCount) {
			break
		}
	}
	chapters := kemonoChapters(posts)
	if len(chapters) == 0 {
		return nil, []error{fmt.Errorf("%s has no posts with pictures", p.Name)}
	}
	return chapters, nil
}

// kemonoChapters turns the listing (newest first) into chapters numbered
// oldest first, keeping only posts with pictures
func kemonoChapters(posts []kemonoPost) Filterables {
	sort.SliceStable(posts, func(i, j int) bool { return posts[i].Published < posts[j].Published })
	var chapters Filterables
	seen := map[string]bool{}
	for _, post := range posts {
		if seen[post.ID] {
			continue
		}
		seen[post.ID] = true
		paths := kemonoImagePaths(post)
		if len(paths) == 0 {
			continue
		}
		chapters = append(chapters, &KemonoChapter{
			Chapter: Chapter{
				Number: float64(len(chapters) + 1),
				Title:  sanitizeTitle(post.Title),
				ID:     post.ID,
				Label:  Label{Style: LabelNumbered},
			},
			Paths: paths,
		})
	}
	return chapters
}

// kemonoImagePaths returns a post's pictures: its main file, then its
// attachments, once each (the main file is usually repeated among them)
func kemonoImagePaths(post kemonoPost) []string {
	var paths []string
	seen := map[string]bool{}
	for _, f := range append([]kemonoFile{post.File}, post.Attachments...) {
		if f.Path == "" || seen[f.Path] || !kemonoImageExtRe.MatchString(f.Path) {
			continue
		}
		seen[f.Path] = true
		paths = append(paths, f.Path)
	}
	return paths
}

// FetchChapter builds the post's image URLs, full size when the file
// servers answer
func (k *Kemono) FetchChapter(f Filterable) (*Chapter, error) {
	kc, ok := f.(*KemonoChapter)
	if !ok || len(kc.Paths) == 0 {
		return nil, fmt.Errorf("no pictures in post %s", f.GetTitle())
	}
	full := k.fullSizeReachable(k.fullURL(kc.Paths[0]))
	chapter := &Chapter{
		Title:      kc.Title,
		Number:     kc.Number,
		Label:      labelOf(f),
		ID:         kc.ID,
		PagesCount: int64(len(kc.Paths)),
	}
	for i, p := range kc.Paths {
		u := k.fullURL(p)
		if !full {
			u = k.thumbnailURL(p)
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	return chapter, nil
}

// fullURL is a file's full-size URL
func (k *Kemono) fullURL(path string) string {
	return k.BaseUrl() + "/data" + path
}

// thumbnailURL is a file's copy on the thumbnail server
func (k *Kemono) thumbnailURL(path string) string {
	u, err := url.Parse(k.BaseUrl())
	if err != nil {
		return k.BaseUrl() + "/thumbnail/data" + path
	}
	host := strings.TrimPrefix(u.Host, "www.")
	return u.Scheme + "://img." + host + "/thumbnail/data" + path
}

// fullSizeReachable checks once whether a full-size file downloads: a HEAD
// that follows the redirect to its file server
func (k *Kemono) fullSizeReachable(sample string) bool {
	k.probeOnce.Do(func() {
		client := &stdhttp.Client{Timeout: kemonoProbeTimeout}
		req, err := stdhttp.NewRequest(stdhttp.MethodHead, sample, nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		resp.Body.Close()
		k.fullOK = resp.StatusCode == stdhttp.StatusOK
	})
	return k.fullOK
}

// GetMaxConcurrency caps the downloads: the file servers rate-limit
func (k *Kemono) GetMaxConcurrency() MaxConcurrency {
	m := k.Grabber.GetMaxConcurrency()
	m.Chapters = min(m.Chapters, 2)
	m.Pages = min(m.Pages, 4)
	return m
}
