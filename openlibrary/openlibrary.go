// Package openlibrary is the library behind the openlibrary command line:
// the HTTP client, request shaping, and the typed data models for openlibrary.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package openlibrary

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to Open Library.
const DefaultUserAgent = "Mozilla/5.0 (compatible; openlibrary-cli/dev; +https://github.com/tamnd/openlibrary-cli)"

// Host is the site this client talks to.
const Host = "openlibrary.org"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Config holds all tunable client parameters.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults for a polite CLI client.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to Open Library over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client using cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// --- internal API types ---

type searchResponse struct {
	NumFound int      `json:"numFound"`
	Docs     []rawDoc `json:"docs"`
}

type rawDoc struct {
	Key              string   `json:"key"`
	Title            string   `json:"title"`
	AuthorName       []string `json:"author_name"`
	FirstPublishYear int      `json:"first_publish_year"`
	EditionCount     int      `json:"edition_count"`
	EbookAccess      string   `json:"ebook_access"`
	Language         []string `json:"language"`
	CoverI           int      `json:"cover_i"`
}

type subjectResponse struct {
	Name      string    `json:"name"`
	WorkCount int       `json:"work_count"`
	Works     []rawWork `json:"works"`
}

type rawWork struct {
	Key          string      `json:"key"`
	Title        string      `json:"title"`
	Authors      []rawAuthor `json:"authors"`
	CoverID      int         `json:"cover_id"`
	EditionCount int         `json:"edition_count"`
}

type rawAuthor struct {
	Name string `json:"name"`
}

// SearchBooks searches Open Library for books matching query.
func (c *Client) SearchBooks(ctx context.Context, query string, limit int) ([]Book, error) {
	u := fmt.Sprintf("%s/search.json?q=%s&limit=%d",
		c.cfg.BaseURL, url.QueryEscape(query), limit)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	books := make([]Book, len(resp.Docs))
	for i, d := range resp.Docs {
		key := stripWorksPrefix(d.Key)
		books[i] = Book{
			Rank:        i + 1,
			Key:         key,
			Title:       d.Title,
			Authors:     d.AuthorName,
			FirstYear:   d.FirstPublishYear,
			Editions:    d.EditionCount,
			EbookAccess: d.EbookAccess,
			Languages:   d.Language,
			CoverID:     d.CoverI,
			URL:         "https://openlibrary.org/works/" + key,
		}
	}
	return books, nil
}

// Subject returns the books under a subject category.
// subject may contain spaces; they are converted to underscores.
func (c *Client) Subject(ctx context.Context, subject string, limit int) ([]Book, error) {
	slug := subjectSlug(subject)
	u := fmt.Sprintf("%s/subjects/%s.json?limit=%d", c.cfg.BaseURL, slug, limit)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp subjectResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode subject response: %w", err)
	}
	books := make([]Book, len(resp.Works))
	for i, w := range resp.Works {
		key := stripWorksPrefix(w.Key)
		authors := make([]string, len(w.Authors))
		for j, a := range w.Authors {
			authors[j] = a.Name
		}
		books[i] = Book{
			Rank:     i + 1,
			Key:      key,
			Title:    w.Title,
			Authors:  authors,
			Editions: w.EditionCount,
			CoverID:  w.CoverID,
			URL:      "https://openlibrary.org/works/" + key,
		}
	}
	return books, nil
}

// --- HTTP helpers ---

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	return min(time.Duration(attempt)*500*time.Millisecond, 5*time.Second)
}

// subjectSlug normalises a subject name into the slug the API expects.
func subjectSlug(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "_"))
}

// stripWorksPrefix removes the /works/ prefix from an Open Library key.
func stripWorksPrefix(key string) string {
	return strings.TrimPrefix(key, "/works/")
}
