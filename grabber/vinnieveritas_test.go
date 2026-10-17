// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVinnieVeritasFetch(t *testing.T) {
	// trimmed copies of the archive (both columns list the strips) and a
	// strip page, whose comment holds stale pictures
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/archiveIndex.php":
			entries := `<div class="cccArchiveEntry" style="font-weight: bold">ENGLISH</div>` +
				`<div class="cccArchiveEntry" onclick='changeToComic("CCC024")'>#CCC CCC024</div>` +
				`<div class="cccArchiveEntry" onclick='changeToComic("CCC000")'>#CCC CCC000</div>`
			fmt.Fprintf(w, `<div class="cccLeftInd">%s</div><div class="cccRightInd">%s</div>`, entries, entries)
		case "/CCC024.php":
			fmt.Fprint(w, `<!-- <img class="cccComic crazylan crazylan-en" src="./comics/CCC291_en.jpg"> -->`+
				`<img class="cccComic crazylan crazylan-es" src="./comics/CCCr024.jpg">`+
				`<img class="cccComic crazylan crazylan-en" src="./comics/CCCr024E.jpg">`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	for lang, want := range map[string]string{"": "/comics/CCCr024E.jpg", "es": "/comics/CCCr024.jpg"} {
		v := NewVinnieVeritas(&Grabber{URL: srv.URL + "/archiveIndex.php", Settings: &Settings{Language: lang}})
		chapters, errs := v.FetchChapters()
		if len(errs) > 0 || len(chapters) != 2 || chapters[0].GetNumber() != 0 || chapters[1].GetNumber() != 24 {
			t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
		}
		chapter, err := v.FetchChapter(chapters[1])
		if err != nil {
			t.Fatal(err)
		}
		if len(chapter.Pages) != 1 || chapter.Pages[0].URL != srv.URL+want {
			t.Errorf("%q pages = %+v", lang, chapter.Pages)
		}
	}
}
