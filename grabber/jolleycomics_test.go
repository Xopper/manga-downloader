// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJolleyComicsFetch(t *testing.T) {
	// trimmed copies of the site's pages: the latest page lists two bold
	// chapters and the pages of the latest one, "Script"
	jumpbox := func(pages string) string {
		return `<select class="jumpbox"><option value="-1">------ Jump To ------</option>` +
			`<option style="font-weight: bold;" value="/TPoH/The_Hook/">The Hook</option>` +
			`<option style="font-weight: bold;" value="/TPoH/Rushes/">Rushes</option>` + pages + `</select>`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/TPoH/":
			fmt.Fprint(w, jumpbox(`<option value="/TPoH/Script/652">Script : Page 652</option><option value="/TPoH/Script/653">Script : Page 653</option>`))
		case "/TPoH/The_Hook/", "/TPoH/The_Hook/2":
			page := "1"
			if r.URL.Path == "/TPoH/The_Hook/2" {
				page = "2"
			}
			fmt.Fprint(w, jumpbox(`<option value="/TPoH/The_Hook/1">The Hook : Page 1</option><option value="/TPoH/The_Hook/2">The Hook : Page 2</option>`)+
				`<div class="comic_comic"><img src="/comics/tpoh_`+page+`.jpg" /></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	j := NewJolleyComics(&Grabber{URL: srv.URL + "/TPoH/Script/653", Settings: &Settings{}})
	if home := NewJolleyComics(&Grabber{URL: "https://jolleycomics.com"}); home.comic() != "TPoH" {
		t.Errorf("home comic = %q", home.comic())
	}
	title, _ := j.FetchTitle()
	if title != "The Property of Hate" {
		t.Errorf("FetchTitle() = %q", title)
	}
	chapters, errs := j.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 || chapters[2].GetTitle() != "Script" || chapters[2].GetNumber() != 3 {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	chapter, err := j.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != srv.URL+"/comics/tpoh_2.jpg" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
