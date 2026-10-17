// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// madaraForkImages returns the pages of the Madara readers that replaced
// div.reading-content with their own, for Tcb: cartoonporn.to's one
// figure.reader-page per page, and hentai4free.net's JSON list in
// script#h4f-r2-data, filled in by script
func madaraForkImages(doc *goquery.Document) (urls []string) {
	doc.Find("figure.reader-page img.manga-img").Each(func(_ int, s *goquery.Selection) {
		if u := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", ""))); u != "" && !strings.HasPrefix(u, "data:") {
			urls = append(urls, u)
		}
	})
	if len(urls) > 0 {
		return urls
	}
	var data struct {
		Images []struct {
			Src string `json:"src"`
		} `json:"images"`
	}
	if err := json.Unmarshal([]byte(doc.Find("script#h4f-r2-data").Text()), &data); err == nil {
		for _, img := range data.Images {
			if img.Src != "" {
				urls = append(urls, img.Src)
			}
		}
	}
	return urls
}
