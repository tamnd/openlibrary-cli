# openlibrary

A command line for openlibrary.

`openlibrary` is a single pure-Go binary. It reads public openlibrary data
over plain HTTPS, shapes it into clean records, and prints output that pipes
into the rest of your tools. No API key, nothing to run alongside it.

The same package is also a [resource-URI driver](#use-it-as-a-resource-uri-driver),
so a host program like [ant](https://github.com/tamnd/ant) can address
openlibrary as `openlibrary://` URIs.

## Install

```bash
go install github.com/tamnd/openlibrary-cli/cmd/openlibrary@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/openlibrary-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/openlibrary:latest --help
```

## Usage

```bash
openlibrary page <path>                      # fetch one page as a record
openlibrary page <path> -o json              # as JSON, ready for jq
openlibrary page <path> --template '{{.Body}}'  # just the readable body text
openlibrary links <path>                     # the pages it links to, one per line
openlibrary --help                           # the whole command tree
```

Every command shares one output contract: `-o table|json|jsonl|csv|tsv|url|raw`,
`--fields` to pick columns, `--template` for a custom line, and `-n` to limit.
The default adapts to where output goes (a table on a terminal, JSONL in a
pipe), so the same command reads well by hand and parses cleanly downstream.

This is a fresh scaffold. It ships one example resource type, `page`, wired end
to end. Model the real openlibrary records in `openlibrary/` and declare their
operations in `openlibrary/domain.go`; each one becomes a command, an HTTP
route, and an MCP tool at once.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents,
with no extra code:

```bash
openlibrary serve --addr :7777    # GET /v1/page/<path>  returns NDJSON
openlibrary mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`openlibrary` registers a `openlibrary` domain the way a program registers a
database driver with `database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/openlibrary-cli/openlibrary"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `openlibrary://` URIs without knowing anything about openlibrary:

```bash
ant get openlibrary://page/<path>   # fetch the record
ant cat openlibrary://page/<path>   # just the body text
ant ls  openlibrary://page/<path>   # the pages it links to, each addressable
ant url openlibrary://page/<path>   # the live https URL
```

## Development

```
cmd/openlibrary/   thin main: hands cli.NewApp to kit.Run
cli/                 assembles the kit App from the openlibrary domain
openlibrary/                the library: HTTP client, data models, and domain.go (the driver)
docs/                tago documentation site
```

```bash
make build      # ./bin/openlibrary
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag v0.1.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
