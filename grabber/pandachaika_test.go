// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestPandachaikaArchiveID(t *testing.T) {
	cases := []struct {
		url, want string
		ok        bool
	}{
		{"https://panda.chaika.moe/archive/41542", "41542", true},
		{"https://panda.chaika.moe/archive/41542/", "41542", true},
		{"https://panda.chaika.moe/archive/41542/image-data-list/", "41542", true},
		{"https://panda.chaika.moe/gallery/36767/", "", false},
	}
	for _, c := range cases {
		p := NewPandachaika(&Grabber{URL: c.url})
		if ok, _ := p.Test(); !ok {
			t.Errorf("Test(%q) = false", c.url)
		}
		got, err := p.archiveID()
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("archiveID(%q) = %q, %v; want %q", c.url, got, err, c.want)
		}
	}
	if ok, _ := NewPandachaika(&Grabber{URL: "https://example.com/archive/1"}).Test(); ok {
		t.Error("Test() matched another host")
	}
}

// TestPandachaikaZip reads a zip as the grabber does: directory from the tail
// (or fetched apart when the tail is short), pages by name, entries inflated
func TestPandachaikaZip(t *testing.T) {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	files := map[string]string{"b.png": strings.Repeat("png!", 300), "A.jpg": "jpeg data", "info.txt": "x"}
	for _, name := range []string{"b.png", "info.txt", "A.jpg"} {
		method := zip.Deflate
		if name == "A.jpg" {
			method = zip.Store
		}
		f, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(files[name]))
	}
	w.Close()
	archive := buf.Bytes()

	fetch := func(from, to int64) ([]byte, error) { return archive[from : to+1], nil }
	rangeOf := func(p Page) []byte {
		spec := strings.TrimPrefix(p.Headers["Range"], "bytes=")
		from, to, _ := strings.Cut(spec, "-")
		a, _ := strconv.ParseInt(from, 10, 64)
		b, _ := strconv.ParseInt(to, 10, 64)
		if b >= int64(len(archive)) {
			b = int64(len(archive)) - 1
		}
		return archive[a : b+1]
	}

	for _, tailSize := range []int{len(archive), 30} {
		entries, err := pandachaikaDirectory(archive[len(archive)-tailSize:], fetch)
		if err != nil {
			t.Fatalf("tail %d: %v", tailSize, err)
		}
		pages := pandachaikaPages("https://panda.chaika.moe/archive/1/download/", entries)
		if len(pages) != 2 {
			t.Fatalf("tail %d: %d pages, want 2", tailSize, len(pages))
		}
		for i, want := range []string{"A.jpg", "b.png"} {
			got, err := pages[i].Transform(rangeOf(pages[i]))
			if err != nil || string(got) != files[want] {
				t.Errorf("tail %d: page %d = %q, %v; want %s", tailSize, i+1, got, err, want)
			}
		}
	}
}
