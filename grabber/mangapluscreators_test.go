// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMangaPlusCreatorsFetch(t *testing.T) {
	// trimmed copies of a title's two list pages and an episode's viewer
	row := func(id, number string) string {
		return fmt.Sprintf(`<a class="mod-item-series unread" href="/episodes/%s"><div class="number"> %s </div></a>`, id, number)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/titles/1h22" && r.URL.Query().Get("page") == "":
			fmt.Fprint(w, `<div class="book-box"><div class="title">Golden Revenger</div></div>`+row("ep1", "#001")+row("ep2", "#002")+
				`<div class="mod-pagination"><a class="next" href="?page=2">Next</a></div>`)
		case r.URL.Path == "/titles/1h22" && r.URL.Query().Get("page") == "2":
			fmt.Fprint(w, row("ep3", "One-shot")+`<div class="mod-pagination"></div>`)
		case r.URL.Path == "/episodes/ep2":
			fmt.Fprint(w, `<div react="viewer" data-pages="{&quot;pc&quot;:[{&quot;page_no&quot;:1,&quot;image_url&quot;:&quot;https://assets.example.com/1.jpg?h=a&quot;},{&quot;page_no&quot;:2,&quot;image_url&quot;:&quot;https://assets.example.com/2.jpg?h=b&quot;}],&quot;sp&quot;:[]}"></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := NewMangaPlusCreators(&Grabber{URL: srv.URL + "/titles/1h22", Settings: &Settings{}})
	title, err := m.FetchTitle()
	if err != nil || title != "Golden Revenger" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	if chapters[1].GetNumber() != 2 || chapters[2].GetNumber() != 3 || chapters[2].GetTitle() != "One-shot" {
		t.Errorf("chapters = %v %v %q", chapters[1].GetNumber(), chapters[2].GetNumber(), chapters[2].GetTitle())
	}
	chapter, err := m.FetchChapter(chapters[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://assets.example.com/2.jpg?h=b" || chapter.ID != "ep2" {
		t.Errorf("chapter = %+v", chapter)
	}
}

func TestMangaPlusCreatorsTest(t *testing.T) {
	cases := map[string]bool{
		"https://mangaplus-creators.jp/titles/1h2210201001173970007447459":   true,
		"https://mangaplus-creators.jp/episodes/tf2210201001174020007447459": false,
		"https://example.com/titles/1h22":                                    false,
	}
	for u, want := range cases {
		if got, _ := NewMangaPlusCreators(&Grabber{URL: u}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", u, got, want)
		}
	}
}
