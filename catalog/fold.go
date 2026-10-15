// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fold normalises a title or query for matching: lowercase, compatibility
// decomposition (full-width and half-width forms become plain ones), accents
// dropped, anything that isn't a letter or digit turned into a space, spaces
// collapsed. Japanese voicing marks (dakuten) are kept, so が stays が.
func Fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := true
	for _, r := range norm.NFKD.String(s) {
		if unicode.Is(unicode.Mn, r) && r != 0x3099 && r != 0x309A {
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.Is(unicode.Mn, r) {
			if !space {
				b.WriteByte(' ')
				space = true
			}
			continue
		}
		b.WriteRune(unicode.ToLower(r))
		space = false
	}

	return norm.NFC.String(strings.TrimSpace(b.String()))
}
