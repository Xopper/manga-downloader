// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"testing"
)

func TestLustoonResolveSeriesURL(t *testing.T) {
	cases := []struct{ input, want string }{
		{"https://lustoon.com/comic/love-upset/chapter-20", "https://lustoon.com/comic/love-upset"},
		{"https://lustoon.com/comic/love-upset/chapter-20/?a=1", "https://lustoon.com/comic/love-upset"},
		{"https://lustoon.com/comic/love-upset", "https://lustoon.com/comic/love-upset"},
		{"https://lustoon.com/comics", "https://lustoon.com/comics"},
	}
	for _, c := range cases {
		g := &Lustoon{Grabber: &Grabber{URL: c.input}}
		g.resolveSeriesURL()
		if g.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, g.URL, c.want)
		}
	}
}

func TestLustoonParseSerie(t *testing.T) {
	stream := `4:[["$","$L17",null,{"bookmark":false,"serie":{"id":37,"name":"LOVE UPSET","slug":"love-upset","idioma":"es","chapters":[` +
		`{"num":23,"name":null,"slug":"chapter-23","id":1379,"patreon":0},` +
		`{"num":22.5,"name":"Extra","slug":"chapter-22-5","id":1345,"patreon":0}]}}]]` + "\n"
	serie, err := lustoonParseSerie(toonzFlightHTML(t, stream))
	if err != nil {
		t.Fatal(err)
	}
	if serie.Name != "LOVE UPSET" || len(serie.Chapters) != 2 || serie.Chapters[1].Num != 22.5 || *serie.Chapters[1].Name != "Extra" {
		t.Errorf("serie = %+v", serie)
	}
}

func TestLustoonPages(t *testing.T) {
	inline := `4:["$","$L19",null,{"chapter":{"id":1225,"pageches":{"urlImg":"[\"http://media.example/00.jpg\",\"https://media.example/01.jpg\"]"}}}]` + "\n"
	// long lists come as a text row ("1a:T{hex length},{text}") referenced
	// by "$1a"
	list := `["http://media.example/00.jpg","http://media.example/01.jpg"]`
	ref := `4:["$","$L19",null,{"chapter":{"id":1379,"pageches":{"urlImg":"$1a","chapterId":1379}}}]` + "\n" +
		fmt.Sprintf("1a:T%x,%s", len(list), list) + `5:["$","div",null,{}]` + "\n"

	for name, stream := range map[string]string{"inline": inline, "text row": ref} {
		images, err := lustoonPages(toonzFlightHTML(t, stream))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := []string{"https://media.example/00.jpg", "https://media.example/01.jpg"}
		if len(images) != len(want) || images[0] != want[0] || images[1] != want[1] {
			t.Errorf("%s: images = %v, want %v", name, images, want)
		}
	}
}
