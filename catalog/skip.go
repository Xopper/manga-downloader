// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package catalog

import (
	"os"
	"path/filepath"
	"strings"
)

// skipped are the sites left out of the catalog: not manga, though a
// Tachiyomi source lists them. They're not crawled, searched or listed,
// and their files are dropped from a catalog dir. Their grabbers, when they
// have one, still read a pasted link.
var skipped = map[string]bool{}

func init() {
	for _, group := range [][]string{
		// photo, cosplay and model galleries
		{"3600000.xyz", "4khd.com", "ahottie.top", "baobua.net", "buondua.com", "cosplaytele.com",
			"elitebabes.com", "en.girlstop.info", "en.xchina.co", "everia.club", "everiaclub.com",
			"femjoyhunter.com", "foamgirl.net", "ftvhunter.com", "hentai-cosplay-xxx.com", "jjcos.com",
			"joymiihub.com", "kiutaku.com", "meijuntu.com", "metarthunter.com", "misskon.com", "mitaku.net",
			"ososedki.com", "photos18.com", "pmatehunter.com", "pornpics.com", "simply-cosplay.com",
			"v2ph.com", "xarthunter.com", "xasiat.com", "xgmn8.vip", "xiutaku.com"},
		// copies of creators' paid posts, image boards and art sites
		{"pawchive.pw", "kemono.cr", "coomer.st", "e621.net", "danbooru.donmai.us", "deviantart.com"},
		// sources for a server of one's own, not sites
		{"127.0.0.1", "localhost", "monochromecms.netlify.app"},
		// western comic strips and webcomics
		{"blastwave-comic.com", "ccc.vinnieveritas.com", "clowncorps.net", "collectedcurios.com",
			"comicaurora.com", "comicfury.com", "commitstrip.com", "darklegacycomics.com",
			"darthsanddroids.net", "dresdencodak.com", "egscomics.com", "elan.school",
			"existentialcomics.com", "explosm.net", "giantitp.com", "grrlpowercomic.com", "gunnerkrigg.com",
			"hiveworkscomics.com", "incase.buttsmithy.com", "irovedout.com", "jolleycomics.com",
			"killsixbilliondemons.com", "loadingartist.com", "megatokyo.com", "mehgazone.com", "oglaf.com",
			"ohjoysextoy.com", "patchfriday.com", "peppercarrot.com", "politeandgood.com",
			"questionablecontent.net", "randowis.com", "reallifecomics.com", "sabrina-online.com",
			"sandraandwoo.com", "schlockmercenary.com", "smbc-comics.com", "snafu-comics.com",
			"supermegacomics.com", "swordscomic.com", "theduckwebcomics.com", "twokinds.keenspot.com",
			"vixenlogic.com", "wp.comicskingdom.com", "xkcd.com", "xrabohrok.github.io"},
		// US comic books
		{"batcave.biz", "comichubfree.com", "digitalcomicmuseum.com", "readallcomics.com",
			"readcomiconline.li", "readcomicsonline.ru", "xoxocomic.com"},
		// games' story comics
		{"universe.leagueoflegends.com", "play.mobilelegends.com"},
	} {
		for _, site := range group {
			skipped[site] = true
		}
	}
}

// Skipped reports whether site (a host, www. and case ignored) is left out
// of the catalog
func Skipped(site string) bool {
	return skipped[strings.TrimPrefix(strings.ToLower(strings.TrimSpace(site)), "www.")]
}

// skippedFile reports whether a catalog file name (<site>.json, plain or
// gzipped) is a skipped site's
func skippedFile(name string) bool {
	return Skipped(strings.TrimSuffix(strings.TrimSuffix(name, ".gz"), ".json"))
}

// dropSkipped deletes the catalog files of the skipped sites from the dir,
// plain and gzipped (Load never reads them)
func (s *Store) dropSkipped() {
	for _, pattern := range []string{"*.json", "*.json.gz"} {
		names, _ := filepath.Glob(filepath.Join(s.Dir, pattern))
		for _, name := range names {
			if base := filepath.Base(name); isCatalogFile(base) && skippedFile(base) {
				_ = os.Remove(name)
			}
		}
	}
}
