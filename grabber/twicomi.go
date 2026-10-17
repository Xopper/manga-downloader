// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Twicomi is a grabber for twicomi.com, an index of manga posted on Twitter:
// an author is the series and each of their manga tweets a chapter (up to
// four pictures), listed oldest first by the site's API. A tweet's page
// (/manga/{author}/{tweet}) lists its author.
type Twicomi struct {
	*Grabber
	tweets []twicomiTweet
}

// TwicomiChapter is one manga tweet
type TwicomiChapter struct {
	Chapter
	Images []string
}

// twicomiTweet is a tweet as the author list gives it
type twicomiTweet struct {
	Author struct {
		Name       string `json:"name"`
		ScreenName string `json:"screen_name"`
	} `json:"author"`
	Tweet struct {
		ID     string   `json:"tweet_id"`
		Text   string   `json:"tweet_text"`
		Images []string `json:"attach_image_urls"`
	} `json:"tweet"`
}

// twicomiAPI is the site's API, a var so tests can point it elsewhere
var twicomiAPI = "https://api.twicomi.com/api/v2"

// twicomiPageSize is how many tweets one list request asks for
const twicomiPageSize = 100

// twicomiAuthorRe captures the author of a /manga/{author}[/{tweet}] path
var twicomiAuthorRe = regexp.MustCompile(`^/manga/([^/]+)`)

func NewTwicomi(g *Grabber) *Twicomi {
	return &Twicomi{Grabber: g}
}

// Test returns true for a twicomi.com author or tweet URL
func (t *Twicomi) Test() (bool, error) {
	return hostMatchesDomain(t.URL, "twicomi.com") && t.author() != "", nil
}

// author returns the URL's author (screen name)
func (t Twicomi) author() string {
	u, err := url.Parse(t.URL)
	if err != nil {
		return ""
	}
	matches := twicomiAuthorRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return ""
	}
	return matches[1]
}

// listTweets walks the author's manga tweets once, oldest first
func (t *Twicomi) listTweets() ([]twicomiTweet, error) {
	if t.tweets != nil {
		return t.tweets, nil
	}
	tweets := []twicomiTweet{}
	for page := 1; ; page++ {
		uri := fmt.Sprintf("%s/author/manga/list?screen_name=%s&order_by=create_time&order=asc&page_no=%d&page_limit=%d",
			twicomiAPI, url.QueryEscape(t.author()), page, twicomiPageSize)
		body, err := http.GetText(http.RequestParams{URL: uri, Referer: t.URL, Accept: "application/json"})
		if err != nil {
			return nil, err
		}
		var feed struct {
			Response struct {
				Total int            `json:"total_count"`
				List  []twicomiTweet `json:"manga_list"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(body), &feed); err != nil {
			return nil, err
		}
		tweets = append(tweets, feed.Response.List...)
		if len(feed.Response.List) < twicomiPageSize || len(tweets) >= feed.Response.Total {
			break
		}
	}
	t.tweets = tweets
	return tweets, nil
}

// FetchTitle returns the author's name
func (t *Twicomi) FetchTitle() (string, error) {
	tweets, err := t.listTweets()
	if err != nil {
		return "", err
	}
	if len(tweets) == 0 {
		return sanitizeTitle(t.author()), nil
	}
	return sanitizeTitle(tweets[0].Author.Name), nil
}

// FetchChapters returns the author's manga tweets, titled by their first line
func (t *Twicomi) FetchChapters() (Filterables, []error) {
	tweets, err := t.listTweets()
	if err != nil {
		return nil, []error{err}
	}
	chapters := Filterables{}
	for _, tw := range tweets {
		if len(tw.Tweet.Images) == 0 {
			continue
		}
		title, _, _ := strings.Cut(strings.TrimSpace(tw.Tweet.Text), "\n")
		chapters = append(chapters, &TwicomiChapter{
			Chapter: Chapter{
				Number: float64(len(chapters) + 1),
				Title:  strings.TrimSpace(title),
				ID:     tw.Tweet.ID,
				Label:  Label{Style: LabelNumbered},
			},
			Images: tw.Tweet.Images,
		})
	}
	return chapters, nil
}

// FetchChapter returns the tweet's pictures, at their original size
func (t *Twicomi) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*TwicomiChapter)
	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   "ja",
		ID:         tchap.ID,
		PagesCount: int64(len(tchap.Images)),
	}
	for i, image := range tchap.Images {
		if strings.HasPrefix(image, "https://pbs.twimg.com/media/") && !strings.Contains(image, "?") {
			image += "?name=orig"
		}
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: image})
	}
	return chapter, nil
}
