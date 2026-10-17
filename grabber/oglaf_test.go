// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestOglafStories(t *testing.T) {
	const archive = `<html><body>
<a href="/archive/"><img src="banner.png" width="400"></a>
<a href="/lavender/"><img src="https://media.oglaf.com/archive/arc-lavender.jpg" width="400" height="100" /></a>
<a href="/damselfly/"><img src="https://media.oglaf.com/archive/arc-damselfly.jpg" width="400" height="100" /></a>
<a href="/lavender/"><img src="https://media.oglaf.com/archive/arc-lavender.jpg" width="400" height="100" /></a>
<a href="/feeds/rss/"><img src="rss.gif"></a>
<a href="https://www.oglaf.com/cumsprite/"><img src="https://media.oglaf.com/archive/arc-cumsprite-d.png" width="400" /></a>
</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	got := oglafStories(doc)
	want := []string{"/lavender/", "/damselfly/", "/cumsprite/"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("stories = %v, want %v", got, want)
	}
}

func TestOglafNextInStory(t *testing.T) {
	cases := []struct {
		next, slug, want string
	}{
		{"/cumsprite/2/", "cumsprite", "/cumsprite/2/"},
		{"/cumsprite/3/", "cumsprite", "/cumsprite/3/"},
		{"/8legs/", "fountain-of-doubt", ""},
		{"/other/2/", "cumsprite", ""},
		{"", "cumsprite", ""},
	}
	for _, c := range cases {
		page := `<html><body><img id="strip" src="x.jpg">`
		if c.next != "" {
			page += `<a href="` + c.next + `" rel="next" class="button next">Next</a>`
		}
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(page + `</body></html>`))
		if got := oglafNextInStory(doc, c.slug); got != c.want {
			t.Errorf("next %q in %q = %q, want %q", c.next, c.slug, got, c.want)
		}
	}
}

func TestOglafTest(t *testing.T) {
	cases := map[string]bool{
		"https://www.oglaf.com/archive/":  true,
		"https://oglaf.com/cumsprite/2/":  true,
		"https://www.oglaf.com.evil.com/": false,
	}
	for u, want := range cases {
		if got, _ := (&Oglaf{Grabber: &Grabber{URL: u}}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", u, got, want)
		}
	}
}
