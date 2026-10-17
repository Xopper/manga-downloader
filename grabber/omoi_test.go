// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOmoiFetch(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	// trimmed copies of the Azuki API's answers
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("azuki-organization-key") != omoiOrganizationKey {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/manga/slug/chihayafuru/v0":
			fmt.Fprint(w, `{"uuid":"ca91","slug":"chihayafuru","name":"Chihayafuru"}`)
		case "/mangas/ca91/chapters/v4":
			fmt.Fprintf(w, `{"chapters":[`+
				`{"uuid":"c1","order_number":100,"label":"1","title":null,"free_published_date":%q,"free_unpublished_date":null,"is_upcoming":false},`+
				`{"uuid":"c2","order_number":200,"label":"1.5","title":"Extra","free_published_date":%q,"free_unpublished_date":%q,"is_upcoming":false},`+
				`{"uuid":"c3","order_number":5002,"label":"247b","title":null,"free_published_date":null,"free_unpublished_date":null,"is_upcoming":false}]}`, past, past, past)
		case "/chapters/c1/pages/v1":
			fmt.Fprint(w, `{"data":{"pages":[{"image":{"webp":[{"url":"https://img.example.com/p1/800.webp","width":800},{"url":"https://img.example.com/p1/1600.webp","width":1600}]}}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := omoiAPI
	omoiAPI = srv.URL
	defer func() { omoiAPI = old }()

	o := NewOmoi(&Grabber{URL: "https://www.omoi.com/series/chihayafuru", Settings: &Settings{}})
	if ok, _ := o.Test(); !ok {
		t.Fatal("omoi.com not recognised")
	}
	title, err := o.FetchTitle()
	if err != nil || title != "Chihayafuru" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := o.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d, %v", len(chapters), errs)
	}
	locked := []bool{false, true, true}
	numbers := []float64{1, 1.5, 247}
	for i, c := range chapters {
		if LockedOf(c) != locked[i] || c.GetNumber() != numbers[i] {
			t.Errorf("chapter %d = %v locked %v", i, c.GetNumber(), LockedOf(c))
		}
	}
	if got := ChapterLabel(chapters[2]); got != "Chapter 247b" {
		t.Errorf("label = %q", got)
	}
	chapter, err := o.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 1 || chapter.Pages[0].URL != "https://img.example.com/p1/1600.webp" || chapter.Pages[0].Transform == nil {
		t.Fatalf("pages = %+v", chapter.Pages)
	}
	// "RIFF" arrives as fc e7 e8 e8
	got, _ := chapter.Pages[0].Transform([]byte{0xfc, 0xe7, 0xe8, 0xe8})
	if string(got) != "RIFF" {
		t.Errorf("decoded = %q", got)
	}
}
