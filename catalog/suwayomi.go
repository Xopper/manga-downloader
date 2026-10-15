// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	nethttp "net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SuwayomiOptions drive CrawlSuwayomi
type SuwayomiOptions struct {
	// Server is the Suwayomi-Server's URL (http://127.0.0.1:4567)
	Server string
	// Langs are the source languages to crawl; "all" (multi-language
	// sources) is always included
	Langs []string
	// Out is the catalog dir the files are merged into
	Out string
	// MaxPages caps every listing walk (popular, latest, search); 300 when 0
	MaxPages int
	// Parallel is how many sources are walked at once; 4 when 0
	Parallel int
	// Shard/Shards keep every source whose index (in id order) is Shard
	// modulo Shards; 0/0 keeps all
	Shard, Shards int
	// Sources keeps only these sources (by name or id); empty keeps all
	Sources []string
	// Install installs the extensions of the languages that aren't yet
	Install bool
	// Timeout stops starting new sources once it has elapsed; 0 is none
	Timeout time.Duration
	// Pause is the wait between pages; 200ms when 0, set to a negative value
	// for none
	Pause time.Duration
	// RequestTimeout bounds every request to the server (a page, an
	// install), so an extension stuck on a site can't hold a slot forever;
	// 120s when 0
	RequestTimeout time.Duration
	// MaxFetch caps, per source and run, the titles whose real link is
	// fetched one by one (sources whose link isn't base + path); the rest
	// wait for the next run. 200 when 0.
	MaxFetch int
}

// SourceRow is one crawled source in sources.json
type SourceRow struct {
	Site     string `json:"site"`
	Name     string `json:"name"`
	Lang     string `json:"lang"`
	NSFW     bool   `json:"nsfw"`
	SourceID string `json:"sourceId"`
	Entries  int    `json:"entries"`
	Complete bool   `json:"complete"`
	Failed   bool   `json:"failed,omitempty"`
	// Links says how the series links were made: "joined" (base + path,
	// checked against one title's real link), "fetched" (every link fetched
	// from the site, since the join was wrong) or "unchecked" (the check
	// itself failed; the joins stand)
	Links string `json:"links,omitempty"`
	// Fetched counts the links fetched this run
	Fetched int `json:"fetched,omitempty"`
	// Downloadable is whether the site is in the site table: the downloader
	// has a grabber for it
	Downloadable bool `json:"downloadable"`
}

// SuwayomiReport is what a crawl did
type SuwayomiReport struct {
	Sources []SourceRow
	// Written lists the sites whose file was written
	Written []string
	// Failed counts the sources that gave nothing, or were skipped by the
	// timeout
	Failed int
}

// suwayomi is the slice of the server's REST API the crawl uses. It has its
// own client with a deadline: the project's http package has none, and a
// request to Suwayomi can hang as long as the site behind the extension does.
type suwayomi struct {
	base   string
	client *nethttp.Client
}

// suwayomiUserAgent names the crawl to the server (which talks to the sites
// with the extensions' own agents)
const suwayomiUserAgent = "manga-downloader catalog crawl"

type suwayomiExtension struct {
	PkgName   string `json:"pkgName"`
	Lang      string `json:"lang"`
	Installed bool   `json:"installed"`
}

type suwayomiSource struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Lang           string `json:"lang"`
	BaseURL        string `json:"baseUrl"`
	SupportsLatest bool   `json:"supportsLatest"`
	NSFW           bool   `json:"isNsfw"`
}

type suwayomiPageResp struct {
	MangaList []struct {
		ID    int    `json:"id"`
		URL   string `json:"url"`
		Title string `json:"title"`
	} `json:"mangaList"`
	HasNextPage bool `json:"hasNextPage"`
}

// suwayomiManga is a manga's details; RealURL is the series page the
// extension computes, filled by an online fetch
type suwayomiManga struct {
	ID      int    `json:"id"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	RealURL string `json:"realUrl"`
}

// walked is a listed manga: its Suwayomi id, title and joined link
type walked struct {
	id    int
	title string
	url   string
}

func (s suwayomi) get(ctx context.Context, path string, into any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, s.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", suwayomiUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != nethttp.StatusOK {
		return fmt.Errorf("received %d response code", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}

	return json.Unmarshal(body, into)
}

func (s suwayomi) extensions(ctx context.Context) ([]suwayomiExtension, error) {
	var out []suwayomiExtension
	err := s.get(ctx, "/api/v1/extension/list", &out)

	return out, err
}

func (s suwayomi) install(ctx context.Context, pkg string) error {
	var ignored any
	err := s.get(ctx, "/api/v1/extension/install/"+url.PathEscape(pkg), &ignored)
	if err != nil && strings.Contains(err.Error(), "invalid character") {
		// the install answers plain text; that's fine
		return nil
	}

	return err
}

func (s suwayomi) sources(ctx context.Context) ([]suwayomiSource, error) {
	var out []suwayomiSource
	err := s.get(ctx, "/api/v1/source/list", &out)

	return out, err
}

// details fetches a manga's details from the site (through the extension),
// which is what fills its real URL
func (s suwayomi) details(ctx context.Context, id int) (*suwayomiManga, error) {
	var out suwayomiManga
	if err := s.get(ctx, fmt.Sprintf("/api/v1/manga/%d?onlineFetch=true", id), &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (s suwayomi) page(ctx context.Context, id, listing string, n int) (*suwayomiPageResp, error) {
	var path string
	switch listing {
	case "search":
		path = fmt.Sprintf("/api/v1/source/%s/search?searchTerm=&pageNum=%d", id, n)
	default:
		path = fmt.Sprintf("/api/v1/source/%s/%s/%d", id, listing, n)
	}
	var out suwayomiPageResp
	if err := s.get(ctx, path, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// wantLang reports whether a source or extension language is crawled
func wantLang(langs []string, lang string) bool {
	if lang == "all" {
		return true
	}
	for _, l := range langs {
		if l == lang {
			return true
		}
	}

	return false
}

// siteOfBase is the catalog site key of a source's base URL: its host
// without www. "" when there's no usable host.
func siteOfBase(base string) string {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if !siteNameRe.MatchString(host) || strings.Contains(host, "..") {
		return ""
	}

	return host
}

// CrawlSuwayomi lists every series of the server's sources in the
// languages wanted and merges them into the catalog files of Out, one per
// site, writing a line per source to w and the sources' rows to
// sources.json (sources-<shard>.json with shards). The error is only for a
// bad option or a server that doesn't answer; a source failing is a row.
func CrawlSuwayomi(ctx context.Context, w io.Writer, o SuwayomiOptions) (*SuwayomiReport, error) {
	if o.Server == "" {
		return nil, fmt.Errorf("no server URL")
	}
	if o.Shards < 0 || o.Shard < 0 || (o.Shards > 0 && o.Shard >= o.Shards) {
		return nil, fmt.Errorf("bad shard %d/%d", o.Shard, o.Shards)
	}
	if o.MaxPages <= 0 {
		o.MaxPages = 300
	}
	if o.Parallel <= 0 {
		o.Parallel = 4
	}
	switch {
	case o.Pause == 0:
		o.Pause = 200 * time.Millisecond
	case o.Pause < 0:
		o.Pause = 0
	}
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = 120 * time.Second
	}
	if o.MaxFetch <= 0 {
		o.MaxFetch = 200
	}
	langs := o.Langs
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	api := suwayomi{base: strings.TrimSuffix(o.Server, "/"), client: &nethttp.Client{Timeout: o.RequestTimeout}}
	started := time.Now()
	// one writer for the log lines: the sources run in parallel
	w = &lockedWriter{w: w}

	if o.Install {
		exts, err := api.extensions(ctx)
		if err != nil {
			return nil, fmt.Errorf("extension list: %w", err)
		}
		for _, e := range exts {
			if e.Installed || !wantLang(langs, e.Lang) {
				continue
			}
			if err := api.install(ctx, e.PkgName); err != nil {
				fmt.Fprintf(w, "FAIL install %s: %s\n", e.PkgName, err)
			}
		}
	}

	all, err := api.sources(ctx)
	if err != nil {
		return nil, fmt.Errorf("source list: %w", err)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	var sources []suwayomiSource
	for i, s := range all {
		if s.ID == "0" || !wantLang(langs, s.Lang) {
			continue
		}
		if len(o.Sources) > 0 && !nameOrID(o.Sources, s) {
			continue
		}
		site := siteOfBase(s.BaseURL)
		if site == "" {
			if strings.TrimSpace(s.BaseURL) != "" {
				fmt.Fprintf(w, "%s  skipped: no usable base URL (%q)\n", s.Name, s.BaseURL)
			}
			continue
		}
		// shards split by host, not by position, so two sources on one host
		// (a mirror, an "en" and an "all" source) merge inside one process
		if o.Shards > 0 && shardOf(site, o.Shards) != o.Shard {
			continue
		}
		_ = i
		sources = append(sources, s)
	}

	store := NewStore(o.Out)
	for _, warn := range store.Load() {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}

	rep := &SuwayomiReport{}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, o.Parallel)
		// writes serialises the merge of one site, so two sources on the
		// same host don't lose each other's entries
		writes sync.Mutex
	)
	for i, src := range sources {
		site := siteOfBase(src.BaseURL)
		row := SourceRow{Site: site, Name: src.Name, Lang: src.Lang, NSFW: src.NSFW, SourceID: src.ID, Complete: true}
		if _, known := SiteByHost(site); known {
			row.Downloadable = true
		}
		// a slot is taken here, before the goroutine, so the clock is read
		// when a source could actually start; the first source always runs
		sem <- struct{}{}
		if i > 0 && o.Timeout > 0 && time.Since(started) > o.Timeout {
			<-sem
			row.Failed = true
			mu.Lock()
			rep.Sources = append(rep.Sources, row)
			rep.Failed++
			mu.Unlock()
			fmt.Fprintf(w, "%s  skipped: timeout\n", site)
			continue
		}
		wg.Add(1)
		go func(src suwayomiSource, site string, row SourceRow) {
			defer wg.Done()
			defer func() { <-sem }()
			listed, pages, complete := walkSource(ctx, api, src, o)
			writes.Lock()
			existing, _ := store.Catalog(site)
			writes.Unlock()
			entries, links, fetched, linked := resolveLinks(ctx, api, listed, existing, o)
			row.Links, row.Fetched = links, fetched
			row.Complete = complete && linked
			complete = row.Complete
			if len(entries) == 0 {
				row.Failed = true
				mu.Lock()
				rep.Sources = append(rep.Sources, row)
				rep.Failed++
				mu.Unlock()
				fmt.Fprintf(w, "%s  FAIL: no entries (%s)\n", site, src.Name)
				return
			}
			writes.Lock()
			n, err := mergeSite(store, site, src, entries, pages, complete, links)
			writes.Unlock()
			if err != nil {
				row.Failed = true
				mu.Lock()
				rep.Sources = append(rep.Sources, row)
				rep.Failed++
				mu.Unlock()
				fmt.Fprintf(w, "%s  FAIL: %s\n", site, err)
				return
			}
			row.Entries = n
			mu.Lock()
			rep.Sources = append(rep.Sources, row)
			rep.Written = append(rep.Written, site)
			mu.Unlock()
			note := ""
			if !complete {
				note = " (cut at the page cap)"
			}
			if links == "fetched" {
				note += fmt.Sprintf(" (%d links fetched)", fetched)
			}
			fmt.Fprintf(w, "%s  %d entries from %s, %d pages%s\n", site, n, src.Name, pages, note)
		}(src, site, row)
	}
	wg.Wait()

	sort.Slice(rep.Sources, func(i, j int) bool { return rep.Sources[i].SourceID < rep.Sources[j].SourceID })
	sort.Strings(rep.Written)
	name := "sources.json"
	if o.Shards > 0 {
		name = fmt.Sprintf("sources-%d.json", o.Shard)
	}
	b, _ := json.MarshalIndent(rep.Sources, "", " ")
	if err := writeFileAtomic(filepath.Join(o.Out, name), b); err != nil {
		return rep, err
	}

	return rep, nil
}

// shardOf picks the shard of a site: a hash of its name modulo the shard
// count, the same on every runner
func shardOf(site string, shards int) int {
	h := fnv.New32a()
	h.Write([]byte(site))

	return int(h.Sum32() % uint32(shards))
}

// lockedWriter serialises the log lines of the parallel walks
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.w.Write(b)
}

// nameOrID reports whether a source is one of the names or ids asked for
func nameOrID(want []string, s suwayomiSource) bool {
	for _, w := range want {
		if strings.EqualFold(w, s.Name) || w == s.ID {
			return true
		}
	}

	return false
}

// walkSource unions the source's popular, latest and empty-search listings.
// It returns the listed manga (joined links), how many pages were read, and
// whether no listing was cut at the page cap.
func walkSource(ctx context.Context, api suwayomi, src suwayomiSource, o SuwayomiOptions) (entries []walked, pages int, complete bool) {
	complete = true
	listings := []string{"popular"}
	if src.SupportsLatest {
		listings = append(listings, "latest")
	}
	listings = append(listings, "search")
	seen := map[string]bool{}
	for _, listing := range listings {
		errs := 0
		for n := 1; ; n++ {
			if n > o.MaxPages {
				complete = false
				break
			}
			if n > 1 && o.Pause > 0 {
				select {
				case <-ctx.Done():
					return entries, pages, false
				case <-time.After(o.Pause):
				}
			}
			resp, err := api.page(ctx, src.ID, listing, n)
			if err != nil {
				if ctx.Err() != nil {
					return entries, pages, false
				}
				errs++
				if errs >= 3 {
					complete = false
					break
				}
				n--
				continue
			}
			errs = 0
			pages++
			added := 0
			for _, m := range resp.MangaList {
				u := joinBase(src.BaseURL, m.URL)
				key := urlKey(u)
				if key == "" || strings.TrimSpace(m.Title) == "" || seen[key] {
					continue
				}
				seen[key] = true
				entries = append(entries, walked{id: m.ID, title: strings.TrimSpace(m.Title), url: u})
				added++
			}
			if !resp.HasNextPage || added == 0 {
				break
			}
		}
	}

	return entries, pages, complete
}

// resolveLinks turns the listed manga into entries with a link that opens.
// One title's details are fetched to compare the extension's real URL with
// the joined one: equal means the joins are right ("joined"); different
// means every title needs its own fetch ("fetched"), up to MaxFetch per run
// (titles the existing file already links are skipped, the rest wait for
// the next run and are left out rather than stored with a wrong link); a
// check that fails keeps the joins ("unchecked"). linked is false when
// titles were left out.
func resolveLinks(ctx context.Context, api suwayomi, listed []walked, existing *SiteCatalog, o SuwayomiOptions) (entries []Entry, links string, fetched int, linked bool) {
	linked = true
	if len(listed) == 0 {
		return nil, "", 0, true
	}
	joined := func(list []walked) []Entry {
		out := make([]Entry, 0, len(list))
		for _, m := range list {
			out = append(out, Entry{Title: m.title, URL: m.url})
		}
		return out
	}
	sample, err := api.details(ctx, listed[0].id)
	if err != nil || sample.RealURL == "" {
		return joined(listed), "unchecked", 0, true
	}
	if sameLink(sample.RealURL, listed[0].url) {
		return joined(listed), "joined", 0, true
	}
	// the join is wrong for this source. Often the real link is the joined
	// path's last part under another prefix (mangadex: /manga/x → /title/x):
	// a template learnt from the sample and confirmed on a second title
	// links every entry without a fetch each
	if tpl, ok := inferLinkTemplate(listed[0].url, sample.RealURL); ok {
		confirmed := len(listed) == 1
		if !confirmed {
			second, err := api.details(ctx, listed[1].id)
			fetched++
			confirmed = err == nil && second.RealURL != "" && tpl.matches(listed[1].url, second.RealURL)
		}
		if confirmed {
			entries = make([]Entry, 0, len(listed))
			for _, m := range listed {
				entries = append(entries, Entry{Title: m.title, URL: tpl.apply(m.url)})
			}
			return entries, "templated", fetched, true
		}
	}
	// fetch the real links one by one, skipping the titles the file already
	// links the same way
	known := map[string]bool{}
	if existing != nil && existing.Source != nil && existing.Source.Links == "fetched" {
		for _, e := range existing.Entries {
			known[Fold(e.Title)] = true
		}
	}
	entries = append(entries, Entry{Title: listed[0].title, URL: sample.RealURL})
	for _, m := range listed[1:] {
		if known[Fold(m.title)] {
			continue
		}
		if fetched >= o.MaxFetch {
			linked = false
			break
		}
		if err := ctx.Err(); err != nil {
			linked = false
			break
		}
		d, err := api.details(ctx, m.id)
		fetched++
		if err != nil || d.RealURL == "" {
			linked = false
			continue
		}
		entries = append(entries, Entry{Title: m.title, URL: d.RealURL})
	}

	return entries, "fetched", fetched, linked
}

// linkTemplate rebuilds a real link from a joined one: the joined path's
// last segment between a prefix and a suffix
type linkTemplate struct {
	prefix, suffix string
	// loose means the real links carry more path after the segment (a
	// title slug), which the template leaves out
	loose bool
}

// apply makes the real link of a joined one
func (t linkTemplate) apply(joined string) string {
	return t.prefix + lastSegment(joined) + t.suffix
}

// lastSegment is a link's last path segment, without a trailing slash
func lastSegment(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	path := strings.TrimSuffix(u.Path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}

	return path
}

// inferLinkTemplate learns how a real link holds the joined one's last
// segment: found whole in it, with nothing but a slash after. An id turned
// into a name, or a per-title hash appended, gives no template.
func inferLinkTemplate(joined, real string) (linkTemplate, bool) {
	seg := lastSegment(joined)
	if len(seg) < 3 {
		return linkTemplate{}, false
	}
	i := strings.Index(real, seg)
	if i < 0 {
		return linkTemplate{}, false
	}
	suffix := real[i+len(seg):]
	switch {
	case suffix == "" || suffix == "/":
		return linkTemplate{prefix: real[:i], suffix: suffix}, true
	case strings.HasPrefix(suffix, "/"):
		// a title slug after the id (mangadex: /title/<id>/<slug>): the short
		// form is the page's canonical link, so the template drops it
		return linkTemplate{prefix: real[:i], loose: true}, true
	}

	return linkTemplate{}, false
}

// matches reports whether a real link is what the template makes of a
// joined one: equal, or for a loose template, the made link plus more path
func (t linkTemplate) matches(joined, real string) bool {
	made := t.apply(joined)
	if sameLink(made, real) {
		return true
	}

	return t.loose && strings.HasPrefix(strings.TrimSuffix(real, "/"), strings.TrimSuffix(made, "/")+"/")
}

// sameLink compares two series links ignoring a trailing slash and www.
func sameLink(a, b string) bool {
	return urlKey(a) == urlKey(b)
}

// joinBase builds a series URL the way Tachiyomi's HttpSource does: the
// source's base URL plus the manga's path, by plain concatenation (a base
// with a path keeps it); an absolute path is kept as is
func joinBase(base, path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return strings.TrimSuffix(strings.TrimSpace(base), "/") + path
}

// mergeSite unions the entries into the site's file (the existing ones
// first, a newer latest chapter winning) and returns the file's new size. A
// file whose set of titles didn't change is left alone, so its published
// hash stays put and users don't re-download it.
func mergeSite(store *Store, site string, src suwayomiSource, entries []Entry, pages int, complete bool, links string) (int, error) {
	c := &SiteCatalog{Site: site, Home: strings.TrimSuffix(strings.TrimSpace(src.BaseURL), "/"), Language: src.Lang}
	var old *SiteCatalog
	if existing, ok := store.Catalog(site); ok {
		old = existing
		cp := *old
		c = &cp
		if c.Home == "" {
			c.Home = strings.TrimSuffix(strings.TrimSpace(src.BaseURL), "/")
		}
		if c.Language == "" {
			c.Language = src.Lang
		}
	}
	if old != nil && old.Source != nil && linksReplace(old.Source.Links, links) {
		// the file's links were joined and the join is now known wrong: the
		// real links replace them rather than join them (the hand-written
		// crawlers' entries for the site come back with their next run)
		c.Entries = nil
	}
	c.Entries = union(c.Entries, entries, true)
	sort.SliceStable(c.Entries, func(i, j int) bool { return Fold(c.Entries[i].Title) < Fold(c.Entries[j].Title) })
	if old != nil && old.Source != nil && old.Source.ID == src.ID && old.Source.Links == links && sameEntries(old.Entries, c.Entries) {
		return len(c.Entries), nil
	}
	c.Crawled = time.Now().UTC()
	c.Source = &SourceInfo{ID: src.ID, Name: src.Name, Lang: src.Lang, NSFW: src.NSFW, Pages: pages, Complete: complete, Links: links}
	if err := store.Write(c); err != nil {
		return 0, err
	}

	return len(c.Entries), nil
}

// linksReplace reports whether a file's entries, linked the old way, must
// give way to entries linked the new way: joined (or unknown) links are
// wrong once a source proves fetched or templated
func linksReplace(old, now string) bool {
	return (old == "" || old == "joined" || old == "unchecked") && (now == "fetched" || now == "templated")
}

// sameEntries reports whether two entry lists hold the same titles, URLs
// and latest chapters in the same order
func sameEntries(a, b []Entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Title != b[i].Title || a[i].URL != b[i].URL || a[i].Latest != b[i].Latest {
			return false
		}
	}

	return true
}
