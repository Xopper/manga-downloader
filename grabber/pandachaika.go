// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// pandachaikaTailSize is how much of the archive's end is read for its zip
// directory: enough for a few hundred entries, the rest is fetched apart
const pandachaikaTailSize = 64 << 10

var (
	pandachaikaHostRe    = regexp.MustCompile(`(?i)^https?://(?:www\.)?panda\.chaika\.moe(?:[/?#]|$)`)
	pandachaikaArchiveRe = regexp.MustCompile(`^/archive/(\d+)`)
	pandachaikaImageRe   = regexp.MustCompile(`(?i)\.(?:jpe?g|png|gif|webp|avif|bmp)$`)
)

// Pandachaika is a grabber for panda.chaika.moe, a gallery backup site. A
// gallery is only kept as a zip archive (no page images are served), so the
// gallery is one chapter whose pages are its zip entries: the zip directory is
// read off the archive's end with a Range request, and each page downloads its
// own entry's bytes with another, inflated by the page's Transform.
type Pandachaika struct {
	*Grabber
	archive *pandachaikaArchive
}

func NewPandachaika(g *Grabber) *Pandachaika {
	return &Pandachaika{Grabber: g}
}

// PandachaikaChapter is the gallery, the only chapter of its series
type PandachaikaChapter struct {
	Chapter
	ArchiveID string
}

// pandachaikaArchive is the /api?archive={id} answer
type pandachaikaArchive struct {
	Title     string   `json:"title"`
	Download  string   `json:"download"`
	Tags      []string `json:"tags"`
	Filecount int      `json:"filecount"`
}

// pandachaikaEntry is a file of the zip, as its central directory says
type pandachaikaEntry struct {
	Name           string
	Method         uint16
	CompressedSize uint32
	Offset         uint32
}

// Test returns true for panda.chaika.moe URLs
func (p *Pandachaika) Test() (bool, error) {
	return pandachaikaHostRe.MatchString(p.URL), nil
}

// archiveID reads the archive id off an /archive/{id} URL (any page of it)
func (p *Pandachaika) archiveID() (string, error) {
	u, err := url.Parse(p.URL)
	if err != nil {
		return "", err
	}
	m := pandachaikaArchiveRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errors.New("not a panda.chaika.moe archive URL (/archive/{id})")
	}

	return m[1], nil
}

// fetchArchive returns the archive details, fetched once
func (p *Pandachaika) fetchArchive() (*pandachaikaArchive, error) {
	if p.archive != nil {
		return p.archive, nil
	}
	id, err := p.archiveID()
	if err != nil {
		return nil, err
	}
	text, err := http.GetText(http.RequestParams{URL: p.BaseUrl() + "/api?archive=" + id})
	if err != nil {
		return nil, err
	}
	archive := &pandachaikaArchive{}
	if err = json.Unmarshal([]byte(text), archive); err != nil {
		return nil, fmt.Errorf("could not read archive %s: %w", id, err)
	}
	if archive.Title == "" {
		return nil, fmt.Errorf("archive %s not found", id)
	}
	p.archive = archive

	return archive, nil
}

// FetchTitle returns the gallery title
func (p *Pandachaika) FetchTitle() (string, error) {
	archive, err := p.fetchArchive()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(archive.Title), nil
}

// FetchChapters returns the gallery as the series' only chapter
func (p *Pandachaika) FetchChapters() (Filterables, []error) {
	archive, err := p.fetchArchive()
	if err != nil {
		return nil, []error{err}
	}
	id, _ := p.archiveID()

	return Filterables{&PandachaikaChapter{
		Chapter: Chapter{
			Number:   1,
			ID:       id,
			Language: pandachaikaLanguage(archive.Tags),
			Label:    Label{Style: LabelUnnumbered},
		},
		ArchiveID: id,
	}}, nil
}

// pandachaikaLanguage maps the "language:english" tag to a language code
func pandachaikaLanguage(tags []string) string {
	codes := map[string]string{"english": "en", "japanese": "ja", "chinese": "zh", "korean": "ko", "spanish": "es", "french": "fr", "russian": "ru", "german": "de", "portuguese": "pt", "italian": "it"}
	for _, t := range tags {
		if lang, ok := strings.CutPrefix(t, "language:"); ok && codes[lang] != "" {
			return codes[lang]
		}
	}

	return ""
}

// rangeGet reads part of the archive (spec is a Range header value)
func (p *Pandachaika) rangeGet(archiveURL, spec string) ([]byte, error) {
	body, err := http.Get(http.RequestParams{URL: archiveURL, Referer: p.BaseUrl(), Headers: map[string]string{"Range": "bytes=" + spec}})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return io.ReadAll(body)
}

// FetchChapter reads the archive's zip directory and lists its pictures, in
// name order, as pages
func (p *Pandachaika) FetchChapter(f Filterable) (*Chapter, error) {
	pchap := f.(*PandachaikaChapter)
	archiveURL := fmt.Sprintf("%s/archive/%s/download/", p.BaseUrl(), pchap.ArchiveID)

	tail, err := p.rangeGet(archiveURL, fmt.Sprintf("-%d", pandachaikaTailSize))
	if err != nil {
		return nil, err
	}
	entries, err := pandachaikaDirectory(tail, func(from, to int64) ([]byte, error) {
		return p.rangeGet(archiveURL, fmt.Sprintf("%d-%d", from, to))
	})
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    pchap.GetTitle(),
		Number:   pchap.GetNumber(),
		Language: pchap.Language,
		ID:       pchap.ID,
		Label:    labelOf(f),
	}
	chapter.Pages = pandachaikaPages(archiveURL, entries)
	chapter.PagesCount = int64(len(chapter.Pages))
	if len(chapter.Pages) == 0 {
		return nil, errors.New("the archive holds no pictures")
	}

	return chapter, nil
}

// pandachaikaPages turns the zip entries into pages. Each page's Range runs
// from its local header to the next entry's, since the local header's extra
// field can differ in length from the central directory's.
func pandachaikaPages(archiveURL string, entries []pandachaikaEntry) []Page {
	byOffset := append([]pandachaikaEntry(nil), entries...)
	sort.Slice(byOffset, func(i, j int) bool { return byOffset[i].Offset < byOffset[j].Offset })
	next := map[uint32]int64{}
	for i, e := range byOffset {
		end := int64(e.Offset) + 30 + int64(len(e.Name)) + 1024 + int64(e.CompressedSize)
		if i+1 < len(byOffset) {
			end = int64(byOffset[i+1].Offset)
		}
		next[e.Offset] = end - 1
	}

	var images []pandachaikaEntry
	for _, e := range entries {
		if pandachaikaImageRe.MatchString(e.Name) && !strings.HasPrefix(path.Base(e.Name), ".") {
			images = append(images, e)
		}
	}
	sort.Slice(images, func(i, j int) bool { return strings.ToLower(images[i].Name) < strings.ToLower(images[j].Name) })

	pages := make([]Page, 0, len(images))
	for i, e := range images {
		entry := e
		pages = append(pages, Page{
			Number:    int64(i + 1),
			URL:       archiveURL,
			Headers:   map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", entry.Offset, next[entry.Offset])},
			Transform: func(b []byte) ([]byte, error) { return pandachaikaInflate(b, entry) },
		})
	}

	return pages
}

// pandachaikaDirectory parses the zip's central directory out of the archive's
// tail, fetching the directory apart (through fetch) when it starts before it
func pandachaikaDirectory(tail []byte, fetch func(from, to int64) ([]byte, error)) ([]pandachaikaEntry, error) {
	eocd := bytes.LastIndex(tail, []byte("PK\x05\x06"))
	if eocd < 0 || len(tail)-eocd < 22 {
		return nil, errors.New("the archive is not a zip file")
	}
	count := int(binary.LittleEndian.Uint16(tail[eocd+10:]))
	cdSize := int64(binary.LittleEndian.Uint32(tail[eocd+12:]))
	cdOffset := int64(binary.LittleEndian.Uint32(tail[eocd+16:]))
	if cdOffset == 0xFFFFFFFF {
		return nil, errors.New("zip64 archives are not supported")
	}

	// the directory ends where the end record starts
	tailStart := cdOffset + cdSize - int64(eocd)
	var cd []byte
	if cdOffset >= tailStart {
		cd = tail[cdOffset-tailStart : cdOffset-tailStart+cdSize]
	} else {
		var err error
		if cd, err = fetch(cdOffset, cdOffset+cdSize-1); err != nil {
			return nil, err
		}
	}

	entries := make([]pandachaikaEntry, 0, count)
	for pos := 0; pos+46 <= len(cd); {
		if binary.LittleEndian.Uint32(cd[pos:]) != 0x02014b50 {
			return nil, errors.New("broken zip directory")
		}
		nameLen := int(binary.LittleEndian.Uint16(cd[pos+28:]))
		extraLen := int(binary.LittleEndian.Uint16(cd[pos+30:]))
		commentLen := int(binary.LittleEndian.Uint16(cd[pos+32:]))
		if pos+46+nameLen > len(cd) {
			return nil, errors.New("broken zip directory")
		}
		entries = append(entries, pandachaikaEntry{
			Name:           string(cd[pos+46 : pos+46+nameLen]),
			Method:         binary.LittleEndian.Uint16(cd[pos+10:]),
			CompressedSize: binary.LittleEndian.Uint32(cd[pos+20:]),
			Offset:         binary.LittleEndian.Uint32(cd[pos+42:]),
		})
		pos += 46 + nameLen + extraLen + commentLen
	}

	return entries, nil
}

// pandachaikaInflate turns a downloaded entry (local header onwards) into the
// file it holds
func pandachaikaInflate(b []byte, e pandachaikaEntry) ([]byte, error) {
	if len(b) < 30 || binary.LittleEndian.Uint32(b) != 0x04034b50 {
		return nil, errors.New("the archive entry has no zip header")
	}
	start := 30 + int(binary.LittleEndian.Uint16(b[26:])) + int(binary.LittleEndian.Uint16(b[28:]))
	end := start + int(e.CompressedSize)
	if end > len(b) {
		return nil, errors.New("the archive entry is truncated")
	}
	data := b[start:end]

	switch e.Method {
	case 0:
		return data, nil
	case 8:
		return io.ReadAll(flate.NewReader(bytes.NewReader(data)))
	}

	return nil, fmt.Errorf("unsupported zip compression %d", e.Method)
}
