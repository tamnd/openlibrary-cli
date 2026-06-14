package openlibrary

import (
	"context"
	"fmt"

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
		Args:    []kit.Arg{{Name: "query", Help: "search query"}}}, searchBooks)

	// subjects: list books in a subject category
	kit.Handle(app, kit.OpMeta{Name: "subjects", Group: "read", List: true,
		Summary: "List books in a subject category",
		Args:    []kit.Arg{{Name: "subject", Help: "subject name (e.g. \"computer science\")"}}}, subjectBooks)

	// authors: search authors by name
	kit.Handle(app, kit.OpMeta{Name: "authors", Group: "read", List: true,
		Summary: "Search authors by name",
		Args:    []kit.Arg{{Name: "query", Help: "author name"}}}, searchAuthorsOp)

	// author: fetch author detail by OL ID
	kit.Handle(app, kit.OpMeta{Name: "author", Group: "read", Single: true,
		Summary: "Get author detail by OL ID (e.g. OL23919A)",
		Args:    []kit.Arg{{Name: "olid", Help: "OL author ID"}}}, getAuthorOp)

	// work: fetch work detail by OL ID
	kit.Handle(app, kit.OpMeta{Name: "work", Group: "read", Single: true,
		Summary: "Get work detail by OL ID (e.g. OL45804W)",
		Args:    []kit.Arg{{Name: "olid", Help: "OL work ID"}}}, getWorkOp)
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
	Limit  int     `kit:"flag,inherit" help:"max results (default 20)"`
	Client *Client `kit:"inject"`
}

type subjectInput struct {
	Subject string  `kit:"arg" help:"subject name"`
	Limit   int     `kit:"flag,inherit" help:"max results (default 20)"`
	Client  *Client `kit:"inject"`
}

type authorsInput struct {
	Query  string  `kit:"arg"          help:"author name"`
	Limit  int     `kit:"flag,inherit" help:"max results (default 10)"`
	Client *Client `kit:"inject"`
}

type authorInput struct {
	OLID   string  `kit:"arg"    help:"OL author ID (e.g. OL23919A)"`
	Client *Client `kit:"inject"`
}

type workInput struct {
	OLID   string  `kit:"arg"    help:"OL work ID (e.g. OL45804W)"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchBooks(ctx context.Context, in searchInput, emit func(*Book) error) error {
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

func searchAuthorsOp(ctx context.Context, in authorsInput, emit func(*Author) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
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

func getAuthorOp(ctx context.Context, in authorInput, emit func(*Author) error) error {
	author, err := in.Client.GetAuthor(ctx, in.OLID)
	if err != nil {
		return mapErr(err)
	}
	return emit(author)
}

func getWorkOp(ctx context.Context, in workInput, emit func(*Work) error) error {
	work, err := in.Client.GetWork(ctx, in.OLID)
	if err != nil {
		return mapErr(err)
	}
	return emit(work)
}

func subjectBooks(ctx context.Context, in subjectInput, emit func(*Book) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	books, err := in.Client.Subject(ctx, in.Subject, limit)
	if err != nil {
		return mapErr(err)
	}
	if len(books) == 0 {
		return errs.NotFound("no books found for subject %q", in.Subject)
	}
	for i := range books {
		if err := emit(&books[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: pure string functions, no network ---

// Classify turns any accepted input into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	if input == "" {
		return "", "", errs.Usage("empty openlibrary reference")
	}
	return "book", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "book":
		return fmt.Sprintf("https://openlibrary.org/works/%s", id), nil
	default:
		return "", errs.Usage("openlibrary has no resource type %q", uriType)
	}
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
