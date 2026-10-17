// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestAkumaGalleryID(t *testing.T) {
	cases := map[string]string{
		"https://akuma.moe/g/ax4jafid":    "ax4jafid",
		"https://akuma.moe/g/ax4jafid/":   "ax4jafid",
		"https://akuma.moe/g/ax4jafid/12": "ax4jafid",
		"https://akuma.moe/":              "",
	}
	for in, want := range cases {
		got, err := akumaGalleryID(in)
		if got != want || (want == "") != (err != nil) {
			t.Errorf("akumaGalleryID(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if got := akumaPageTitleRe.ReplaceAllString("Page 1 - [Circle] Title [English] — akuma.moe", ""); got != "[Circle] Title [English]" {
		t.Errorf("reader title = %q", got)
	}
	if got := akumaLanguage("[Circle] Title [French] [Digital]"); got != "fr" {
		t.Errorf("akumaLanguage() = %q", got)
	}
}

func TestYabaiPages(t *testing.T) {
	got := yabaiPages(yabaiPageList{
		Root: "https://s2.yabai.si/", Code: 4254056,
		Head: []string{"10", "2"}, Hash: []string{"c2f2004e", "2c63a4a0"}, Rand: []string{"6nxitody", "usxc28aw"}, Type: []string{"jpg", "png"},
	})
	want := []string{"https://s2.yabai.si/4254056/0002-2c63a4a0-usxc28aw.png", "https://s2.yabai.si/4254056/0010-c2f2004e-6nxitody.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("yabaiPages() = %v, want %v", got, want)
	}
	if got := yabaiPages(yabaiPageList{Root: "x", Head: []string{"1"}}); got != nil {
		t.Errorf("yabaiPages(uneven lists) = %v", got)
	}
	if slug, err := yabaiSlug("https://yabai.si/g/x8adeqxs/read"); err != nil || slug != "x8adeqxs" {
		t.Errorf("yabaiSlug() = %q, %v", slug, err)
	}
}

// TestDDoSGuardPass loads the path check.js names from the protected site,
// once per host
func TestDDoSGuardPass(t *testing.T) {
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		if r.URL.Path == "/check.js" {
			w.Write([]byte(`(function(){new Image().src = '/.well-known/ddos-guard/id/abc123'; new Image().src='https://check.ddos-guard.net/set/id/abc123';})()`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/.well-known/ddos-guard/id/") {
			http.SetCookie(w, &http.Cookie{Name: "__ddg2_", Value: "ok"})
		}
	}))
	defer srv.Close()
	old := ddosGuardCheckURL
	ddosGuardCheckURL = srv.URL + "/check.js"
	defer func() { ddosGuardCheckURL = old }()

	ddosGuardPass(srv.URL)
	ddosGuardPass(srv.URL + "/g/x")
	if hits["/check.js"] != 1 || hits["/.well-known/ddos-guard/id/abc123"] != 1 {
		t.Errorf("hits = %v, want check.js and the well-known path once", hits)
	}
}
