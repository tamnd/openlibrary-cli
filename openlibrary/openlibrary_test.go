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
      "title": "Fantastic Mr Fox",
      "author_name": ["Roald Dahl"],
      "first_publish_year": 1970,
      "isbn": ["019279635X", "9780142410349"]
    },
    {
      "key": "/works/OL999W",
      "title": "Charlie and the Chocolate Factory",
      "author_name": ["Roald Dahl"],
      "first_publish_year": 1964,
      "isbn": []
    }
  ]
}`

func TestSearchBooksFlattensAuthorsAndISBN(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, fakeSearch)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	books, err := c.SearchBooks(context.Background(), "fantastic mr fox", 2)
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
	if b.Title != "Fantastic Mr Fox" {
		t.Errorf("Title = %q, want Fantastic Mr Fox", b.Title)
	}
	if b.Authors != "Roald Dahl" {
		t.Errorf("Authors = %q, want comma-joined string", b.Authors)
	}
	if b.FirstPublishYear != 1970 {
		t.Errorf("FirstPublishYear = %d, want 1970", b.FirstPublishYear)
	}
	if b.ISBN != "019279635X" {
		t.Errorf("ISBN = %q, want first ISBN only", b.ISBN)
	}

	// second book has no ISBN
	if books[1].ISBN != "" {
		t.Errorf("ISBN = %q, want empty when none", books[1].ISBN)
	}
}

func TestSearchBooksMultipleAuthorsJoined(t *testing.T) {
	const multiAuthor = `{"numFound":1,"docs":[{"key":"/works/OL1W","title":"Co-authored","author_name":["Alice","Bob"],"first_publish_year":2000,"isbn":[]}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, multiAuthor)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	books, err := c.SearchBooks(context.Background(), "co-authored", 1)
	if err != nil {
		t.Fatal(err)
	}
	if books[0].Authors != "Alice, Bob" {
		t.Errorf("Authors = %q, want Alice, Bob", books[0].Authors)
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
	_, _ = c.SearchBooks(context.Background(), "fantastic mr fox", 3)
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
  "title": "Fantastic Mr Fox",
  "description": {"type": "/type/text", "value": "The text of the story."},
  "subjects": ["Animals", "Foxes", "Fiction", "Juvenile fiction", "Children"],
  "first_publish_date": "1970"
}`

const fakeWorkStringDesc = `{
  "key": "/works/OL99W",
  "title": "Some Book",
  "description": "A plain string description.",
  "subjects": [],
  "first_publish_date": ""
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
	if work.Title != "Fantastic Mr Fox" {
		t.Errorf("Title = %q, want Fantastic Mr Fox", work.Title)
	}
	if work.Description != "The text of the story." {
		t.Errorf("Description = %q", work.Description)
	}
	// 5 subjects all joined
	if !strings.Contains(work.Subjects, "Animals") {
		t.Errorf("Subjects = %q, want Animals in there", work.Subjects)
	}
	if work.FirstPublish != "1970" {
		t.Errorf("FirstPublish = %q, want 1970", work.FirstPublish)
	}
}

func TestGetWorkSubjectsCappedAt5(t *testing.T) {
	const manySubjects = `{
		"key": "/works/OL1W",
		"title": "Book",
		"subjects": ["A","B","C","D","E","F","G"],
		"first_publish_date": "2000"
	}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, manySubjects)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	work, err := c.GetWork(context.Background(), "OL1W")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(work.Subjects, ", ")
	if len(parts) != 5 {
		t.Errorf("Subjects has %d parts, want 5", len(parts))
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
  "key": "/authors/OL34184A",
  "name": "Roald Dahl",
  "birth_date": "13 September 1916",
  "death_date": "23 November 1990",
  "bio": {"type": "/type/text", "value": "Roald Dahl was a British novelist."}
}`

const fakeAuthorBioString = `{
  "key": "/authors/OL999A",
  "name": "Some Author",
  "birth_date": "1970",
  "death_date": "",
  "bio": "A plain bio string."
}`

func TestGetAuthorParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeAuthor)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	author, err := c.GetAuthor(context.Background(), "OL34184A")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/authors/OL34184A.json") {
		t.Errorf("path = %q, want /authors/OL34184A.json", gotPath)
	}
	if author.Key != "OL34184A" {
		t.Errorf("Key = %q, want OL34184A (prefix stripped)", author.Key)
	}
	if author.Name != "Roald Dahl" {
		t.Errorf("Name = %q", author.Name)
	}
	if author.BirthDate != "13 September 1916" {
		t.Errorf("BirthDate = %q", author.BirthDate)
	}
	if author.DeathDate != "23 November 1990" {
		t.Errorf("DeathDate = %q", author.DeathDate)
	}
	if author.Bio != "Roald Dahl was a British novelist." {
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

// --- Editions ---

const fakeEditions = `{
  "entries": [
    {
      "key": "/books/OL7353617M",
      "title": "Fantastic Mr Fox",
      "publishers": ["Puffin Books"],
      "publish_date": "2007",
      "isbn_13": ["9780141311418"]
    },
    {
      "key": "/books/OL7353618M",
      "title": "Fantastic Mr Fox",
      "publishers": ["Knopf"],
      "publish_date": "2002",
      "isbn_13": []
    }
  ]
}`

func TestGetEditionsParses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = fmt.Fprint(w, fakeEditions)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	editions, err := c.GetEditions(context.Background(), "OL45804W", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, "/works/OL45804W/editions.json") {
		t.Errorf("path = %q, want /works/OL45804W/editions.json", gotPath)
	}
	if len(editions) != 2 {
		t.Fatalf("want 2 editions, got %d", len(editions))
	}

	e := editions[0]
	if e.Key != "/books/OL7353617M" {
		t.Errorf("Key = %q", e.Key)
	}
	if e.Title != "Fantastic Mr Fox" {
		t.Errorf("Title = %q", e.Title)
	}
	if e.Publisher != "Puffin Books" {
		t.Errorf("Publisher = %q, want first publisher", e.Publisher)
	}
	if e.Published != "2007" {
		t.Errorf("Published = %q", e.Published)
	}
	if e.ISBN13 != "9780141311418" {
		t.Errorf("ISBN13 = %q, want first isbn_13", e.ISBN13)
	}

	// second edition has no ISBN13
	if editions[1].ISBN13 != "" {
		t.Errorf("ISBN13 = %q, want empty when none", editions[1].ISBN13)
	}
}

func TestGetEditionsSendsLimit(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = fmt.Fprint(w, fakeEditions)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, _ = c.GetEditions(context.Background(), "OL45804W", 5)
	if !strings.Contains(gotQuery, "limit=5") {
		t.Errorf("query %q does not contain limit=5", gotQuery)
	}
}

func TestGetEditionsReturnsErrorOn404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.GetEditions(context.Background(), "OL0W", 10)
	if err == nil {
		t.Error("expected error for 404 work")
	}
}

// --- JSON marshal roundtrip for types ---

func TestTypesJSONMarshal(t *testing.T) {
	book := openlibrary.Book{
		Key:              "OL45804W",
		Title:            "Fantastic Mr Fox",
		Authors:          "Roald Dahl",
		FirstPublishYear: 1970,
		ISBN:             "019279635X",
	}
	b, err := json.Marshal(book)
	if err != nil {
		t.Fatal(err)
	}
	var got openlibrary.Book
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Key != book.Key || got.Title != book.Title || got.Authors != book.Authors {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	work := openlibrary.Work{
		Key:          "OL45804W",
		Title:        "Fantastic Mr Fox",
		Description:  "A story about a fox.",
		Subjects:     "Animals, Foxes, Fiction",
		FirstPublish: "1970",
	}
	wb, err := json.Marshal(work)
	if err != nil {
		t.Fatal(err)
	}
	var gotWork openlibrary.Work
	if err := json.Unmarshal(wb, &gotWork); err != nil {
		t.Fatal(err)
	}
	if gotWork.Key != work.Key || gotWork.Subjects != work.Subjects {
		t.Errorf("Work roundtrip mismatch: %+v", gotWork)
	}

	author := openlibrary.Author{
		Key:       "OL34184A",
		Name:      "Roald Dahl",
		BirthDate: "13 September 1916",
		DeathDate: "23 November 1990",
		Bio:       "Roald Dahl was a British novelist.",
	}
	ab, err := json.Marshal(author)
	if err != nil {
		t.Fatal(err)
	}
	var gotAuthor openlibrary.Author
	if err := json.Unmarshal(ab, &gotAuthor); err != nil {
		t.Fatal(err)
	}
	if gotAuthor.Key != author.Key || gotAuthor.Name != author.Name {
		t.Errorf("Author roundtrip mismatch: %+v", gotAuthor)
	}

	edition := openlibrary.Edition{
		Key:       "/books/OL7353617M",
		Title:     "Fantastic Mr Fox",
		Publisher: "Puffin Books",
		Published: "2007",
		ISBN13:    "9780141311418",
	}
	eb, err := json.Marshal(edition)
	if err != nil {
		t.Fatal(err)
	}
	var gotEdition openlibrary.Edition
	if err := json.Unmarshal(eb, &gotEdition); err != nil {
		t.Fatal(err)
	}
	if gotEdition.Key != edition.Key || gotEdition.Publisher != edition.Publisher {
		t.Errorf("Edition roundtrip mismatch: %+v", gotEdition)
	}
}
