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
	"sync"
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
		Rate:      500 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to Open Library over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client using cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// --- internal wire types ---

type searchResponse struct {
	NumFound int      `json:"numFound"`
	Docs     []rawDoc `json:"docs"`
}

type rawDoc struct {
	Key              string   `json:"key"`
	Title            string   `json:"title"`
	AuthorName       []string `json:"author_name"`
	FirstPublishYear int      `json:"first_publish_year"`
	ISBN             []string `json:"isbn"`
}

type rawWorkDetail struct {
	Key         string          `json:"key"`
	Title       string          `json:"title"`
	Description json.RawMessage `json:"description"`
	Subjects    []string        `json:"subjects"`
	Covers      []int           `json:"covers"`
}

type rawAuthorDetail struct {
	Key       string          `json:"key"`
	Name      string          `json:"name"`
	BirthDate string          `json:"birth_date"`
	DeathDate string          `json:"death_date"`
	Bio       json.RawMessage `json:"bio"`
}

type rawAuthorWorks struct {
	Size    int       `json:"size"`
	Entries []rawEntry `json:"entries"`
}

type rawEntry struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

type rawSubjectResponse struct {
	Name      string        `json:"name"`
	WorkCount int           `json:"work_count"`
	Works     []rawSubjWork `json:"works"`
}

type rawSubjWork struct {
	Key     string      `json:"key"`
	Title   string      `json:"title"`
	Authors []rawAuthor `json:"authors"`
}

type rawAuthor struct {
	Name string `json:"name"`
}

type rawEdition struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Publishers  []string `json:"publishers"`
	PublishDate string   `json:"publish_date"`
	Pages       int      `json:"number_of_pages"`
	ISBN10      []string `json:"isbn_10"`
	ISBN13      []string `json:"isbn_13"`
}

// flattenText handles Open Library's polymorphic text fields, which can be
// either a plain string or {"type":"/type/text","value":"..."}.
func flattenText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Value
	}
	return ""
}

// SearchBooks searches Open Library for books matching query.
func (c *Client) SearchBooks(ctx context.Context, query string, limit int) ([]Book, error) {
	u := fmt.Sprintf("%s/search.json?q=%s&fields=key,title,author_name,first_publish_year,isbn&limit=%d",
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
		books[i] = Book{
			Key:         stripWorksPrefix(d.Key),
			Title:       d.Title,
			Authors:     d.AuthorName,
			PublishYear: d.FirstPublishYear,
			ISBN:        d.ISBN,
		}
	}
	return books, nil
}

// GetWork fetches the full work record by OL ID (e.g. "OL45804W").
// The /works/ prefix is stripped from olid if present.
func (c *Client) GetWork(ctx context.Context, olid string) (*Work, error) {
	olid = stripWorksPrefix(olid)
	u := fmt.Sprintf("%s/works/%s.json", c.cfg.BaseURL, olid)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw rawWorkDetail
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode work response: %w", err)
	}
	return &Work{
		Key:         stripWorksPrefix(raw.Key),
		Title:       raw.Title,
		Description: flattenText(raw.Description),
		Subjects:    raw.Subjects,
		Covers:      raw.Covers,
	}, nil
}

// GetAuthor fetches the full author record by OL ID (e.g. "OL26320A").
// The /authors/ prefix is stripped from olid if present.
func (c *Client) GetAuthor(ctx context.Context, olid string) (*Author, error) {
	olid = stripAuthorsPrefix(olid)
	u := fmt.Sprintf("%s/authors/%s.json", c.cfg.BaseURL, olid)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw rawAuthorDetail
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode author response: %w", err)
	}
	return &Author{
		Key:       stripAuthorsPrefix(raw.Key),
		Name:      raw.Name,
		BirthDate: raw.BirthDate,
		DeathDate: raw.DeathDate,
		Bio:       flattenText(raw.Bio),
	}, nil
}

// GetAuthorWorks fetches the works list for an author by OL ID.
func (c *Client) GetAuthorWorks(ctx context.Context, olid string, limit int) ([]SubjectWork, error) {
	olid = stripAuthorsPrefix(olid)
	u := fmt.Sprintf("%s/authors/%s/works.json?limit=%d", c.cfg.BaseURL, olid, limit)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw rawAuthorWorks
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode author works response: %w", err)
	}
	works := make([]SubjectWork, len(raw.Entries))
	for i, e := range raw.Entries {
		works[i] = SubjectWork{
			Key:   stripWorksPrefix(e.Key),
			Title: e.Title,
		}
	}
	return works, nil
}

// GetSubject fetches the works under a subject category.
func (c *Client) GetSubject(ctx context.Context, subject string, limit int) ([]SubjectWork, error) {
	slug := subjectSlug(subject)
	u := fmt.Sprintf("%s/subjects/%s.json?limit=%d", c.cfg.BaseURL, slug, limit)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp rawSubjectResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode subject response: %w", err)
	}
	works := make([]SubjectWork, len(resp.Works))
	for i, w := range resp.Works {
		authors := make([]string, len(w.Authors))
		for j, a := range w.Authors {
			authors[j] = a.Name
		}
		works[i] = SubjectWork{
			Key:     stripWorksPrefix(w.Key),
			Title:   w.Title,
			Authors: authors,
		}
	}
	return works, nil
}

// GetEditionByISBN fetches a book edition by ISBN (10 or 13 digits).
func (c *Client) GetEditionByISBN(ctx context.Context, isbn string) (*Edition, error) {
	u := fmt.Sprintf("%s/isbn/%s.json", c.cfg.BaseURL, isbn)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw rawEdition
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode isbn response: %w", err)
	}
	return &Edition{
		Key:         raw.Key,
		Title:       raw.Title,
		Publishers:  raw.Publishers,
		PublishDate: raw.PublishDate,
		Pages:       raw.Pages,
		ISBN10:      raw.ISBN10,
		ISBN13:      raw.ISBN13,
	}, nil
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
	c.mu.Lock()
	defer c.mu.Unlock()
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

// stripAuthorsPrefix removes the /authors/ prefix from an Open Library key.
func stripAuthorsPrefix(key string) string {
	return strings.TrimPrefix(key, "/authors/")
}
