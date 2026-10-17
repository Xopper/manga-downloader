// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"
)

func TestMadaraForkImages(t *testing.T) {
	cases := map[string]string{
		"cartoonporn": `<html><body><div class="reader-pages">
<figure class="reader-page active" data-index="1" data-page-no="1"><img class="manga-img" referrerpolicy="no-referrer" src="https://cartoonporn.to/wp-content/uploads/WP-manga/data/manga_1/abc/image-1.jpg" alt="Page 1" /></figure>
<figure class="reader-page" data-index="2" data-page-no="2"><img class="manga-img" referrerpolicy="no-referrer" src="https://cartoonporn.to/wp-content/uploads/WP-manga/data/manga_1/abc/image-2.jpg" alt="Page 2" /></figure>
</div></body></html>`,
		"hentai4free": `<html><body><section></section>
<script type="application/json" id="h4f-r2-data">{"images":[{"id":1,"src":"https://cartoonporn.to/wp-content/uploads/WP-manga/data/manga_1/abc/image-1.jpg","srcset":"","alt":"","width":0,"height":0},{"id":2,"src":"https://cartoonporn.to/wp-content/uploads/WP-manga/data/manga_1/abc/image-2.jpg","srcset":"","alt":"","width":0,"height":0}]}</script>
</body></html>`,
	}
	want := "https://cartoonporn.to/wp-content/uploads/WP-manga/data/manga_1/abc/image-1.jpg https://cartoonporn.to/wp-content/uploads/WP-manga/data/manga_1/abc/image-2.jpg"
	for name, html := range cases {
		if got := strings.Join(madaraForkImages(docFromHTML(t, html)), " "); got != want {
			t.Errorf("%s: madaraForkImages() = %q, want %q", name, got, want)
		}
	}
}
