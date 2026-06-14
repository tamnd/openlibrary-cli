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
		Rate:      300 * time.Millisecond,
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

type authorSearchResponse struct {
	NumFound int             `json:"numFound"`
	Docs     []rawAuthorDoc  `json:"docs"`
}

type rawAuthorDoc struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	BirthDate string `json:"birth_date"`
	WorkCount int    `json:"work_count"`
	TopWork   string `json:"top_work"`
}

type rawAuthorDetail struct {
	Key       string          `json:"key"`
	Name      string          `json:"name"`
	BirthDate string          `json:"birth_date"`
	DeathDate string          `json:"death_date"`
}

type rawWorkDetail struct {
	Key         string           `json:"key"`
	Title       string           `json:"title"`
	Description json.RawMessage  `json:"description"`
	Subjects    []string         `json:"subjects"`
	Authors     []rawWorkAuthor  `json:"authors"`
}

type rawWorkAuthor struct {
	Author struct {
		Key string `json:"key"`
	} `json:"author"`
}

// flattenText handles Open Library's polymorphic text fields, which can be
// either a plain string or {"type":"/type/text","value":"..."}.
func flattenText(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	var obj struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Value != "" {
		return obj.Value, true
	}
	return "", false
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

// SearchAuthors searches Open Library for authors matching query.
func (c *Client) SearchAuthors(ctx context.Context, query string, limit int) ([]Author, error) {
	u := fmt.Sprintf("%s/search/authors.json?q=%s&limit=%d",
		c.cfg.BaseURL, url.QueryEscape(query), limit)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp authorSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode author search response: %w", err)
	}
	authors := make([]Author, len(resp.Docs))
	for i, d := range resp.Docs {
		key := stripAuthorsPrefix(d.Key)
		authors[i] = Author{
			Rank:      i + 1,
			Key:       key,
			Name:      d.Name,
			BirthDate: d.BirthDate,
			WorkCount: d.WorkCount,
			TopWork:   d.TopWork,
			URL:       "https://openlibrary.org/authors/" + key,
		}
	}
	return authors, nil
}

// GetAuthor fetches the full author record by OL ID (e.g. "OL23919A").
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
	key := stripAuthorsPrefix(raw.Key)
	return &Author{
		Key:       key,
		Name:      raw.Name,
		BirthDate: raw.BirthDate,
		DeathDate: raw.DeathDate,
		URL:       "https://openlibrary.org/authors/" + key,
	}, nil
}

// GetBookByISBN fetches a book record by ISBN (10 or 13 digits).
// Uses the /api/books endpoint with jscmd=data.
func (c *Client) GetBookByISBN(ctx context.Context, isbn string) (*Book, error) {
	key := "ISBN:" + isbn
	u := fmt.Sprintf("%s/api/books?bibkeys=%s&format=json&jscmd=data",
		c.cfg.BaseURL, url.QueryEscape(key))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	// response: map[string]json.RawMessage keyed by "ISBN:NNNN"
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode isbn response: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("isbn %s not found", isbn)
	}
	// take the first (and typically only) value
	var first json.RawMessage
	for _, v := range raw {
		first = v
		break
	}
	var b wireISBNBook
	if err := json.Unmarshal(first, &b); err != nil {
		return nil, fmt.Errorf("decode isbn book: %w", err)
	}
	authors := make([]string, 0, len(b.Authors))
	for _, a := range b.Authors {
		if a.Name != "" {
			authors = append(authors, a.Name)
		}
	}
	publishers := make([]string, 0, len(b.Publishers))
	for _, p := range b.Publishers {
		if p.Name != "" {
			publishers = append(publishers, p.Name)
		}
	}
	subjects := make([]string, 0, len(b.Subjects))
	for _, s := range b.Subjects {
		if s.Name != "" {
			subjects = append(subjects, s.Name)
		}
	}
	return &Book{
		Title:       b.Title,
		Authors:     authors,
		Publishers:  publishers,
		PublishDate: b.PublishDate,
		Pages:       b.NumberOfPages,
		Subjects:    subjects,
		CoverURL:    b.Cover.Medium,
		URL:         b.URL,
	}, nil
}

// wireISBNBook is the wire type for a single book from /api/books?jscmd=data.
type wireISBNBook struct {
	Title       string `json:"title"`
	Authors     []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
	PublishDate   string `json:"publish_date"`
	Subjects      []struct {
		Name string `json:"name"`
	} `json:"subjects"`
	Cover struct {
		Medium string `json:"medium"`
	} `json:"cover"`
	NumberOfPages int    `json:"number_of_pages"`
	URL           string `json:"url"`
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
	desc, _ := flattenText(raw.Description)
	key := stripWorksPrefix(raw.Key)
	authorKeys := make([]string, len(raw.Authors))
	for i, a := range raw.Authors {
		authorKeys[i] = a.Author.Key
	}
	return &Work{
		Key:        key,
		Title:      raw.Title,
		Desc:       desc,
		Subjects:   raw.Subjects,
		AuthorKeys: authorKeys,
		URL:        "https://openlibrary.org/works/" + key,
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
