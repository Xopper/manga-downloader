// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/base64"
	"testing"
)

// A themesia reader whose inline scripts were moved to base64 data: URLs by
// an optimization plugin (scythescans.com): the ts_reader.run call is only in
// the decoded script
func TestTsReaderInDataScript(t *testing.T) {
	script := `ts_reader.run({"post_id":74455,"mode":"full","sources":[{"source":"Server 1","images":["https:\/\/scythescans.com\/wp-content\/uploads\/2026\/06\/001-8.jpg","https:\/\/scythescans.com\/wp-content\/uploads\/2026\/06\/002-8.jpg"]}]});`
	doc := docFromHTML(t, `<html><head>
<script defer src="data:text/javascript;base64,`+base64.StdEncoding.EncodeToString([]byte(`var ajaxurl = "https://scythescans.com/wp-admin/admin-ajax.php";`))+`"></script>
</head><body><div id="readerarea"></div>
<script defer src="data:text/javascript;base64,`+base64.StdEncoding.EncodeToString([]byte(script))+`"></script>
</body></html>`)

	got := getPlainHTMLImageURL("#readerarea img", doc)
	want := []string{
		"https://scythescans.com/wp-content/uploads/2026/06/001-8.jpg",
		"https://scythescans.com/wp-content/uploads/2026/06/002-8.jpg",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("page %d = %q, want %q", i+1, got[i], want[i])
		}
	}
}
