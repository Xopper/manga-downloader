// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
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

// statusRow is a site in the status output
type statusRow struct {
	Site     string `json:"site"`
	Label    string `json:"label,omitempty"`
	Language string `json:"lang"`
	// Crawlable is whether the site has a crawler at all
	Crawlable bool `json:"crawlable"`
	// Catalog is whether a catalog is on disk
	Catalog bool      `json:"catalog"`
	Entries int       `json:"entries"`
	Crawled time.Time `json:"crawled,omitempty"`
	// Age is how long ago the local file was written
	Age string `json:"age,omitempty"`
}

// runStatus prints the store's status, as a table or JSON
func runStatus(w io.Writer, store *catalog.Store, asJSON bool) {
	var rows []statusRow
	for _, st := range store.Status() {
		r := statusRow{Site: st.Site, Label: st.Label, Language: st.Language, Crawlable: st.Crawlable, Catalog: st.HasCatalog, Entries: st.Entries, Crawled: st.Crawled}
		if st.HasCatalog {
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
	fmt.Fprintln(tw, "SITE\tLANG\tENTRIES\tCRAWLED\tAGE")
	for _, r := range rows {
		switch {
		case r.Catalog:
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", r.Site, r.Language, r.Entries, r.Crawled.Local().Format("2006-01-02 15:04"), r.Age)
		case r.Crawlable:
			fmt.Fprintf(tw, "%s\t%s\t-\tnot downloaded\t\n", r.Site, r.Language)
		default:
			fmt.Fprintf(tw, "%s\t%s\t-\tno catalog\t\n", r.Site, r.Language)
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
				failed = true
			}
			continue
		}
		wg.Add(1)
		go func(site catalog.SiteInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			entries, err := crawler.Crawl(ctx, site, limit)
			if len(entries) == 0 {
				if err == nil {
					err = fmt.Errorf("no entries")
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
			if err != nil {
				delta += color.YellowString(" (stopped early: %s)", err)
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
	catalogCmd.AddCommand(catalogUpdateCmd, catalogStatusCmd, catalogCrawlCmd)
	rootCmd.AddCommand(catalogCmd)
}
