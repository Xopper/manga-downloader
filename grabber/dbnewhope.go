// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

const dbNewHopeDomain = "dbnewhope.com"

// dbNewHopeEdition is one language of the fan manga: its folder, chapter
// list page and the language code the labels use
type dbNewHopeEdition struct {
	dir, list, lang, name string
}

// dbNewHopeEditions are the translations the site publishes, each a series of
// its own (as Tachiyomi lists them)
var dbNewHopeEditions = []dbNewHopeEdition{
	{"english", "chapters.php", "en", "English"},
	{"francais", "chapitres.php", "fr", "Français"},
	{"espanol", "capitulos.php", "es", "Español"},
}

// DbNewHope is a grabber for dbnewhope.com, a Dragon Ball fan manga. Each
// language folder has a chapter list of cards linking {n}/01.php; the reader
// shows one page per request (?page=N, counted by its page dropdown) and the
// images have hashed names, so every page document is fetched.
type DbNewHope struct {
	*Grabber
	doc *goquery.Document
}

// DbNewHopeChapter is a card of the chapter list
type DbNewHopeChapter struct {
	Chapter
	URL string
}

// NewDbNewHope returns a new DbNewHope grabber
func NewDbNewHope(g *Grabber) *DbNewHope {
	return &DbNewHope{Grabber: g}
}

// Test matches the site's domain
func (d *DbNewHope) Test() (bool, error) {
	return hostMatchesDomain(d.URL, dbNewHopeDomain), nil
}

// edition finds the language from the URL's first folder, so a chapter link
// (/espanol/1/01.php) or the language's landing page (/espanol.php) loads
// that language's list
func (d *DbNewHope) edition() (dbNewHopeEdition, error) {
	_, path, ok := chapterPath(d.URL)
	if ok {
		first := strings.ToLower(strings.TrimSuffix(strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)[0], ".php"))
		for _, e := range dbNewHopeEditions {
			if first == e.dir {
				return e, nil
			}
		}
	}
	return dbNewHopeEdition{}, errSeriesNotFound(d.URL)
}

// series fetches the edition's chapter list once
func (d *DbNewHope) series() (*goquery.Document, dbNewHopeEdition, error) {
	e, err := d.edition()
	if err != nil {
		return nil, e, err
	}
	if d.doc != nil {
		return d.doc, e, nil
	}
	body, err := http.Get(http.RequestParams{URL: "https://" + dbNewHopeDomain + "/" + e.dir + "/" + e.list})
	if err != nil {
		return nil, e, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, e, err
	}
	d.doc = doc
	return doc, e, nil
}

// FetchTitle names the series after the edition, so the translations don't
// share one name
func (d *DbNewHope) FetchTitle() (string, error) {
	e, err := d.edition()
	if err != nil {
		return "", err
	}
	return "DB New Hope (" + e.name + ")", nil
}

// FetchChapters returns the edition's chapter cards
func (d *DbNewHope) FetchChapters() (Filterables, []error) {
	doc, e, err := d.series()
	if err != nil {
		return nil, []error{err}
	}
	chapters := dbNewHopeChapters(doc, e)
	if len(chapters) == 0 {
		return nil, []error{fmt.Errorf("no chapters found for %s", d.URL)}
	}
	return chapters, nil
}

// dbNewHopeChapters parses the cards: "Capitulo 1:<br/>Una Oscura Derotta"
// gives the number and the chapter's own title
func dbNewHopeChapters(doc *goquery.Document, e dbNewHopeEdition) Filterables {
	base, _ := url.Parse("https://" + dbNewHopeDomain + "/" + e.dir + "/" + e.list)
	var chapters Filterables
	doc.Find("div.card").Each(func(_ int, card *goquery.Selection) {
		href := card.Find(`a[href$="/01.php"]`).First().AttrOr("href", "")
		if href == "" {
			return
		}
		ref, err := url.Parse(href)
		if err != nil {
			return
		}
		name := sanitizeTitle(card.Find("h5.card-title").First().Text())
		m := ChapterTitlePrefix.FindStringSubmatchIndex(name)
		if m == nil {
			return
		}
		numStr := name[m[2]:m[3]]
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return
		}
		u := base.ResolveReference(ref).String()
		chapters = append(chapters, &DbNewHopeChapter{
			Chapter{
				Number:   num,
				Title:    strings.TrimSpace(name[m[1]:]),
				Language: e.lang,
				ID:       chapterURLID(u),
				Label:    Label{Style: LabelPrefixed, Number: numStr, Lang: e.lang},
			},
			u,
		})
	})
	return chapters
}

// dbNewHopeWorkers bounds the page-document fetches of one chapter
const dbNewHopeWorkers = 4

// FetchChapter reads the first reader page for the page count, then each
// ?page=N document for its image
func (d *DbNewHope) FetchChapter(f Filterable) (*Chapter, error) {
	dchap := f.(*DbNewHopeChapter)
	first, err := d.readerPage(dchap.URL)
	if err != nil {
		return nil, err
	}
	count := first.Find("div.nav select option").Length()
	if count < 1 {
		count = 1
	}
	urls := make([]string, count)
	urls[0] = dbNewHopeImage(first, dchap.URL)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, dbNewHopeWorkers)
	for i := 2; i <= count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pageURL := dchap.URL + "?page=" + strconv.Itoa(i)
			doc, err := d.readerPage(pageURL)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			urls[i-1] = dbNewHopeImage(doc, pageURL)
		}(i)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	pages := make([]Page, 0, count)
	for i, u := range urls {
		if u == "" {
			return nil, fmt.Errorf("no image found on page %d of %s", i+1, ChapterLabel(f))
		}
		pages = append(pages, Page{Number: int64(i + 1), URL: u})
	}
	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   dchap.Language,
		ID:         dchap.ID,
		Pages:      pages,
	}, nil
}

// readerPage fetches one reader document
func (d *DbNewHope) readerPage(uri string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: d.BaseUrl()})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return goquery.NewDocumentFromReader(body)
}

// dbNewHopeImage returns the reader page's image, made absolute
func dbNewHopeImage(doc *goquery.Document, pageURL string) string {
	src := strings.TrimSpace(doc.Find("img.manga-page").First().AttrOr("src", ""))
	if src == "" {
		return ""
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(src)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}
