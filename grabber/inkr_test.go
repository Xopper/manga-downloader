// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInkrFetch(t *testing.T) {
	// the API answers by the first field asked for, as trimmed copies of
	// icd-api.inkr.com's answers
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query []struct {
			Fields []string `json:"fields"`
			OIDs   []string `json:"oids"`
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.Header.Get("ikc-platform") != "android" || json.Unmarshal(body, &query) != nil || len(query) != 1 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch query[0].Fields[len(query[0].Fields)-1] {
		case "chapterList":
			fmt.Fprint(w, `{"code":0,"data":{"ik-title-2842":{"chapterList":["ik-chapter-160690","ik-chapter-159590"],"name":"Good-Bye, Heron","oid":"ik-title-2842"}}}`)
		case "revenueType":
			fmt.Fprint(w, `{"code":0,"data":{"ik-chapter-159590":{"name":"GOOD-BYE, HERON","oid":"ik-chapter-159590","order":2,"revenueType":"coin-only"},"ik-chapter-160690":{"name":"Free Preview Chapter","oid":"ik-chapter-160690","order":1,"revenueType":"ad"}}}`)
		case "chapterPages":
			fmt.Fprint(w, `{"code":0,"data":{"ik-chapter-160690":{"chapterPages":[{"page":"ik-page-1","url":"https://i.inkr.com/l/2021/4/17/6/3251390-441"}]}}}`)
		}
	}))
	defer srv.Close()
	old := inkrAPI
	inkrAPI = srv.URL
	defer func() { inkrAPI = old }()

	i := NewInkr(&Grabber{URL: "https://comics.inkr.com/ik-title-2842", Settings: &Settings{}})
	if ok, _ := i.Test(); !ok {
		t.Fatal("comics.inkr.com not recognised")
	}
	title, err := i.FetchTitle()
	if err != nil || title != "Good-Bye, Heron" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := i.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 || LockedOf(chapters[0]) || !LockedOf(chapters[1]) || chapters[1].GetNumber() != 2 {
		t.Fatalf("FetchChapters() = %v, %v", chapters, errs)
	}
	chapter, err := i.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(chapter.Pages) != 1 || chapter.Pages[0].URL != "https://i.inkr.com/l/2021/4/17/6/3251390-441/w1600.ikc" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}

func TestInkrDecrypt(t *testing.T) {
	plain := []byte("RIFF\x00\x00\x00\x00WEBPVP8 some picture bytes")
	padded := append(append([]byte{}, plain...), make([]byte, aes.BlockSize-len(plain)%aes.BlockSize)...)
	iv := bytes.Repeat([]byte{7}, aes.BlockSize)
	block, _ := aes.NewCipher(inkrKey)
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
	data := binary.LittleEndian.AppendUint32(nil, uint32(len(plain)))
	data = append(append(data, iv...), encrypted...)

	got, err := inkrDecrypt(data)
	if err != nil || !bytes.Equal(got, plain) {
		t.Errorf("inkrDecrypt() = %q, %v", got, err)
	}
	if _, err := inkrDecrypt([]byte("<html>")); err == nil {
		t.Error("an HTML page decrypted")
	}
}
