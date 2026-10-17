// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"testing"
)

func TestComiciTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://comic-growl.com/series/20ee9953c2ef2", true},
		{"https://comic-growl.com/episodes/12de0df236120", true},
		{"https://shonenjumpplus.com/episode/123", false},
	}
	for _, tt := range tests {
		got, _ := NewComici(&Grabber{URL: tt.url}).Test()
		if got != tt.want {
			t.Errorf("Test(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestComiciTitle(t *testing.T) {
	tests := []struct {
		raw    string
		title  string
		number float64
		part   string
	}{
		{"1", "", 1, ""},
		{"2-①", "", 2, "1"},
		{"24‐②", "", 24, "2"},
		{"１２", "", 12, ""},
		{"3.5 番外編", "番外編", 3.5, ""},
		{"第3話 タイトル", "タイトル", 3, ""},
	}
	for _, tt := range tests {
		title, number, part := comiciTitle(tt.raw, "Series")
		if title != tt.title || number != tt.number || part != tt.part {
			t.Errorf("comiciTitle(%q) = %q, %v, %q; want %q, %v, %q", tt.raw, title, number, part, tt.title, tt.number, tt.part)
		}
	}
}

func TestComiciTiles(t *testing.T) {
	if tiles, err := comiciTiles(""); tiles != nil || err != nil {
		t.Errorf("empty scramble = %v, %v", tiles, err)
	}
	if _, err := comiciTiles("[1, 2, 3]"); err == nil {
		t.Error("a short scramble was accepted")
	}
	tiles, err := comiciTiles("[13, 6, 15, 11, 10, 12, 5, 8, 2, 7, 4, 1, 0, 14, 3, 9]")
	if err != nil || len(tiles) != 16 || tiles[0] != 13 {
		t.Errorf("scramble = %v, %v", tiles, err)
	}
}

// comiciTileColor is the solid color of tile i in the unscrambled test page
func comiciTileColor(i int) color.RGBA {
	return color.RGBA{R: uint8(i * 15), G: uint8(255 - i*15), B: uint8(i * 7), A: 255}
}

func TestComiciUnscramble(t *testing.T) {
	const tile = 16
	tiles := []int{13, 6, 15, 11, 10, 12, 5, 8, 2, 7, 4, 1, 0, 14, 3, 9}
	// tile i of the page sits at tiles[i] in the scrambled image; the extra
	// column on the right is outside the grid and must stay put
	scrambled := image.NewRGBA(image.Rect(0, 0, 4*tile+3, 4*tile))
	draw.Draw(scrambled, scrambled.Bounds(), &image.Uniform{color.RGBA{0, 0, 255, 255}}, image.Point{}, draw.Src)
	for to, from := range tiles {
		at := image.Pt(from/4*tile, from%4*tile)
		draw.Draw(scrambled, image.Rectangle{Min: at, Max: at.Add(image.Pt(tile, tile))}, &image.Uniform{comiciTileColor(to)}, image.Point{}, draw.Src)
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, scrambled); err != nil {
		t.Fatal(err)
	}

	out, err := comiciUnscramble(buf.Bytes(), tiles)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	near := func(a, b uint32) bool { d := int(a>>8) - int(b>>8); return d >= -12 && d <= 12 }
	for i := range 16 {
		r, g, b, _ := got.At(i/4*tile+tile/2, i%4*tile+tile/2).RGBA()
		wr, wg, wb, _ := comiciTileColor(i).RGBA()
		if !near(r, wr) || !near(g, wg) || !near(b, wb) {
			t.Errorf("tile %d = %d,%d,%d, want %d,%d,%d", i, r>>8, g>>8, b>>8, wr>>8, wg>>8, wb>>8)
		}
	}
	if _, _, b, _ := got.At(4*tile+1, tile).RGBA(); b>>8 < 200 {
		t.Errorf("the strip past the grid moved")
	}
}

func TestComiciFetch(t *testing.T) {
	mux := http.NewServeMux()
	series := `{"series":{"summary":{"id":"20ee9953c2ef2","name":"カードファイト!! ヴァンガード ターナバウト"},"episodes":[{"id":"12de0df236120","title":"1"},{"id":"1788b8b34b9dc","title":"2-①"}]}}`
	respond(mux, "/api/episodes", series)
	respond(mux, "/api/series/access", `{"seriesAccess":{"episodeAccesses":[{"episodeId":"12de0df236120","hasAccess":true,"accessType":"trial"},{"episodeId":"1788b8b34b9dc","hasAccess":false,"accessType":"waitAndFree"}]}}`)
	respond(mux, "/api/episodes/12de0df236120", `{"episode":{"contentId":26,"series":{"id":"20ee9953c2ef2"},"content":[{"type":"viewer","viewerId":"ee1585736994725ec614ee3a3347f1b6"}]}}`)
	mux.HandleFunc("/api/book/contentsInfo", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("comici-viewer-id") != "ee1585736994725ec614ee3a3347f1b6" || r.URL.Query().Get("contentId") != "26" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("page-to") == "0" {
			_, _ = w.Write([]byte(`{"totalPages":2,"result":[{"imageUrl":"https://viewer.comic-growl.com/book/x/01.jpg","scramble":"[13, 6, 15, 11, 10, 12, 5, 8, 2, 7, 4, 1, 0, 14, 3, 9]","sort":0}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"totalPages":2,"result":[{"imageUrl":"https://viewer.comic-growl.com/book/x/02.jpg","scramble":"[1, 14, 12, 6, 0, 9, 10, 4, 2, 8, 11, 3, 7, 5, 15, 13]","sort":1},{"imageUrl":"https://viewer.comic-growl.com/book/x/01.jpg","scramble":"[13, 6, 15, 11, 10, 12, 5, 8, 2, 7, 4, 1, 0, 14, 3, 9]","sort":0}]}`))
	})
	srv := serve(t, mux)

	// a pasted episode link finds its series through the episode's details
	c := NewComici(&Grabber{URL: srv.URL + "/episodes/12de0df236120", Settings: &Settings{}})
	title, err := c.FetchTitle()
	if err != nil || title != "カードファイト!! ヴァンガード ターナバウト" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	if c.URL != srv.URL+"/series/20ee9953c2ef2" {
		t.Errorf("URL = %q", c.URL)
	}

	chapters, errs := c.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(chapters))
	}
	free, paid := chapters[0].(*ComiciChapter), chapters[1].(*ComiciChapter)
	if free.Locked || free.Number != 1 || free.ID != "12de0df236120" || free.Language != "ja" {
		t.Errorf("free episode = %+v", free.Chapter)
	}
	if !paid.Locked || paid.Number != 2 || ChapterLabel(paid) != "Chapter 2 Part 1" {
		t.Errorf("paid episode = %+v (%q)", paid.Chapter, ChapterLabel(paid))
	}

	chapter, err := c.FetchChapter(free)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[0].URL != "https://viewer.comic-growl.com/book/x/01.jpg" || chapter.Pages[0].Transform == nil {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}
