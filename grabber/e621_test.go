// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"testing"
)

func TestE621PoolID(t *testing.T) {
	cases := map[string]string{
		"https://e621.net/pools/57205":        "57205",
		"https://e621.net/pools/57205?page=2": "57205",
		"https://e926.net/pools/41909/":       "41909",
		"https://e621.net/posts/5137478":      "",
		"https://e621.net/pools?search=x":     "",
	}
	for in, want := range cases {
		got, err := e621PoolID(in)
		if got != want || (want == "") != (err != nil) {
			t.Errorf("e621PoolID(%q) = %q, %v; want %q", in, got, err, want)
		}
		if ok, _ := NewE621(&Grabber{URL: in}).Test(); !ok {
			t.Errorf("Test(%q) = false", in)
		}
	}
}

func TestE621Pages(t *testing.T) {
	str := func(s string) *string { return &s }
	post := func(id int, ext string, u *string) e621Post {
		p := e621Post{ID: id}
		p.File.Ext, p.File.URL = ext, u
		return p
	}
	posts := map[int]e621Post{
		1: post(1, "png", str("https://static1.e621.net/data/1.png")),
		2: post(2, "jpg", nil), // hidden from anonymous visitors
		3: post(3, "webm", str("https://static1.e621.net/data/3.webm")),
		4: post(4, "jpg", str("https://static1.e621.net/data/4.jpg")),
	}
	got := e621Pages([]int{4, 2, 3, 5, 1}, posts)
	want := []string{"https://static1.e621.net/data/4.jpg", "https://static1.e621.net/data/1.png"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("e621Pages() = %v, want %v", got, want)
	}
}
