// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/elboletaire/manga-downloader/catalog"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var searchFlags struct {
	site  string
	limit int
	json  bool
}

// searchCmd finds a series in the downloaded catalogs
var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Finds a manga in the supported sites' catalogs",
	Long: `Searches the catalogs of the supported sites (title and other names) and
prints the matching series with their URLs, which the download command takes.

The catalogs are downloaded on first use and refreshed by "catalog update".`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		store, err := openStore(true)
		if err != nil {
			red("%s", err)
			exit(1)
		}
		query := ""
		for i, a := range args {
			if i > 0 {
				query += " "
			}
			query += a
		}
		exit(runSearch(os.Stdout, store, query, searchFlags.site, searchFlags.limit, searchFlags.json))
	},
}

// searchHit is a search result, the JSON output's row
type searchHit struct {
	Site  string `json:"site"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// runSearch prints the hits of query (in one site, or all), as aligned
// columns or JSON, and returns the exit code: 1 when nothing matched
func runSearch(w io.Writer, store *catalog.Store, query, site string, limit int, asJSON bool) int {
	var sites []string
	if site != "" {
		sites = []string{site}
	}
	hits := store.Search(query, sites, limit)
	rows := make([]searchHit, 0, len(hits))
	for _, h := range hits {
		rows = append(rows, searchHit{Site: h.Site, Title: h.Entry.Title, URL: h.Entry.URL})
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(rows)
	} else if len(rows) == 0 {
		fmt.Fprintln(w, color.YellowString("No results"))
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SITE\tTITLE\tURL")
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Site, truncateString(r.Title, 60), r.URL)
		}
		tw.Flush()
	}
	if len(rows) == 0 {
		return 1
	}

	return 0
}

// openStore loads the catalogs from the config dir. With download set, a
// store without an index yet (first run) downloads the published catalogs
// first; failing that, the caller gets an error telling what to run.
func openStore(download bool) (*catalog.Store, error) {
	dir, err := catalog.DefaultDir()
	if err != nil {
		return nil, err
	}
	store := catalog.NewStore(dir)
	for _, warn := range store.Load() {
		yellow("%s", warn)
	}
	if !download {
		return store, nil
	}
	if _, err := catalog.ReadIndex(dir); err == nil {
		return store, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		yellow("%s", err)
	}
	fmt.Println("Downloading the catalog…")
	if _, err := store.Update(context.Background(), catalog.BaseURL()); err != nil {
		return nil, fmt.Errorf("no catalog yet (%s): run manga-downloader catalog update", err)
	}

	return store, nil
}

func init() {
	searchCmd.Flags().StringVar(&searchFlags.site, "site", "", "only this site (its domain, e.g. mangaread.org)")
	searchCmd.Flags().IntVar(&searchFlags.limit, "limit", 50, "most results to print")
	searchCmd.Flags().BoolVar(&searchFlags.json, "json", false, "print the results as JSON")
	rootCmd.AddCommand(searchCmd)
}
