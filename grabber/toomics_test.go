// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToomicsTest(t *testing.T) {
	cases := map[string]bool{
		"https://global.toomics.com/en/webtoon/episode/toon/6723/search/Y":        true,
		"https://global.toomics.com/en/webtoon/detail/code/173364/ep/1/toon/6723": true,
		"https://global.toomics.com/en/webtoon/ranking":                           false,
		"https://example.com/en/webtoon/episode/toon/6723":                        false,
	}
	for u, want := range cases {
		if got, _ := NewToomics(&Grabber{URL: u}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestToomicsFetch(t *testing.T) {
	// trimmed copies of a title's episode list (a free, a read and a VIP
	// episode) and a reader
	row := func(onclick, number, coin string) string {
		return fmt.Sprintf(`<li class="normal_ep"><a href="javascript:;" onclick="Webtoon.chkec(this);%s">`+
			`<div class="cell-num"><span class="num">%s</span></div><div class="cell-title"><strong class="line-3"> Home of Geeks </strong></div>`+
			`<div class="cell-coin"><span class="%s">x</span></div></a></li>`, onclick, number, coin)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/en/webtoon/episode/toon/6723":
			fmt.Fprint(w, `<h2 class="line-clamp-2 font-bold"> Home of Geeks </h2><ul>`+
				row(`location.href='/en/webtoon/detail/code/173364/ep/1/toon/6723'`, "1", "coin-type1")+
				row(`location.href='/en/webtoon/detail/code/173365/ep/2/toon/6723'`, "2", "coin-type6")+
				row(`Base.popupForStat('modal-login', 'login', '/en/webtoon/detail/code/173368/ep/5/toon/6723', 'N', 'B', '')`, "5", "coin-type5")+`</ul>`)
		case "/en/webtoon/detail/code/173364/ep/1/toon/6723":
			fmt.Fprint(w, `<div id="load_image_0"><img id="set_image_0" src="data:image/png;base64,AA" data-src="https://toon.example.com/a?time=1"></div>`+
				`<div id="load_image_1"><img id="set_image_1" src="data:image/png;base64,AA" data-src="https://toon.example.com/b?time=1"></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tm := NewToomics(&Grabber{URL: srv.URL + "/en/webtoon/episode/toon/6723/search/Y", Settings: &Settings{}})
	title, err := tm.FetchTitle()
	if err != nil || title != "Home of Geeks" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := tm.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	if LockedOf(chapters[0]) || LockedOf(chapters[1]) || !LockedOf(chapters[2]) || chapters[2].GetNumber() != 5 {
		t.Errorf("locked = %v %v %v", LockedOf(chapters[0]), LockedOf(chapters[1]), LockedOf(chapters[2]))
	}
	chapter, err := tm.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != "https://toon.example.com/b?time=1" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
