// Package openlibrary is the library behind the openlibrary command line:
// the HTTP client, request shaping, and the typed data models for Open Library.
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
const DefaultUserAgent = "openlibrary-cli/0.1 (tamnd87@gmail.com)"

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
		Rate:      200 * time.Millisecond,
		Timeout:   15 * time.Second,
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
	Key                  string   `json:"key"`
	Title                string   `json:"title"`
	AuthorName           []string `json:"author_name"`
	FirstPublishYear     int      `json:"first_publish_year"`
	ISBN                 []string `json:"isbn"`
	Subject              []string `json:"subject"`
	NumberOfPagesMedian  int      `json:"number_of_pages_median"`
}

// rawISBNBook is the per-ISBN entry returned by /api/books.
type rawISBNBook struct {
	Title         string          `json:"title"`
	URL           string          `json:"url"`
	Authors       []rawISBNAuthor `json:"authors"`
	Publishers    []rawISBNPub    `json:"publishers"`
	PublishDate   string          `json:"publish_date"`
	NumberOfPages int             `json:"number_of_pages"`
	Subjects      []rawISBNSub    `json:"subjects"`
	Identifiers   rawISBNIdents   `json:"identifiers"`
}

type rawISBNAuthor struct {
	Name string `json:"name"`
}

type rawISBNPub struct {
	Name string `json:"name"`
}

type rawISBNSub struct {
	Name string `json:"name"`
}

type rawISBNIdents struct {
	ISBN10 []string `json:"isbn_10"`
	ISBN13 []string `json:"isbn_13"`
}

// rawAuthorSearchResponse is the result of /search/authors.json.
type rawAuthorSearchResponse struct {
	NumFound int           `json:"numFound"`
	Docs     []rawAuthorDoc `json:"docs"`
}

type rawAuthorDoc struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	BirthDate string `json:"birth_date"`
	DeathDate string `json:"death_date"`
}

type rawWorkDetail struct {
	Key              string          `json:"key"`
	Title            string          `json:"title"`
	Description      json.RawMessage `json:"description"`
	Subjects         []string        `json:"subjects"`
	FirstPublishDate string          `json:"first_publish_date"`
}

type rawAuthorDetail struct {
	Key       string          `json:"key"`
	Name      string          `json:"name"`
	BirthDate string          `json:"birth_date"`
	DeathDate string          `json:"death_date"`
	Bio       json.RawMessage `json:"bio"`
}

type rawEditionsResponse struct {
	Entries []rawEditionEntry `json:"entries"`
}

type rawEditionEntry struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Publishers  []string `json:"publishers"`
	PublishDate string   `json:"publish_date"`
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
	u := fmt.Sprintf("%s/search.json?q=%s&fields=key,title,author_name,first_publish_year,isbn,subject,number_of_pages_median&limit=%d",
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
		isbn := ""
		if len(d.ISBN) > 0 {
			isbn = d.ISBN[0]
		}
		subjects := d.Subject
		if len(subjects) > 3 {
			subjects = subjects[:3]
		}
		books[i] = Book{
			Key:              stripWorksPrefix(d.Key),
			Title:            d.Title,
			Authors:          strings.Join(d.AuthorName, ", "),
			FirstPublishYear: d.FirstPublishYear,
			ISBN:             isbn,
			Subjects:         strings.Join(subjects, ", "),
			Pages:            d.NumberOfPagesMedian,
		}
	}
	return books, nil
}

// normalizeISBN strips hyphens from an ISBN string and returns digits only.
func normalizeISBN(isbn string) string {
	var b strings.Builder
	for _, r := range isbn {
		if r >= '0' && r <= '9' || r == 'X' || r == 'x' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// GetBookByISBN fetches a book by ISBN using the /api/books endpoint.
// The isbn argument may contain hyphens; they are stripped automatically.
func (c *Client) GetBookByISBN(ctx context.Context, isbn string) (*Book, error) {
	isbn = normalizeISBN(isbn)
	bibkey := "ISBN:" + isbn
	u := fmt.Sprintf("%s/api/books?bibkeys=%s&format=json&jscmd=data",
		c.cfg.BaseURL, url.QueryEscape(bibkey))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp map[string]rawISBNBook
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode isbn response: %w", err)
	}
	raw, ok := resp[bibkey]
	if !ok {
		return nil, fmt.Errorf("book not found for ISBN %s", isbn)
	}
	authorNames := make([]string, len(raw.Authors))
	for i, a := range raw.Authors {
		authorNames[i] = a.Name
	}
	subjects := make([]string, len(raw.Subjects))
	for i, s := range raw.Subjects {
		subjects[i] = s.Name
	}
	if len(subjects) > 3 {
		subjects = subjects[:3]
	}
	return &Book{
		Key:      bibkey,
		Title:    raw.Title,
		Authors:  strings.Join(authorNames, ", "),
		ISBN:     isbn,
		Subjects: strings.Join(subjects, ", "),
		Pages:    raw.NumberOfPages,
	}, nil
}

// SearchAuthors searches Open Library for authors matching query.
func (c *Client) SearchAuthors(ctx context.Context, query string, limit int) ([]Author, error) {
	u := fmt.Sprintf("%s/search/authors.json?q=%s&limit=%d",
		c.cfg.BaseURL, url.QueryEscape(query), limit)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp rawAuthorSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode author search response: %w", err)
	}
	authors := make([]Author, len(resp.Docs))
	for i, d := range resp.Docs {
		authors[i] = Author{
			Key:       stripAuthorsPrefix(d.Key),
			Name:      d.Name,
			BirthDate: d.BirthDate,
			DeathDate: d.DeathDate,
		}
	}
	return authors, nil
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
	subjects := raw.Subjects
	if len(subjects) > 5 {
		subjects = subjects[:5]
	}
	return &Work{
		Key:          stripWorksPrefix(raw.Key),
		Title:        raw.Title,
		Description:  flattenText(raw.Description),
		Subjects:     strings.Join(subjects, ", "),
		FirstPublish: raw.FirstPublishDate,
	}, nil
}

// GetAuthor fetches the full author record by OL ID (e.g. "OL34184A").
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

// GetEditions fetches the editions list for a work by OL ID (e.g. "OL45804W").
func (c *Client) GetEditions(ctx context.Context, olid string, limit int) ([]Edition, error) {
	olid = stripWorksPrefix(olid)
	u := fmt.Sprintf("%s/works/%s/editions.json?limit=%d", c.cfg.BaseURL, olid, limit)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp rawEditionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode editions response: %w", err)
	}
	editions := make([]Edition, len(resp.Entries))
	for i, e := range resp.Entries {
		publisher := ""
		if len(e.Publishers) > 0 {
			publisher = e.Publishers[0]
		}
		isbn13 := ""
		if len(e.ISBN13) > 0 {
			isbn13 = e.ISBN13[0]
		}
		editions[i] = Edition{
			Key:       e.Key,
			Title:     e.Title,
			Publisher: publisher,
			Published: e.PublishDate,
			ISBN13:    isbn13,
		}
	}
	return editions, nil
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

// stripWorksPrefix removes the /works/ prefix from an Open Library key.
func stripWorksPrefix(key string) string {
	return strings.TrimPrefix(key, "/works/")
}

// stripAuthorsPrefix removes the /authors/ prefix from an Open Library key.
func stripAuthorsPrefix(key string) string {
	return strings.TrimPrefix(key, "/authors/")
}
