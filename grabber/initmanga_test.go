// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
)

// initMangaEncrypt encrypts html the way the Init Manga theme does, so the
// test reader page holds a blob the decryption must undo
func initMangaEncrypt(t *testing.T, html, password string) string {
	t.Helper()
	salt := bytes.Repeat([]byte{0xc2}, 30)
	iv := bytes.Repeat([]byte{0x0f}, aes.BlockSize)
	key, err := pbkdf2.Key(sha512.New, password, salt, 999, 32)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	pad := aes.BlockSize - len(html)%aes.BlockSize
	plain := append([]byte(html), bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, plain)
	return fmt.Sprintf(`{"ciphertext":%q,"iv":%q,"salt":%q}`,
		base64.StdEncoding.EncodeToString(ct), hex.EncodeToString(iv), hex.EncodeToString(salt))
}

func TestInitManga(t *testing.T) {
	const password = "00ACCC5AF6B348AB"
	content := `<img src="https://cdn.mangadrama.com/chapters/28468/73/a.jpg" alt=""><img data-src="https://cdn.mangadrama.com/chapters/28468/73/b.jpg" src="data:image/gif;base64,R0lGOD" alt="">`
	reader := `<html><body class="single-chapter">
<script>var InitMangaData = {"nonce":"6b44643b99","decryption_key":"` + base64.StdEncoding.EncodeToString([]byte(password)) + `","readerLayout":"default"};</script>
<h1 class="uk-h2">Chapter 73: Chapter 73</h1>
<div id="chapter-content" class="uk-article image-based"><div class="imc-skeleton-line"></div><script>var InitMangaEncryptedChapter = ` + initMangaEncrypt(t, content, password) + `;</script></div>
</body></html>`

	checkPlainHTMLEntry(t, plainHTMLEntryCase{
		series: `<html><body><h1 class="uk-h2">
    Isekai Ryouridou</h1>
<div class="uk-position-relative"><a class="uk-button uk-button-default uk-button-large uk-width-1-1 uk-text-initcase uk-border-rounded" href="https://mangadrama.com/manga/isekai-ryouridou/chapter-73/"><div class="uk-flex uk-flex-middle"><div class="uk-flex-none">Chapter 73</div><div class="uk-width-expand"></div><div class="uk-text-meta">2 months</div></div></a></div>
<div class="uk-position-relative"><a class="uk-button uk-button-default uk-button-large uk-width-1-1 uk-text-initcase uk-border-rounded" href="https://mangadrama.com/manga/isekai-ryouridou/chapter-72/"><div class="uk-flex uk-flex-middle"><div class="uk-flex-none">Chapter 72</div><div class="uk-width-expand"></div><div class="uk-text-meta">2 months</div></div></a></div>
</body></html>`,
		reader:    reader,
		wantRows:  `a.uk-button.uk-text-initcase[href*="/chapter-"]`,
		wantTitle: "Isekai Ryouridou",
		wantNums:  []float64{73, 72},
		wantPages: []string{
			"https://cdn.mangadrama.com/chapters/28468/73/a.jpg",
			"https://cdn.mangadrama.com/chapters/28468/73/b.jpg",
		},
	})
}

func TestDecryptInitMangaWrongKey(t *testing.T) {
	blob := initMangaEncrypt(t, "<p>hello</p>", "right")
	if html, err := decryptInitManga(blob, base64.StdEncoding.EncodeToString([]byte("wrong"))); err == nil && html == "<p>hello</p>" {
		t.Error("decryptInitManga() decrypted with the wrong key")
	}
}
