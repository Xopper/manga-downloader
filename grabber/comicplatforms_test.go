// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestComicWorld(t *testing.T) {
	row := func(n, class string) string {
		return `<li class="chapter ` + class + `" data-title="A Villainess! Chapter ` + n + `" data-permalink="SRV/chapter/villainess-chapter-` + n + `/"><a href="#">x</a></li>`
	}
	more, _ := json.Marshal(map[string]any{"success": true, "data": map[string]string{"html": row("1", "")}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := "http://" + r.Host
		switch {
		case r.URL.Path == "/wp-admin/admin-ajax.php":
			r.ParseForm()
			if r.Form.Get("action") != "load_more_chapters" || r.Form.Get("nonce") != "n0nce" || r.Form.Get("comic_id") != "42" {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			if r.Form.Get("offset") == "2" {
				w.Write([]byte(strings.ReplaceAll(string(more), "SRV", host)))
				return
			}
			w.Write([]byte(`{"success":false}`))
		case r.URL.Path == "/comic/villainess/":
			page := `<html><body><h1>A Villainess!</h1><ul class="chapters-list">` + row("3", "locked-chapter") + row("2", "") + `</ul>
				<button id="loadMoreChaptersBtn" data-comic-id="42" data-offset="2"></button>
				<script>var comicworld_ajax = {"ajax_url":"x","nonce":"n0nce"};</script></body></html>`
			w.Write([]byte(strings.ReplaceAll(page, "SRV", host)))
		case r.URL.Path == "/chapter/villainess-chapter-1/":
			w.Write([]byte(`<img data-src="https://cdn.test/1/01.webp" class="chapter-image lazy-image"><img data-src="https://cdn.test/1/02.webp" class="chapter-image">`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewComicWorld(&Grabber{URL: srv.URL + "/comic/villainess/", Settings: &Settings{}})
	if ok, err := c.Test(); !ok || err != nil {
		t.Fatalf("Test() = %v, %v", ok, err)
	}
	if title, _ := c.FetchTitle(); title != "A Villainess!" {
		t.Errorf("title = %q", title)
	}
	chapters, errs := c.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	if chapters[0].GetNumber() != 1 || !chapters[2].(*ComicWorldChapter).Locked || chapters[1].(*ComicWorldChapter).Locked {
		t.Errorf("chapters = %+v", chapters)
	}
	ch, err := c.FetchChapter(chapters[0])
	if err != nil || len(ch.Pages) != 2 || ch.Pages[1].URL != "https://cdn.test/1/02.webp" {
		t.Errorf("FetchChapter() = %+v, %v", ch, err)
	}
}

func TestTapas(t *testing.T) {
	episodes := func(rows string, next bool) string {
		b, _ := json.Marshal(map[string]any{"data": map[string]any{"pagination": map[string]any{"has_next": next}, "body": rows}})
		return string(b)
	}
	row := func(id, n, title string, locked bool) string {
		lock := ""
		if locked {
			lock = `<div class="thumb__overlay thumb__overlay--locked"></div>`
		}
		return `<li data-id="` + id + `" data-permalink="/episode/` + id + `">` + lock + `<a class="info__title">` + n + `. ` + title + `</a></li>`
	}
	srv := wcTestServer(t, map[string]string{
		"/series/the-bride": `<html><head><meta property="al:android:url" content="tapastic://series/322817"></head>
			<body><p class="center-info__title">The Fake Bride</p></body></html>`,
		"/series/322817/episodes?page=1&sort=OLDEST&max_limit=20": episodes(row("11", "1", "Endurance", false), true),
		"/series/322817/episodes?page=2&sort=OLDEST&max_limit=20": episodes(row("12", "2", "Escape", true), false),
		"/episode/11": `<img class="content__img js-lazy" src="data:image/gif;base64,R0lG" data-src="https://us-a.tapas.test/pc/1.jpg?__token__=x&amp;version=v4">`,
	})
	old := tapasBase
	tapasBase = srv.URL
	t.Cleanup(func() { tapasBase = old })

	tp := NewTapas(&Grabber{URL: srv.URL + "/series/the-bride", Settings: &Settings{}})
	if title, err := tp.FetchTitle(); err != nil || title != "The Fake Bride" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := tp.FetchChapters()
	if len(errs) > 0 || wcLabels(chapters) != "Chapter 1 - Endurance|Chapter 2 - Escape" {
		t.Fatalf("FetchChapters() = %q, %v", wcLabels(chapters), errs)
	}
	if chapters[0].(*TapasChapter).Locked || !chapters[1].(*TapasChapter).Locked {
		t.Error("only the second episode is locked")
	}
	ch, err := tp.FetchChapter(chapters[0])
	if err != nil || len(ch.Pages) != 1 || ch.Pages[0].URL != "https://us-a.tapas.test/pc/1.jpg?__token__=x&version=v4" {
		t.Errorf("FetchChapter() = %+v, %v", ch, err)
	}
}

func TestNamiComi(t *testing.T) {
	srv := wcTestServer(t, map[string]string{
		"/title/TatWWTNm":      `{"data":{"attributes":{"title":{"fr":"Vin grec","en":"Greek Wine"}}}}`,
		"/chapter":             `{"data":[{"id":"vCJq","attributes":{"volume":"1","chapter":"1","name":"Pan and Puk","translatedLanguage":"en"}},{"id":"QgWb","attributes":{"volume":null,"chapter":"2","name":"","translatedLanguage":"en"}}],"meta":{"total":2}}`,
		"/images/chapter/vCJq": `{"data":{"baseUrl":"https://uploads.test","hash":"c646","source":[{"filename":"1-d7.jpg"},{"filename":"2-da.jpg"}]}}`,
	})
	old := namiComiAPI
	namiComiAPI = srv.URL
	t.Cleanup(func() { namiComiAPI = old })

	n := NewNamiComi(&Grabber{URL: "https://namicomi.com/en/title/TatWWTNm/greek-wine", Settings: &Settings{}})
	if ok, _ := n.Test(); !ok || n.id != "TatWWTNm" {
		t.Fatalf("Test() didn't read the title's ID: %q", n.id)
	}
	if title, err := n.FetchTitle(); err != nil || title != "Greek Wine" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := n.FetchChapters()
	if len(errs) > 0 || wcLabels(chapters) != "Chapter 1 - Pan and Puk|Chapter 2" || VolumeOf(chapters[0]) != 1 {
		t.Fatalf("FetchChapters() = %q, %v", wcLabels(chapters), errs)
	}
	ch, err := n.FetchChapter(chapters[0])
	if err != nil || len(ch.Pages) != 2 || ch.Pages[1].URL != "https://uploads.test/chapter/vCJq/c646/source/2-da.jpg" {
		t.Errorf("FetchChapter() = %+v, %v", ch, err)
	}
}

func TestManta(t *testing.T) {
	srv := wcTestServer(t, map[string]string{
		"/series/3429": `{"data":{"data":{"title":{"en":"If It's Remorse You Desire"}},"episodes":[
			{"id":149538,"ord":1,"data":{"title":"S1 Episode 1"},"lockData":{"state":110}},
			{"id":149535,"ord":2,"data":{"title":"S1 Episode 2"},"lockData":{"state":40}}]}}`,
		"/episodes/149538": `{"data":{"cutImages":[{"downloadUrl":"https://si.manta.test/a.jpg?sign=1"},{"downloadUrl":"https://si.manta.test/b.jpg?sign=2"}]}}`,
	})
	old := mantaAPI
	mantaAPI = srv.URL
	t.Cleanup(func() { mantaAPI = old })

	m := NewManta(&Grabber{URL: "https://manta.net/en/series/if-it-s-remorse-you-desire?seriesId=3429", Settings: &Settings{}})
	if ok, _ := m.Test(); !ok || m.id != "3429" {
		t.Fatalf("Test() didn't read the series ID: %q", m.id)
	}
	if title, err := m.FetchTitle(); err != nil || title != "If It's Remorse You Desire" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 || chapters[0].(*MantaChapter).Locked || !chapters[1].(*MantaChapter).Locked {
		t.Fatalf("FetchChapters() = %+v, %v", chapters, errs)
	}
	ch, err := m.FetchChapter(chapters[0])
	if err != nil || len(ch.Pages) != 2 || ch.Pages[0].URL != "https://si.manta.test/a.jpg?sign=1" {
		t.Errorf("FetchChapter() = %+v, %v", ch, err)
	}
}

func TestComicsKingdom(t *testing.T) {
	var strips []string
	for i := range wpPerPage {
		strips = append(strips, `{"id":`+strconv.Itoa(i)+`,"title":{"rendered":"2014-07-`+strconv.Itoa(i)+`"},"assets":{"featured":{"url":"https://wp.ck.test/`+strconv.Itoa(i)+`.jpg"}}}`)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ck_feature":
			w.Write([]byte(`[{"title":{"rendered":"Intelligent Life"}}]`))
		case r.URL.Path == "/ck_comic" && r.URL.Query().Get("ck_feature") == "intelligent":
			switch r.URL.Query().Get("page") {
			case "1":
				w.Write([]byte("[" + strings.Join(strips, ",") + "]"))
			case "2":
				w.Write([]byte(`[{"id":500,"title":{"rendered":"2026-10-16"},"assets":{"featured":{"url":"https://wp.ck.test/last.jpg"}}}]`))
			default:
				http.Error(w, `{"code":"rest_post_invalid_page_number"}`, http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := comicsKingdomAPI
	comicsKingdomAPI = srv.URL + "/"
	t.Cleanup(func() { comicsKingdomAPI = old })

	c := NewComicsKingdom(&Grabber{URL: "https://wp.comicskingdom.com/intelligent", Settings: &Settings{}})
	if ok, _ := c.Test(); !ok || c.slug != "intelligent" {
		t.Fatalf("Test() didn't read the slug: %q", c.slug)
	}
	if title, err := c.FetchTitle(); err != nil || title != "Intelligent Life" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := c.FetchChapters()
	if len(errs) > 0 || len(chapters) != wpPerPage+1 {
		t.Fatalf("FetchChapters() gave %d chapters, %v", len(chapters), errs)
	}
	last := chapters[wpPerPage]
	if ChapterLabel(last) != "Chapter 101 - 2026-10-16" {
		t.Errorf("last chapter = %q", ChapterLabel(last))
	}
	ch, err := c.FetchChapter(last)
	if err != nil || ch.Pages[0].URL != "https://wp.ck.test/last.jpg" {
		t.Errorf("FetchChapter() = %+v, %v", ch, err)
	}
}
