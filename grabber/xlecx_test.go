// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestXlecxTest(t *testing.T) {
	cases := map[string]bool{
		"https://xlecx.one/3150-halloween-with-bb-and-raven.html": true,
		"https://xlecx.one/tags/squirting/":                       false,
		"https://example.com/3150-x.html":                         false,
	}
	for in, want := range cases {
		if got, _ := NewXlecx(&Grabber{URL: in}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestXlecxImages(t *testing.T) {
	cases := []struct {
		name, html string
		want       []string
	}{
		{
			name: "post text, inline pictures and linked thumbnails",
			html: `<div id="content-1"><div class="imagegall23"></div></div><div id="content-2"><div class="imagegall23"></div></div>
				<div class="page__text full-text"><img data-src="/uploads/posts/a.png">
				<a href="https://xlecx.one/uploads/posts/b.jpg" class="highslide"><img data-src="/uploads/posts/thumbs/b.jpg"></a>
				<a href="https://xlecx.one/tags/x/"><img src="/c.png"></a></div>`,
			want: []string{"https://xlecx.one/uploads/posts/a.png", "https://xlecx.one/uploads/posts/b.jpg", "https://xlecx.one/c.png"},
		},
		{
			name: "full size tab first",
			html: `<div id="content-1"><div class="imagegall23"><img data-src="/t/1.jpg"></div></div>
				<div id="content-2"><div class="imagegall23"><img data-src="/f/1.jpg"><img src="/f/2.jpg"></div></div>
				<div class="page__text"><img src="/x.jpg"></div>`,
			want: []string{"https://xlecx.one/f/1.jpg", "https://xlecx.one/f/2.jpg"},
		},
	}
	for _, c := range cases {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(c.html))
		if got := xlecxImages(doc, "https://xlecx.one"); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
