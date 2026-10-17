// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"
)

// TestWarForRayubaURLs tells the map from a round file and other links
func TestWarForRayubaURLs(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		file string
	}{
		{"https://xrabohrok.github.io/WarMap/#/", true, ""},
		{"https://xrabohrok.github.io/WarMap/#/chars", true, ""},
		{"https://raw.githubusercontent.com/xrabohrok/WarMap/master/tools/master-list-8.json", true, "master-list-8.json"},
		{"https://github.com/xrabohrok/WarMap/blob/master/tools/pyre-clash-r5.json", true, "pyre-clash-r5.json"},
		{"https://github.com/xrabohrok/WarMap", false, ""},
		{"https://xrabohrok.github.io/other/", false, ""},
	}
	for _, c := range cases {
		w := NewWarForRayuba(&Grabber{URL: c.in, Settings: &Settings{}})
		ok, _ := w.Test()
		file, _ := w.roundFile()
		if ok != c.ok || file != c.file {
			t.Errorf("%q: Test = %v, file = %q; want %v, %q", c.in, ok, file, c.ok, c.file)
		}
	}
}

// TestWarForRayubaChapters orders a round and chains rounds into one series
func TestWarForRayubaChapters(t *testing.T) {
	round, err := warForRayubaParse(strings.NewReader(`{"title":"War For Rayuba Round 2","chapters":{
"10":{"title":"Ten","volume":"2","groups":{"primary":"/proxy/api/imgur/chapter/B/"}},
"2":{"title":"Two","volume":"2","groups":{"primary":"/proxy/api/imgur/chapter/A/"}},
"x":{"title":"Bad","groups":{"primary":"/proxy/api/imgur/chapter/C/"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	alone := warForRayubaChapters(round, 2, 0, false)
	if len(alone) != 2 || alone[0].GetNumber() != 2 || alone[1].GetNumber() != 10 || ChapterLabel(alone[0]) != "Chapter 2 - Two" {
		t.Errorf("alone = %v %v %q", alone[0].GetNumber(), alone[1].GetNumber(), ChapterLabel(alone[0]))
	}
	chained := warForRayubaChapters(round, 2, 155, true)
	c := chained[1].(*WarForRayubaChapter)
	if c.Number != 157 || c.Volume != 2 || c.Cubari != "/proxy/api/imgur/chapter/B/" || ChapterLabel(c) != "Chapter 157 - Ten" {
		t.Errorf("chained = %+v %q", c, ChapterLabel(c))
	}
}

// TestWarForRayubaPages reads cubari's page list
func TestWarForRayubaPages(t *testing.T) {
	pages, err := warForRayubaPages(strings.NewReader(`[{"description":"","src":"https://i.imgur.com/a.png"},{"description":"","src":""},{"src":"https://i.imgur.com/b.png"}]`))
	if err != nil || len(pages) != 2 || pages[1].URL != "https://i.imgur.com/b.png" || pages[1].Number != 2 {
		t.Errorf("pages = %+v, %v", pages, err)
	}
}
