package openlibrary_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/openlibrary-cli/openlibrary"
)

// newTestClient creates a client pointed at a test server with rate limiting off.
func newTestClient(ts *httptest.Server) *openlibrary.Client {
	cfg := openlibrary.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return openlibrary.NewClient(cfg)
}

// --- Search ---

const fakeSearch = `{
  "numFound": 2,
  "docs": [
    {
      "key": "/works/OL45804W",
      "title": "Dune",
      "author_name": ["Frank Herbert"],
      "first_publish_year": 1965,
      "isbn": ["0441013597", "9780441013593"]
    },
    {
      "key": "/works/OL999W",
      "title": "Dune Messiah",
      "author_name": ["Frank Herbert"],
      "first_publish_year": 1969,
      "isbn": []
    }
  ]
}`

func TestSearchBooksParsesItems(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakeSearch)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	books, err := c.SearchBooks(context.Background(), "dune", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("want 2 books, got %d", len(books))
	}

	b := books[0]
	if b.Key != "OL45804W" {
		t.Errorf("Key = %q, want OL45804W (prefix stripped)", b.Key)
	}
	if b.Title != "Dune" {
		t.Errorf("Title = %q, want Dune", b.Title)
	}
	if len(b.Authors) != 1 || b.Authors[0] != "Frank Herbert" {
		t.Errorf("Authors = %v", b.Authors)
	}
	if b.PublishYear != 1965 {
		t.Errorf("PublishYear = %d, want 1965", b.PublishYear)
	}
	if len(b.ISBN) != 2 {
		t.Errorf("ISBN len = %d, want 2", len(b.ISBN))
	}
}

func TestSearchBooksSendsFieldsAndLimit(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = fmt.Fprint(w, fakeSearch)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, _ = c.SearchBooks(context.Background(), "dune", 3)
	if !strings.Contains(gotQuery, "limit=3") {
		t.Errorf("query %q does not contain limit=3", gotQuery)
	}
	if !strings.Contains(gotQuery, "fields=") {
		t.Errorf("query %q does not contain fields=", gotQuery)
	}
}

func TestSearchBooksSendsUserAgent(t *testing.T) {
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = fmt.Fprint(w, fakeSearch)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.SearchBooks(context.Background(), "dune", 2)
	if err != nil {
		t.Fatal(err)
	}
	want := openlibrary.DefaultConfig().UserAgent
	if gotUA != want {
		t.Errorf("User-Agent = %q, want %q", gotUA, want)
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

	books, err := c.SearchBooks(context.Background(), "dune", 2)
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

// --- Work ---

const fakeWork = `{
  "key": "/works/OL45804W",
  "title": "Dune",
  "description": {"type": "/type/text", "value": "A desert planet story."},
  "subjects": ["Science fiction", "Desert ecology"],
  "covers": [7979059, 1234567]
}`

const fakeWorkStringDesc = `{
  "key": "/works/OL99W",
  "title": "Some Book",
  "description": "A plain string description.",
  "subjects": [],
  "covers": []
}`

func TestGetWorkParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeWork)
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
	if work.Key != "OL45804W" {
		t.Errorf("Key = %q, want OL45804W", work.Key)
	}
	if work.Title != "Dune" {
		t.Errorf("Title = %q, want Dune", work.Title)
	}
	if work.Description != "A desert planet story." {
		t.Errorf("Description = %q", work.Description)
	}
	if len(work.Subjects) != 2 {
		t.Errorf("Subjects len = %d, want 2", len(work.Subjects))
	}
	if len(work.Covers) != 2 || work.Covers[0] != 7979059 {
		t.Errorf("Covers = %v", work.Covers)
	}
}

func TestGetWorkDescriptionStringFlattened(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakeWorkStringDesc)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	work, err := c.GetWork(context.Background(), "OL99W")
	if err != nil {
		t.Fatal(err)
	}
	if work.Description != "A plain string description." {
		t.Errorf("Description = %q, want plain string", work.Description)
	}
}

// --- Author ---

const fakeAuthor = `{
  "key": "/authors/OL26320A",
  "name": "J.R.R. Tolkien",
  "birth_date": "3 January 1892",
  "death_date": "2 September 1973",
  "bio": {"type": "/type/text", "value": "British author of The Lord of the Rings."}
}`

const fakeAuthorBioString = `{
  "key": "/authors/OL999A",
  "name": "Some Author",
  "birth_date": "1970",
  "death_date": "",
  "bio": "A plain bio string."
}`

const fakeAuthorWorks = `{
  "size": 403,
  "entries": [
    {"key": "/works/OL27516W", "title": "The Lord of the Rings"},
    {"key": "/works/OL27519W", "title": "The Hobbit"}
  ]
}`

func TestGetAuthorParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeAuthor)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	author, err := c.GetAuthor(context.Background(), "OL26320A")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/authors/OL26320A.json") {
		t.Errorf("path = %q, want /authors/OL26320A.json", gotPath)
	}
	if author.Key != "OL26320A" {
		t.Errorf("Key = %q, want OL26320A (prefix stripped)", author.Key)
	}
	if author.Name != "J.R.R. Tolkien" {
		t.Errorf("Name = %q", author.Name)
	}
	if author.BirthDate != "3 January 1892" {
		t.Errorf("BirthDate = %q", author.BirthDate)
	}
	if author.DeathDate != "2 September 1973" {
		t.Errorf("DeathDate = %q", author.DeathDate)
	}
	if author.Bio != "British author of The Lord of the Rings." {
		t.Errorf("Bio = %q", author.Bio)
	}
}

func TestGetAuthorBioStringFlattened(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakeAuthorBioString)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	author, err := c.GetAuthor(context.Background(), "OL999A")
	if err != nil {
		t.Fatal(err)
	}
	if author.Bio != "A plain bio string." {
		t.Errorf("Bio = %q, want plain string", author.Bio)
	}
}

func TestGetAuthorWorksParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeAuthorWorks)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	works, err := c.GetAuthorWorks(context.Background(), "OL26320A", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/authors/OL26320A/works.json") {
		t.Errorf("path = %q, want /authors/OL26320A/works.json", gotPath)
	}
	if len(works) != 2 {
		t.Fatalf("want 2 works, got %d", len(works))
	}
	if works[0].Key != "OL27516W" {
		t.Errorf("works[0].Key = %q, want OL27516W", works[0].Key)
	}
	if works[0].Title != "The Lord of the Rings" {
		t.Errorf("works[0].Title = %q", works[0].Title)
	}
}

// --- Subject ---

const fakeSubject = `{
  "name": "science fiction",
  "subject_type": "subject",
  "work_count": 21054,
  "works": [
    {
      "key": "/works/OL45340131W",
      "title": "Alice's Adventures in Wonderland",
      "authors": [{"key": "/authors/OL21594A", "name": "Lewis Carroll"}]
    },
    {
      "key": "/works/OL45804W",
      "title": "Dune",
      "authors": [{"name": "Frank Herbert"}]
    }
  ]
}`

func TestGetSubjectParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeSubject)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	works, err := c.GetSubject(context.Background(), "science_fiction", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/subjects/science_fiction.json") {
		t.Errorf("path = %q, want /subjects/science_fiction.json", gotPath)
	}
	if len(works) != 2 {
		t.Fatalf("want 2 works, got %d", len(works))
	}
	w := works[0]
	if w.Key != "OL45340131W" {
		t.Errorf("Key = %q, want OL45340131W", w.Key)
	}
	if w.Title != "Alice's Adventures in Wonderland" {
		t.Errorf("Title = %q", w.Title)
	}
	if len(w.Authors) != 1 || w.Authors[0] != "Lewis Carroll" {
		t.Errorf("Authors = %v", w.Authors)
	}
}

func TestGetSubjectSlugNormalized(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeSubject)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, _ = c.GetSubject(context.Background(), "science fiction", 3)
	if !strings.Contains(gotPath, "/subjects/science_fiction.json") {
		t.Errorf("path = %q, spaces should be converted to underscores", gotPath)
	}
}

// --- ISBN ---

const fakeISBN = `{
  "key": "/books/OL7394022M",
  "title": "Fantastic Mr. Fox",
  "publishers": ["Puffin"],
  "publish_date": "2009",
  "number_of_pages": 96,
  "isbn_10": ["0140328726"],
  "isbn_13": ["9780140328721"]
}`

func TestGetEditionByISBNParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeISBN)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	edition, err := c.GetEditionByISBN(context.Background(), "0140328726")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/isbn/0140328726.json") {
		t.Errorf("path = %q, want /isbn/0140328726.json", gotPath)
	}
	if edition.Key != "/books/OL7394022M" {
		t.Errorf("Key = %q", edition.Key)
	}
	if edition.Title != "Fantastic Mr. Fox" {
		t.Errorf("Title = %q", edition.Title)
	}
	if len(edition.Publishers) != 1 || edition.Publishers[0] != "Puffin" {
		t.Errorf("Publishers = %v", edition.Publishers)
	}
	if edition.PublishDate != "2009" {
		t.Errorf("PublishDate = %q", edition.PublishDate)
	}
	if edition.Pages != 96 {
		t.Errorf("Pages = %d, want 96", edition.Pages)
	}
	if len(edition.ISBN10) != 1 || edition.ISBN10[0] != "0140328726" {
		t.Errorf("ISBN10 = %v", edition.ISBN10)
	}
	if len(edition.ISBN13) != 1 || edition.ISBN13[0] != "9780140328721" {
		t.Errorf("ISBN13 = %v", edition.ISBN13)
	}
}

func TestGetEditionByISBNReturnsErrorOn404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.GetEditionByISBN(context.Background(), "0000000000")
	if err == nil {
		t.Error("expected error for 404 ISBN")
	}
}

// --- JSON marshal roundtrip for types ---

func TestTypesJSONMarshal(t *testing.T) {
	book := openlibrary.Book{
		Key:         "OL45804W",
		Title:       "Dune",
		Authors:     []string{"Frank Herbert"},
		PublishYear: 1965,
		ISBN:        []string{"0441013597"},
	}
	b, err := json.Marshal(book)
	if err != nil {
		t.Fatal(err)
	}
	var got openlibrary.Book
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Key != book.Key || got.Title != book.Title {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	work := openlibrary.Work{
		Key:         "OL45804W",
		Title:       "Dune",
		Description: "A desert planet.",
		Subjects:    []string{"Science fiction"},
		Covers:      []int{7979059},
	}
	wb, err := json.Marshal(work)
	if err != nil {
		t.Fatal(err)
	}
	var gotWork openlibrary.Work
	if err := json.Unmarshal(wb, &gotWork); err != nil {
		t.Fatal(err)
	}
	if gotWork.Key != work.Key {
		t.Errorf("Work roundtrip Key = %q", gotWork.Key)
	}
}
