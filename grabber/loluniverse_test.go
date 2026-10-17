// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

// TestLolUniverseParseURL splits series, issue and one-shot URLs alike
func TestLolUniverseParseURL(t *testing.T) {
	cases := []struct {
		in, locale, first, second string
		ok                        bool
	}{
		{"https://universe.leagueoflegends.com/en_us/comic/zed", "en_us", "zed", "", true},
		{"https://universe.leagueoflegends.com/en_us/comic/zed/issue-2/", "en_us", "zed", "issue-2", true},
		{"https://universe.leagueoflegends.com/ES_ES/comic/nami/intotheabyss?x#y", "es_es", "nami", "intotheabyss", true},
		{"https://universe.leagueoflegends.com/en_us/champion/zed/", "", "", "", false},
	}
	for _, c := range cases {
		locale, first, second, ok := lolUniverseParseURL(c.in)
		if ok != c.ok || locale != c.locale || first != c.first || second != c.second {
			t.Errorf("lolUniverseParseURL(%q) = %q %q %q %v", c.in, locale, first, second, ok)
		}
	}
}

// TestLolUniverseIssues numbers issues by index and recognises their ids
func TestLolUniverseIssues(t *testing.T) {
	issues := &lolUniverseIssues{}
	if err := json.Unmarshal([]byte(`{"name":"Zed","issues":[
{"title":"Issue #1","index":1,"url":"/en_us/comic/zed/issue-1"},
{"title":"Issue #2","index":2,"url":"/en_us/comic/zed/issue-2"}]}`), issues); err != nil {
		t.Fatal(err)
	}
	chapters := lolUniverseChapters(issues, "en")
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters", len(chapters))
	}
	c := chapters[1].(*LolUniverseChapter)
	if c.Number != 2 || c.ComicID != "zed/issue-2" || ChapterLabel(c) != "Issue #2" || c.Language != "en" {
		t.Errorf("chapter = %+v", c)
	}
	if !lolUniverseHasIssue(issues, "zed/issue-1") || lolUniverseHasIssue(issues, "zed/other") {
		t.Error("lolUniverseHasIssue")
	}
}

// TestLolUniversePages flattens spreads, preferring the 2x image
func TestLolUniversePages(t *testing.T) {
	data := &lolUniversePages{}
	if err := json.Unmarshal([]byte(`{"title":"Nami","desktop-pages":[
[{"uri":"https://c/1.jpg","2x":"https://c/1@2x.jpg"}],
[{"uri":"https://c/2.jpg"},{"uri":"https://c/3.jpg","2x":"https://c/3@2x.jpg"}]]}`), data); err != nil {
		t.Fatal(err)
	}
	pages := lolUniversePageList(data)
	want := []string{"https://c/1@2x.jpg", "https://c/2.jpg", "https://c/3@2x.jpg"}
	if len(pages) != len(want) {
		t.Fatalf("pages = %+v", pages)
	}
	for i, w := range want {
		if pages[i].URL != w || pages[i].Number != int64(i+1) {
			t.Errorf("page %d = %+v, want %s", i, pages[i], w)
		}
	}
}
