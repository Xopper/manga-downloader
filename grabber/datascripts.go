// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/base64"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// dataScripts returns the source of the page's scripts that are inlined as
// base64 data: URLs, joined by newlines. Some WordPress optimization plugins
// move every inline script there (scythescans.com), which hides a themesia
// reader's ts_reader.run(...) call from a search of the HTML.
func dataScripts(doc *goquery.Document) string {
	const prefix = "data:text/javascript;base64,"
	var b strings.Builder
	doc.Find(`script[src^="` + prefix + `"]`).Each(func(_ int, s *goquery.Selection) {
		src, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s.AttrOr("src", ""), prefix))
		if err == nil {
			b.WriteByte('\n')
			b.Write(src)
		}
	})
	return b.String()
}
