// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestMangadraft serves trimmed copies of a webtoon (two chapters, the
// second paid) and a one-shot
func newTestMangadraft(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/webtoon/mesalliance":
			fmt.Fprint(w, `<script>window.project = {"id":28125,"name":"Mésalliance","slug":"mesalliance","user":{"id":1}};window.episodes=[72894];</script>`)
		case "/manga/gungnir":
			fmt.Fprint(w, `<script>window.project = {"id":1451,"name":"Gungnir","slug":"gungnir"};</script>`)
		case "/api/reader/28125/categories":
			fmt.Fprint(w, `{"CHAPTER":{"72893":[`+
				`{"id":82859,"name":"Episode 18","order":18,"type":"CHAPTER","nb_pages":72,"is_free":false,"price":"0.50"},`+
				`{"id":72894,"name":"Episode 01 - Leythia","order":1,"type":"CHAPTER","nb_pages":51,"is_free":true,"price":0}]},`+
				`"ROOT":[{"id":72893,"name":"Webtoon","order":1,"type":"ROOT","nb_pages":842}]}`)
		case "/api/reader/1451/categories":
			fmt.Fprint(w, `{"CHAPTER":[],"ROOT":[{"id":4088,"name":"Manga","order":0,"type":"ROOT","nb_pages":20}]}`)
		case "/reader/mesalliance/c.72894":
			fmt.Fprint(w, `<script>window.firstPage = {"id":997448,"number":1,"cat":72894};</script>`)
		case "/api/reader/listPages":
			if r.URL.Query().Get("first_page") != "997448" {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, `{"data":[`+
				`{"id":997448,"number":1,"url":"https://www.mangadraft.com/image/page/a","cat":72894},`+
				`{"id":997449,"number":2,"url":"https://www.mangadraft.com/image/page/b","cat":72894},`+
				`{"id":997500,"number":1,"url":"https://www.mangadraft.com/image/page/c","cat":73242}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMangadraftFetch(t *testing.T) {
	srv := newTestMangadraft(t)
	m := NewMangadraft(&Grabber{URL: srv.URL + "/webtoon/mesalliance", Settings: &Settings{}})

	title, err := m.FetchTitle()
	if err != nil || title != "Mésalliance" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	first := chapters[0].(*MangadraftChapter)
	paid := chapters[1].(*MangadraftChapter)
	if first.Number != 1 || first.CategoryID != 72894 || first.Locked || paid.Number != 2 || !paid.Locked {
		t.Errorf("chapters = %+v, %+v", first.Chapter, paid.Chapter)
	}
	chapter, err := m.FetchChapter(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://www.mangadraft.com/image/page/b?size=full" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}

func TestMangadraftOneShot(t *testing.T) {
	srv := newTestMangadraft(t)
	m := NewMangadraft(&Grabber{URL: srv.URL + "/manga/gungnir", Settings: &Settings{}})
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 1 || chapters[0].(*MangadraftChapter).CategoryID != 4088 || LockedOf(chapters[0]) {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
}
