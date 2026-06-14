package openlibrary_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/openlibrary-cli/openlibrary"
)

const fakeSearch = `{
  "numFound": 2,
  "docs": [
    {
      "key": "/works/OL30906747W",
      "title": "Core Python Programming",
      "author_name": ["R. Nageswara Rao"],
      "first_publish_year": 2016,
      "edition_count": 3,
      "ebook_access": "no_ebook",
      "language": ["eng"],
      "cover_i": 12345
    },
    {
      "key": "/works/OL1234567W",
      "title": "Python Cookbook",
      "author_name": ["David Beazley", "Brian K. Jones"],
      "first_publish_year": 2013,
      "edition_count": 2,
      "ebook_access": "borrowable",
      "language": ["eng"],
      "cover_i": 0
    }
  ]
}`

const fakeSubject = `{
  "name": "Computer science",
  "work_count": 5000,
  "works": [
    {
      "key": "/works/OL30906747W",
      "title": "Core Python Programming",
      "authors": [{"name": "R. Nageswara Rao"}],
      "cover_id": 12345,
      "edition_count": 3
    }
  ]
}`

func newTestClient(ts *httptest.Server) *openlibrary.Client {
	cfg := openlibrary.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return openlibrary.NewClient(cfg)
}

func TestSearchBooksSendsUserAgent(t *testing.T) {
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = fmt.Fprint(w, fakeSearch)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.SearchBooks(context.Background(), "python", 2)
	if err != nil {
		t.Fatal(err)
	}
	want := openlibrary.DefaultConfig().UserAgent
	if gotUA != want {
		t.Errorf("User-Agent = %q, want %q", gotUA, want)
	}
}

func TestSearchBooksParsesItems(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakeSearch)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	books, err := c.SearchBooks(context.Background(), "python", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("want 2 books, got %d", len(books))
	}

	b := books[0]
	if b.Key != "OL30906747W" {
		t.Errorf("Key = %q, want OL30906747W", b.Key)
	}
	if b.Title != "Core Python Programming" {
		t.Errorf("Title = %q", b.Title)
	}
	if len(b.Authors) != 1 || b.Authors[0] != "R. Nageswara Rao" {
		t.Errorf("Authors = %v", b.Authors)
	}
	if b.FirstYear != 2016 {
		t.Errorf("FirstYear = %d, want 2016", b.FirstYear)
	}
	if b.Editions != 3 {
		t.Errorf("Editions = %d, want 3", b.Editions)
	}
	if b.EbookAccess != "no_ebook" {
		t.Errorf("EbookAccess = %q", b.EbookAccess)
	}
	if len(b.Languages) != 1 || b.Languages[0] != "eng" {
		t.Errorf("Languages = %v", b.Languages)
	}
	if b.CoverID != 12345 {
		t.Errorf("CoverID = %d, want 12345", b.CoverID)
	}
	if b.URL != "https://openlibrary.org/works/OL30906747W" {
		t.Errorf("URL = %q", b.URL)
	}
	if b.Rank != 1 {
		t.Errorf("Rank = %d, want 1", b.Rank)
	}
	if books[1].Rank != 2 {
		t.Errorf("books[1].Rank = %d, want 2", books[1].Rank)
	}
	if books[1].CoverID != 0 {
		t.Errorf("books[1].CoverID = %d, want 0", books[1].CoverID)
	}
}

func TestSearchBooksLimitRespected(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = fmt.Fprint(w, fakeSearch)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.SearchBooks(context.Background(), "python", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "limit=3") {
		t.Errorf("query %q does not contain limit=3", gotQuery)
	}
}

func TestSearchBooksRetriesOn503(t *testing.T) {
	var hits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, fakeSearch)
	}))
	defer ts.Close()

	cfg := openlibrary.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 3
	c := openlibrary.NewClient(cfg)

	books, err := c.SearchBooks(context.Background(), "python", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("want 2 books after retry, got %d", len(books))
	}
	if hits != 2 {
		t.Errorf("server saw %d hits, want 2", hits)
	}
}

const fakeAuthorSearch = `{
  "numFound": 3,
  "docs": [
    {
      "key": "OL26320A",
      "name": "J.R.R. Tolkien",
      "birth_date": "1892",
      "top_work": "The Lord of the Rings",
      "work_count": 183
    },
    {
      "key": "OL999A",
      "name": "Christopher Tolkien",
      "birth_date": "1924",
      "top_work": "The Silmarillion",
      "work_count": 42
    }
  ]
}`

const fakeAuthorDetail = `{
  "key": "/authors/OL23919A",
  "name": "J. K. Rowling",
  "birth_date": "31 July 1965",
  "death_date": ""
}`

const fakeWorkDetail = `{
  "key": "/works/OL45804W",
  "title": "Fantastic Mr Fox",
  "description": {"type": "/type/text", "value": "A story about foxes."},
  "subjects": ["Animals", "Foxes", "Children's fiction"],
  "authors": [
    {"author": {"key": "/authors/OL34184A"}}
  ]
}`

const fakeWorkDetailStringDesc = `{
  "key": "/works/OL99W",
  "title": "Some Book",
  "description": "A plain string description.",
  "subjects": [],
  "authors": []
}`

func TestSearchAuthorsParsesItems(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeAuthorSearch)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	authors, err := c.SearchAuthors(context.Background(), "tolkien", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/search/authors.json") {
		t.Errorf("path = %q, want /search/authors.json", gotPath)
	}
	if len(authors) != 2 {
		t.Fatalf("want 2 authors, got %d", len(authors))
	}
	a := authors[0]
	if a.Key != "OL26320A" {
		t.Errorf("Key = %q, want OL26320A", a.Key)
	}
	if a.Name != "J.R.R. Tolkien" {
		t.Errorf("Name = %q", a.Name)
	}
	if a.BirthDate != "1892" {
		t.Errorf("BirthDate = %q, want 1892", a.BirthDate)
	}
	if a.TopWork != "The Lord of the Rings" {
		t.Errorf("TopWork = %q", a.TopWork)
	}
	if a.WorkCount != 183 {
		t.Errorf("WorkCount = %d, want 183", a.WorkCount)
	}
	if a.Rank != 1 {
		t.Errorf("Rank = %d, want 1", a.Rank)
	}
	if authors[1].Rank != 2 {
		t.Errorf("authors[1].Rank = %d, want 2", authors[1].Rank)
	}
	wantURL := "https://openlibrary.org/authors/OL26320A"
	if a.URL != wantURL {
		t.Errorf("URL = %q, want %q", a.URL, wantURL)
	}
}

func TestGetAuthorParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeAuthorDetail)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	author, err := c.GetAuthor(context.Background(), "OL23919A")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/authors/OL23919A.json") {
		t.Errorf("path = %q, want to contain /authors/OL23919A.json", gotPath)
	}
	if author.Name != "J. K. Rowling" {
		t.Errorf("Name = %q", author.Name)
	}
	if author.BirthDate != "31 July 1965" {
		t.Errorf("BirthDate = %q", author.BirthDate)
	}
	if author.Key != "OL23919A" {
		t.Errorf("Key = %q, want OL23919A (prefix stripped)", author.Key)
	}
	wantURL := "https://openlibrary.org/authors/OL23919A"
	if author.URL != wantURL {
		t.Errorf("URL = %q, want %q", author.URL, wantURL)
	}
}

func TestGetWorkParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeWorkDetail)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	work, err := c.GetWork(context.Background(), "OL45804W")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/works/OL45804W.json") {
		t.Errorf("path = %q, want to contain /works/OL45804W.json", gotPath)
	}
	if work.Title != "Fantastic Mr Fox" {
		t.Errorf("Title = %q", work.Title)
	}
	if work.Key != "OL45804W" {
		t.Errorf("Key = %q, want OL45804W", work.Key)
	}
	if work.Desc != "A story about foxes." {
		t.Errorf("Desc = %q", work.Desc)
	}
	if len(work.Subjects) != 3 {
		t.Errorf("len(Subjects) = %d, want 3", len(work.Subjects))
	}
	if len(work.AuthorKeys) != 1 || work.AuthorKeys[0] != "/authors/OL34184A" {
		t.Errorf("AuthorKeys = %v", work.AuthorKeys)
	}
	wantURL := "https://openlibrary.org/works/OL45804W"
	if work.URL != wantURL {
		t.Errorf("URL = %q, want %q", work.URL, wantURL)
	}
}

func TestGetWorkDescriptionFlattened(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakeWorkDetailStringDesc)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	work, err := c.GetWork(context.Background(), "OL99W")
	if err != nil {
		t.Fatal(err)
	}
	if work.Desc != "A plain string description." {
		t.Errorf("Desc = %q, want plain string", work.Desc)
	}
}

func TestSubjectParsesItems(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeSubject)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	books, err := c.Subject(context.Background(), "computer science", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/subjects/computer_science.json") {
		t.Errorf("path = %q, want to contain /subjects/computer_science.json", gotPath)
	}
	if len(books) != 1 {
		t.Fatalf("want 1 book, got %d", len(books))
	}
	b := books[0]
	if b.Title != "Core Python Programming" {
		t.Errorf("Title = %q", b.Title)
	}
	if len(b.Authors) != 1 || b.Authors[0] != "R. Nageswara Rao" {
		t.Errorf("Authors = %v", b.Authors)
	}
}
