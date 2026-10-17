// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// The Init Manga WordPress theme (mangadrama.com) lists every chapter as a
// UIkit button on the series page. Its reader ships the chapter's HTML
// encrypted in an InitMangaEncryptedChapter script, which the theme's own
// script decrypts with the page's InitMangaData.decryption_key (see
// initMangaImages).
func init() {
	plainHTMLSelectors = append(plainHTMLSelectors, SiteSelector{
		Title:        "h1",
		Rows:         `a.uk-button.uk-text-initcase[href*="/chapter-"]`,
		Chapter:      ".uk-flex-none",
		ChapterTitle: ".uk-flex-none",
		Image:        "#chapter-content img",
	})
}

var (
	initMangaChapterRe = regexp.MustCompile(`var InitMangaEncryptedChapter = (\{.*?\});`)
	initMangaKeyRe     = regexp.MustCompile(`"decryption_key":"([^"]+)"`)
)

// initMangaImages returns the page images of an Init Manga reader; ok is false
// for any other page. The chapter's HTML is AES-256-CBC encrypted with a key
// derived (PBKDF2, SHA-512, 999 rounds) from the base64 decryption_key.
func initMangaImages(doc *goquery.Document) (urls []string, ok bool, err error) {
	var script, data string
	doc.Find("script").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := s.Text()
		if strings.Contains(text, "InitMangaEncryptedChapter") {
			script = text
		}
		if strings.Contains(text, `"decryption_key"`) {
			data = text
		}
		return script == "" || data == ""
	})
	if script == "" {
		return nil, false, nil
	}
	m := initMangaChapterRe.FindStringSubmatch(script)
	k := initMangaKeyRe.FindStringSubmatch(data)
	if m == nil || k == nil {
		return nil, true, errors.New("the encrypted chapter or its key is missing")
	}
	html, err := decryptInitManga(m[1], k[1])
	if err != nil {
		return nil, true, err
	}
	content, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, true, err
	}
	content.Find("img").Each(func(_ int, s *goquery.Selection) {
		u := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", "")))
		if u != "" && !strings.HasPrefix(u, "data:") {
			urls = append(urls, u)
		}
	})
	return urls, true, nil
}

// decryptInitManga decrypts the InitMangaEncryptedChapter JSON blob into the
// chapter's HTML
func decryptInitManga(blob, key string) (string, error) {
	var enc struct {
		Ciphertext string `json:"ciphertext"`
		IV         string `json:"iv"`
		Salt       string `json:"salt"`
	}
	if err := json.Unmarshal([]byte(blob), &enc); err != nil {
		return "", err
	}
	pass, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", err
	}
	ct, err := base64.StdEncoding.DecodeString(enc.Ciphertext)
	if err != nil {
		return "", err
	}
	salt, err := hex.DecodeString(enc.Salt)
	if err != nil {
		return "", err
	}
	iv, err := hex.DecodeString(enc.IV)
	if err != nil {
		return "", err
	}
	derived, err := pbkdf2.Key(sha512.New, string(pass), salt, 999, 32)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return "", err
	}
	if len(ct) == 0 || len(ct)%aes.BlockSize != 0 || len(iv) != aes.BlockSize {
		return "", errors.New("invalid ciphertext")
	}
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ct)
	pad := int(out[len(out)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(out) {
		return "", errors.New("invalid padding, wrong key?")
	}
	return string(out[:len(out)-pad]), nil
}
