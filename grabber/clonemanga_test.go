// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloneMangaFetch(t *testing.T) {
	// a trimmed copy of the viewer, one picture per page
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/viewer.php" || q.Get("series") != "snax" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<select class="series_select"><option value="other">Other</option><option value="snax" selected="selected">My Shut-In Vampire Princess Bride</option></select>`+
			`<select class="page_select"><option value="1">1</option><option value="2">2</option><option value="3">3</option></select>`+
			`<div class="subsectionContainer"><img src="snax_vicky/snax_and_vicky_00%s.png" alt="x"></div>`, q.Get("page"))
	}))
	defer srv.Close()

	c := NewCloneManga(&Grabber{URL: srv.URL + "/viewer.php?series=snax", Settings: &Settings{}})
	chapters, errs := c.FetchChapters()
	if len(errs) > 0 || len(chapters) != 1 || chapters[0].GetTitle() != "My Shut-In Vampire Princess Bride" {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	chapter, err := c.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 3 || chapter.Pages[2].URL != srv.URL+"/snax_vicky/snax_and_vicky_003.png" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
	if ok, _ := NewCloneManga(&Grabber{URL: "https://manga.clone-army.org/viewer.php?series=snax"}).Test(); !ok {
		t.Error("clone-army.org not recognised")
	}
}
