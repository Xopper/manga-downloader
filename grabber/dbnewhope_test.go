// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestDbNewHopeEdition picks the language from any page of its folder
func TestDbNewHopeEdition(t *testing.T) {
	cases := map[string]string{
		"https://dbnewhope.com/espanol/capitulos.php":  "es",
		"https://dbnewhope.com/english/chapters.php":   "en",
		"https://www.dbnewhope.com/francais/3/01.php":  "fr",
		"https://dbnewhope.com/espanol.php":            "es",
		"https://dbnewhope.com/english/1/01.php?page=": "en",
		"https://dbnewhope.com/":                       "",
	}
	for u, want := range cases {
		e, err := NewDbNewHope(&Grabber{URL: u, Settings: &Settings{}}).edition()
		if e.lang != want || (want == "") != (err != nil) {
			t.Errorf("edition(%q) = %q, %v; want %q", u, e.lang, err, want)
		}
	}
}

// TestDbNewHopeChapters reads the cards' number and title
func TestDbNewHopeChapters(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div class="row">
<div class="card"><a href="1/01.php"><img src="/espanol/1/x.png"></a>
<div class="card-body"><h5 class="card-title">Capitulo 1:<br/>Una Oscura Derotta</h5><p>44 Páginas</p></div></div>
<div class="card"><a href="../espanol.php#patreon">Patreon</a><h5 class="card-title">Apoyo</h5></div>
</div>`))
	chapters := dbNewHopeChapters(doc, dbNewHopeEditions[2])
	if len(chapters) != 1 {
		t.Fatalf("got %d chapters", len(chapters))
	}
	c := chapters[0].(*DbNewHopeChapter)
	if c.Number != 1 || c.URL != "https://dbnewhope.com/espanol/1/01.php" || ChapterLabel(c) != "Capítulo 1 - Una Oscura Derotta" || c.Language != "es" {
		t.Errorf("chapter = %+v, label %q", c, ChapterLabel(c))
	}
}

// TestDbNewHopeImage resolves the reader's image against its page
func TestDbNewHopeImage(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div class="image-wrapper"><a href="?page=2"><img src="/espanol/1/fe07.png?v=1" class="manga-page"></a></div>`))
	if got := dbNewHopeImage(doc, "https://dbnewhope.com/espanol/1/01.php?page=1"); got != "https://dbnewhope.com/espanol/1/fe07.png?v=1" {
		t.Errorf("image = %q", got)
	}
}
