// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// toonzFlightHTML wraps an RSC stream the way Next.js streams it, split in
// two push() chunks
func toonzFlightHTML(t *testing.T, stream string) string {
	t.Helper()
	half := len(stream) / 2
	html := "<html><body>"
	for _, part := range []string{stream[:half], stream[half:]} {
		raw, err := json.Marshal(part)
		if err != nil {
			t.Fatal(err)
		}
		html += `<script>self.__next_f.push([1,` + string(raw) + `])</script>`
	}
	return html + "</body></html>"
}

// toonzSeriesStream is a trimmed series page payload: the comic's ids and
// title sit in a client component's props
const toonzSeriesStream = `45:["$","$L4b",null,{"comicId":1078007,"comicSlug":"ill-dream-of-you","comicType":"manhwa","comicTitle":"I'll dream of \"you\"","isAuthenticated":false}]` + "\n"

// toonzReaderStream is a trimmed reader page payload, with a neighbour
// chapter mentioned before the one being read
const toonzReaderStream = `5:{"next":{"chapter":{"id":13409415,"images":[]}},"chapter":{"id":13409414,"comicId":1078007,"chapterNumber":"2.00","images":[` +
	`{"id":2,"filename":"002.webp","sortOrder":2},` +
	`{"id":1,"filename":"chapters/1078007/13409414/001-01.webp","sortOrder":1}]}}` + "\n"

func TestToonzResolveSeriesURL(t *testing.T) {
	cases := []struct{ input, want string }{
		{"https://toonz.to/manhwa/ill-dream-of-you/chapter/0.01", "https://toonz.to/manhwa/ill-dream-of-you"},
		{"https://toonz.to/manga/chainsaw-man/chapter/12?x=1#top", "https://toonz.to/manga/chainsaw-man"},
		{"https://toonz.to/manhwa/ill-dream-of-you", "https://toonz.to/manhwa/ill-dream-of-you"},
		{"https://toonz.to/manhwa/browse", "https://toonz.to/manhwa/browse"},
	}
	for _, c := range cases {
		g := &Toonz{Grabber: &Grabber{URL: c.input}}
		g.resolveSeriesURL()
		if g.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, g.URL, c.want)
		}
	}
}

func TestToonzTest(t *testing.T) {
	for url, want := range map[string]bool{
		"https://toonz.to/manhwa/ill-dream-of-you": true,
		"https://www.toonz.to/manga/x":             true,
		"https://nottoonz.to/manga/x":              false,
	} {
		if got, _ := (&Toonz{Grabber: &Grabber{URL: url}}).Test(); got != want {
			t.Errorf("Test(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestToonzFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manhwa/ill-dream-of-you", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(toonzFlightHTML(t, toonzSeriesStream)))
	})
	mux.HandleFunc("/api/comics/1078007/chapters", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"chapters":[
			{"id":13409414,"chapterNumber":"2.00","title":null,"slug":"chapter-2","pageCount":2},
			{"id":13409412,"chapterNumber":"0.01","title":"Prologue.","slug":"chapter-0.01","pageCount":23}]}`))
	})
	mux.HandleFunc("/manhwa/ill-dream-of-you/chapter/2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(toonzFlightHTML(t, toonzReaderStream)))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	g := NewToonz(&Grabber{URL: srv.URL + "/manhwa/ill-dream-of-you/chapter/2", Settings: &Settings{}})
	title, err := g.FetchTitle()
	if err != nil || title != `I'll dream of "you"` {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := g.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v", len(chapters), errs)
	}
	second := chapters[1].(*ToonzChapter)
	if second.Number != 0.01 || second.Title != "Prologue." || second.URL != srv.URL+"/manhwa/ill-dream-of-you/chapter/0.01" {
		t.Errorf("chapter[1] = %v %q %q", second.Number, second.Title, second.URL)
	}

	chapter, err := g.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		srv.URL + "/uploads/chapters/1078007/13409414/001-01.webp",
		srv.URL + "/uploads/chapters/1078007/13409414/002.webp",
	}
	if len(chapter.Pages) != len(want) {
		t.Fatalf("got %d pages, want %d", len(chapter.Pages), len(want))
	}
	for i, p := range chapter.Pages {
		if p.URL != want[i] {
			t.Errorf("page %d = %q, want %q", i+1, p.URL, want[i])
		}
	}
}
