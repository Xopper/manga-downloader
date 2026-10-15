// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/elboletaire/manga-downloader/catalog"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var catalogFlags struct {
	site  string
	limit int
	out   string
	json  bool
	gzip  bool
}

var suwayomiFlags struct {
	server    string
	langs     []string
	maxPages  int
	maxFetch  int
	parallel  int
	shard     string
	sources   []string
	noInstall bool
	timeout   time.Duration
}

var catalogSuwayomiCmd = &cobra.Command{
	Use:   "crawl-suwayomi",
	Short: "Builds catalogs through a running Suwayomi-Server (every Tachiyomi source of a language)",
	Long: `Walks the popular, latest and empty-search listings of every source of the
languages given on a Suwayomi-Server, and merges the titles into the catalog
files of --out, one per site (a site a hand-written crawler already covers
gets the union). The extensions of those languages are installed first unless
--no-install. Writes sources.json (sources-<shard>.json with --shard) listing
every source. Exit code 2 when a source failed or was skipped by --timeout.

tools/suwayomi/run.sh starts a server on this machine; the nightly job runs one
on GitHub.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		shard, shards, err := parseShard(suwayomiFlags.shard)
		if err != nil {
			red("%s", err)
			exit(1)
		}
		rep, err := catalog.CrawlSuwayomi(context.Background(), os.Stdout, catalog.SuwayomiOptions{
			Server:   suwayomiFlags.server,
			Langs:    suwayomiFlags.langs,
			Out:      catalogFlags.out,
			MaxPages: suwayomiFlags.maxPages,
			MaxFetch: suwayomiFlags.maxFetch,
			Parallel: suwayomiFlags.parallel,
			Shard:    shard,
			Shards:   shards,
			Sources:  suwayomiFlags.sources,
			Install:  !suwayomiFlags.noInstall,
			Timeout:  suwayomiFlags.timeout,
		})
		if err != nil {
			red("%s", err)
			exit(1)
		}
		fmt.Printf("%d sources, %d written, %d failed\n", len(rep.Sources), len(rep.Written), rep.Failed)
		if rep.Failed > 0 {
			exit(2)
		}
	},
}

// parseShard reads "i/n" ("" is no sharding)
func parseShard(s string) (shard, shards int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, nil
	}
	i, n, ok := strings.Cut(s, "/")
	if !ok {
		return 0, 0, fmt.Errorf("--shard wants i/n, got %q", s)
	}
	shard, err1 := strconv.Atoi(i)
	shards, err2 := strconv.Atoi(n)
	if err1 != nil || err2 != nil || shards <= 0 || shard < 0 || shard >= shards {
		return 0, 0, fmt.Errorf("--shard wants i/n with 0 <= i < n, got %q", s)
	}

	return shard, shards, nil
}

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manages the sites' catalogs that \"search\" looks in",
}

var catalogUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Downloads the catalogs that changed since the last update",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		store, err := openStore(false)
		if err != nil {
			red("%s", err)
			exit(1)
		}
		changed, err := store.Update(context.Background(), catalog.BaseURL())
		for _, site := range changed {
			fmt.Println("updated", site)
		}
		if err != nil {
			red("%s", err)
			exit(1)
		}
		if len(changed) == 0 {
			fmt.Println("Already up to date")
		}
	},
}

var catalogStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Shows every site's catalog: entries, crawl time, age",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		store, err := openStore(false)
		if err != nil {
			red("%s", err)
			exit(1)
		}
		runStatus(os.Stdout, store, catalogFlags.json)
	},
}

var catalogCrawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Builds the catalogs from the sites (what the nightly job runs)",
	Long: `Crawls every site with a crawler (or the one given with --site) and writes
one <site>.json per site plus index.json into --out. A site's existing file
is kept when the new crawl has fewer than half its entries, and the command
ends with exit code 2 if any site failed or was kept.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		exit(runCrawl(context.Background(), os.Stdout, catalogFlags.out, catalogFlags.site, catalogFlags.limit))
	},
}

var catalogIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Rebuilds index.json (and sources.json) of a catalog directory",
	Long: `Merges the sources-<shard>.json files a sharded Suwayomi crawl left into
sources.json, then writes index.json listing every catalog file with its
hash. With --gzip, the catalog files are gzipped first (the published form).`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		exit(runIndex(os.Stdout, catalogFlags.out, catalogFlags.gzip))
	},
}

// runIndex merges the sources files and rebuilds the index of dir
func runIndex(w io.Writer, dir string, gzip bool) int {
	rows, err := catalog.MergeSources(dir)
	if err != nil {
		fmt.Fprintln(w, color.RedString("sources: %s", err))
		return 2
	}
	if gzip {
		if err := catalog.Gzip(dir); err != nil {
			fmt.Fprintln(w, color.RedString("gzip: %s", err))
			return 2
		}
	}
	idx, err := catalog.BuildIndex(dir)
	if err == nil {
		err = catalog.WriteIndex(dir, idx)
	}
	if err != nil {
		fmt.Fprintln(w, color.RedString("index: %s", err))
		return 2
	}
	total := 0
	for _, s := range idx.Sites {
		total += s.Entries
	}
	fmt.Fprintf(w, "%d %s, %d entries, %d sources\n", len(idx.Sites), plural(len(idx.Sites), "site"), total, len(rows))

	return 0
}

// plural is "site" or "sites"
func plural(n int, word string) string {
	if n == 1 {
		return word
	}

	return word + "s"
}

// statusRow is a site in the status output
type statusRow struct {
	Site     string `json:"site"`
	Label    string `json:"label,omitempty"`
	Language string `json:"lang"`
	// Crawlable is whether the site has a crawler at all
	Crawlable bool `json:"crawlable"`
	// Downloadable is whether the downloader has a grabber for the site
	Downloadable bool `json:"downloadable"`
	// NSFW and Source come from the site's Tachiyomi source
	NSFW   bool   `json:"nsfw,omitempty"`
	Source string `json:"source,omitempty"`
	// Catalog is whether a catalog is on disk
	Catalog bool       `json:"catalog"`
	Entries int        `json:"entries"`
	Crawled *time.Time `json:"crawled,omitempty"`
	// Age is how long ago the local file was written
	Age string `json:"age,omitempty"`
}

// runStatus prints the store's status, as a table or JSON
func runStatus(w io.Writer, store *catalog.Store, asJSON bool) {
	var rows []statusRow
	for _, st := range store.Status() {
		r := statusRow{Site: st.Site, Label: st.Label, Language: st.Language, Crawlable: st.Crawlable, Downloadable: st.Downloadable, NSFW: st.NSFW, Source: st.Source, Catalog: st.HasCatalog, Entries: st.Entries}
		if st.HasCatalog {
			crawled := st.Crawled
			r.Crawled = &crawled
			r.Age = st.FileAge.Round(time.Minute).String()
		}
		rows = append(rows, r)
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(rows)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SITE\tLANG\tDL\tENTRIES\tCRAWLED\tAGE\tNOTE")
	for _, r := range rows {
		dl := "-"
		if r.Downloadable {
			dl = "yes"
		}
		note := ""
		if r.NSFW {
			note = "nsfw"
		}
		switch {
		case r.Catalog:
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n", r.Site, r.Language, dl, r.Entries, r.Crawled.Local().Format("2006-01-02 15:04"), r.Age, note)
		case r.Crawlable:
			fmt.Fprintf(tw, "%s\t%s\t%s\t-\tnot downloaded\t\t%s\n", r.Site, r.Language, dl, note)
		default:
			fmt.Fprintf(tw, "%s\t%s\t%s\t-\tno catalog\t\t%s\n", r.Site, r.Language, dl, note)
		}
	}
	tw.Flush()
}

// sitesForCrawl lists the site table; tests replace it
var sitesForCrawl = catalog.Sites

// crawlParallel is how many sites are crawled at once
const crawlParallel = 2

// runCrawl crawls the sites into out and writes the index. The exit code is
// 1 when the requested site is unknown, 2 when a site failed or its old file
// was kept, 0 otherwise.
func runCrawl(ctx context.Context, w io.Writer, out, only string, limit int) int {
	sites := sitesForCrawl()
	if only != "" {
		only = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(only)), "www.")
		var found []catalog.SiteInfo
		for _, s := range sites {
			if s.Site == only {
				found = append(found, s)
			}
		}
		if len(found) == 0 {
			fmt.Fprintln(w, color.RedString("Unknown site: %s", only))
			return 1
		}
		sites = found
	}
	store := catalog.NewStore(out)
	for _, warn := range store.Load() {
		fmt.Fprintln(w, color.YellowString("%s", warn))
	}

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		failed bool
		sem    = make(chan struct{}, crawlParallel)
	)
	report := func(format string, a ...any) {
		mu.Lock()
		fmt.Fprintf(w, format+"\n", a...)
		mu.Unlock()
	}
	for _, site := range sites {
		if !site.Crawl && only == "" {
			continue
		}
		crawler, ok := catalog.CrawlerFor(site)
		if !ok {
			report("%s  %s", site.Site, color.YellowString("no crawler for family %q", site.Family))
			if only != "" {
				mu.Lock()
				failed = true
				mu.Unlock()
			}
			continue
		}
		wg.Add(1)
		go func(site catalog.SiteInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			entries, err := crawler.Crawl(ctx, site, limit)
			// a walk that stopped on an error (a 503 on page 12) is partial: it
			// never replaces a file, or the catalog would shrink run by run
			if err == nil && len(entries) == 0 {
				err = fmt.Errorf("no entries")
			}
			if err != nil {
				if len(entries) > 0 {
					err = fmt.Errorf("stopped after %d entries: %w", len(entries), err)
				}
				report("%s  %s", site.Site, color.RedString("FAIL: %s", err))
				mu.Lock()
				failed = true
				mu.Unlock()
				return
			}
			old, had := store.Catalog(site.Site)
			if had && len(entries)*2 < len(old.Entries) {
				report("%s  %s", site.Site, color.YellowString("kept the old file: %d entries now, %d before", len(entries), len(old.Entries)))
				mu.Lock()
				failed = true
				mu.Unlock()
				return
			}
			c := &catalog.SiteCatalog{Site: site.Site, Home: site.Home, Language: site.Lang, Crawled: time.Now().UTC(), Entries: entries}
			if had && old.Source != nil {
				// a Tachiyomi source also lists this site: its entries stay,
				// after the crawl's own, so the nightly jobs don't undo each other
				c.Entries = catalog.Dedupe(append(append([]catalog.Entry{}, entries...), old.Entries...))
				c.Source = old.Source
			}
			if werr := store.Write(c); werr != nil {
				report("%s  %s", site.Site, color.RedString("FAIL: %s", werr))
				mu.Lock()
				failed = true
				mu.Unlock()
				return
			}
			delta := ""
			if had {
				delta = fmt.Sprintf(" (%+d)", len(entries)-len(old.Entries))
			}
			report("%s  %d entries%s", site.Site, len(entries), delta)
		}(site)
	}
	wg.Wait()

	idx, err := catalog.BuildIndex(out)
	if err == nil {
		sort.Slice(idx.Sites, func(i, j int) bool { return idx.Sites[i].Site < idx.Sites[j].Site })
		err = catalog.WriteIndex(out, idx)
	}
	if err != nil {
		fmt.Fprintln(w, color.RedString("index: %s", err))
		return 2
	}
	if failed {
		return 2
	}

	return 0
}

func init() {
	catalogStatusCmd.Flags().BoolVar(&catalogFlags.json, "json", false, "print the status as JSON")
	catalogCrawlCmd.Flags().StringVar(&catalogFlags.site, "site", "", "only this site (its domain)")
	catalogCrawlCmd.Flags().IntVar(&catalogFlags.limit, "limit", 0, "stop after this many list pages per site (smoke checks)")
	catalogCrawlCmd.Flags().StringVar(&catalogFlags.out, "out", "./catalog", "directory for the catalog files")
	catalogIndexCmd.Flags().StringVar(&catalogFlags.out, "out", "./catalog", "the catalog directory")
	catalogIndexCmd.Flags().BoolVar(&catalogFlags.gzip, "gzip", false, "gzip the catalog files first (the published form)")
	catalogSuwayomiCmd.Flags().StringVar(&suwayomiFlags.server, "server", "http://127.0.0.1:4567", "the Suwayomi-Server URL")
	catalogSuwayomiCmd.Flags().StringSliceVar(&suwayomiFlags.langs, "lang", []string{"en"}, "source languages to crawl (repeatable; multi-language sources are always included)")
	catalogSuwayomiCmd.Flags().IntVar(&suwayomiFlags.maxPages, "max-pages", 300, "most pages read per listing of a source")
	catalogSuwayomiCmd.Flags().IntVar(&suwayomiFlags.maxFetch, "max-fetch", 200, "most series links fetched one by one per source and run (sources whose link isn't base + path)")
	catalogSuwayomiCmd.Flags().IntVar(&suwayomiFlags.parallel, "parallel", 4, "sources walked at once")
	catalogSuwayomiCmd.Flags().StringVar(&suwayomiFlags.shard, "shard", "", "only every n-th source, starting at i: \"i/n\"")
	catalogSuwayomiCmd.Flags().StringSliceVar(&suwayomiFlags.sources, "source", nil, "only these sources, by name or id (repeatable)")
	catalogSuwayomiCmd.Flags().BoolVar(&suwayomiFlags.noInstall, "no-install", false, "don't install the languages' missing extensions first")
	catalogSuwayomiCmd.Flags().DurationVar(&suwayomiFlags.timeout, "timeout", 0, "stop starting sources after this long (e.g. 4h30m)")
	catalogSuwayomiCmd.Flags().StringVar(&catalogFlags.out, "out", "./catalog", "directory for the catalog files")
	catalogCmd.AddCommand(catalogUpdateCmd, catalogStatusCmd, catalogCrawlCmd, catalogIndexCmd, catalogSuwayomiCmd)
	rootCmd.AddCommand(catalogCmd)
}
