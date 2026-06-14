package openlibrary

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes openlibrary as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/openlibrary-cli/openlibrary"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// openlibrary:// URIs by routing to the operations Register installs.
func init() { kit.Register(Domain{}) }

// Domain is the openlibrary driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "openlibrary",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "openlibrary",
			Short:  "Browse Open Library from the command line",
			Long: `A command line for Open Library (openlibrary.org).

openlibrary reads the Open Library catalog over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/openlibrary-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// search: search books by query string
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search books by query",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}}}, searchOp)

	// book: fetch book by ISBN
	kit.Handle(app, kit.OpMeta{Name: "book", Group: "read", Single: true,
		Summary: "Get book by ISBN (hyphens optional, e.g. 9780140328721)",
		Args:    []kit.Arg{{Name: "isbn", Help: "ISBN-10 or ISBN-13 (hyphens stripped automatically)"}}}, bookOp)

	// work: fetch work detail by OL ID
	kit.Handle(app, kit.OpMeta{Name: "work", Group: "read", Single: true,
		Summary: "Get work detail by OL ID (e.g. OL45804W)",
		Args:    []kit.Arg{{Name: "id", Help: "work key e.g. OL45804W (without /works/ prefix)"}}}, workOp)

	// author: fetch author detail by OL ID
	kit.Handle(app, kit.OpMeta{Name: "author", Group: "read", Single: true,
		Summary: "Get author detail by OL ID (e.g. OL34184A)",
		Args:    []kit.Arg{{Name: "id", Help: "author key e.g. OL34184A (without /authors/ prefix)"}}}, authorOp)

	// search-authors: search authors by query string
	kit.Handle(app, kit.OpMeta{Name: "search-authors", Group: "read", List: true,
		Summary: "Search authors by query",
		Args:    []kit.Arg{{Name: "query", Help: "author name or query"}}}, searchAuthorsOp)

	// editions: list editions of a work
	kit.Handle(app, kit.OpMeta{Name: "editions", Group: "read", List: true,
		Summary: "List editions of a work by OL ID (e.g. OL45804W)",
		Args:    []kit.Arg{{Name: "id", Help: "work key e.g. OL45804W"}}}, editionsOp)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient(DefaultConfig())
	if cfg.UserAgent != "" {
		c.cfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.cfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.cfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.cfg.Timeout = cfg.Timeout
		c.http.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg" help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client *Client `kit:"inject"`
}

type bookInput struct {
	ISBN   string  `kit:"arg" help:"ISBN-10 or ISBN-13 (hyphens stripped automatically)"`
	Client *Client `kit:"inject"`
}

type workInput struct {
	ID     string  `kit:"arg" help:"work key e.g. OL45804W (without /works/ prefix)"`
	Client *Client `kit:"inject"`
}

type authorInput struct {
	ID     string  `kit:"arg" help:"author key e.g. OL34184A (without /authors/ prefix)"`
	Client *Client `kit:"inject"`
}

type searchAuthorsInput struct {
	Query  string  `kit:"arg" help:"author name or query"`
	Limit  int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client *Client `kit:"inject"`
}

type editionsInput struct {
	ID     string  `kit:"arg" help:"work key e.g. OL45804W"`
	Limit  int     `kit:"flag,inherit" help:"max editions" default:"10"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchOp(ctx context.Context, in searchInput, emit func(*Book) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	books, err := in.Client.SearchBooks(ctx, in.Query, limit)
	if err != nil {
		return mapErr(err)
	}
	if len(books) == 0 {
		return errs.NotFound("no books found for %q", in.Query)
	}
	for i := range books {
		if err := emit(&books[i]); err != nil {
			return err
		}
	}
	return nil
}

func bookOp(ctx context.Context, in bookInput, emit func(*Book) error) error {
	book, err := in.Client.GetBookByISBN(ctx, in.ISBN)
	if err != nil {
		return mapErr(err)
	}
	return emit(book)
}

func workOp(ctx context.Context, in workInput, emit func(*Work) error) error {
	work, err := in.Client.GetWork(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(work)
}

func authorOp(ctx context.Context, in authorInput, emit func(*Author) error) error {
	author, err := in.Client.GetAuthor(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(author)
}

func searchAuthorsOp(ctx context.Context, in searchAuthorsInput, emit func(*Author) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	authors, err := in.Client.SearchAuthors(ctx, in.Query, limit)
	if err != nil {
		return mapErr(err)
	}
	if len(authors) == 0 {
		return errs.NotFound("no authors found for %q", in.Query)
	}
	for i := range authors {
		if err := emit(&authors[i]); err != nil {
			return err
		}
	}
	return nil
}

func editionsOp(ctx context.Context, in editionsInput, emit func(*Edition) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	editions, err := in.Client.GetEditions(ctx, in.ID, limit)
	if err != nil {
		return mapErr(err)
	}
	if len(editions) == 0 {
		return errs.NotFound("no editions found for work %q", in.ID)
	}
	for i := range editions {
		if err := emit(&editions[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: pure string functions, no network ---

// Classify turns any accepted input into the canonical (type, id).
//
//   - starts with "OL" and ends in "A" (like OL34184A) -> ("author", input)
//   - starts with "OL" and ends in "W" (like OL45804W) -> ("work", input)
//   - all digits, 10 or 13 chars -> ("isbn", input)
//   - otherwise -> ("query", input)
func (Domain) Classify(input string) (uriType, id string, err error) {
	if input == "" {
		return "", "", errs.Usage("empty openlibrary reference")
	}
	upper := strings.ToUpper(input)
	if strings.HasPrefix(upper, "OL") {
		if strings.HasSuffix(upper, "A") {
			return "author", input, nil
		}
		if strings.HasSuffix(upper, "W") {
			return "work", input, nil
		}
	}
	if isDigits(input) && (len(input) == 10 || len(input) == 13) {
		return "isbn", input, nil
	}
	return "query", input, nil
}

// isDigits reports whether s consists entirely of ASCII digits.
func isDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "author":
		return fmt.Sprintf("https://openlibrary.org/authors/%s", id), nil
	case "work":
		return fmt.Sprintf("https://openlibrary.org/works/%s", id), nil
	case "isbn":
		return fmt.Sprintf("https://openlibrary.org/isbn/%s", id), nil
	case "query":
		return fmt.Sprintf("https://openlibrary.org/search?q=%s", id), nil
	default:
		return "", errs.Usage("openlibrary has no resource type %q", uriType)
	}
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
