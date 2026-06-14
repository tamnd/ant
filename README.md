# ant

Every public record is a URI, and `ant` dereferences it.

`ant` is a single pure-Go binary that puts one address space over the whole
`tamnd/*-cli` family. A book on Goodreads, an account on X, an article on
Wikipedia, a video on YouTube — each is a short URI like `goodreads://book/2767052`,
`x://user/nasa`, `wikipedia://page/Alan_Turing`, or `youtube://video/dQw4w9WgXcQ`.
Hand one to `ant` and
it fetches the record, follows the typed links out of it, and writes the slice
of the graph you asked for to disk, no matter which site each name lives on.

No API key, no daemon, nothing to run alongside it. The site CLIs are linked in
as Go libraries, so `ant` is one static binary that already knows how to read
every site it was built with.

```bash
ant get goodreads://book/2767052          # fetch and print the record
ant resolve "https://x.com/nasa"          # x://user/nasa
ant links goodreads://book/2767052        # the outbound edges, as URIs
ant export goodreads://author/153394 --follow 1   # write the subgraph to disk
```

## Install

```bash
go install github.com/tamnd/ant/cmd/ant@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/ant/releases),
or run the container image:

```bash
docker run --rm ghcr.io/tamnd/ant:latest --help
```

## The idea

A resource URI is `scheme://authority/path`. The scheme picks the site, the
authority picks the record type, the path is the id:

```
goodreads://book/2767052
goodreads://author/153394
x://user/nasa
x://status/20
wikipedia://page/Alan_Turing
wikipedia://category/Computability_theory
youtube://video/dQw4w9WgXcQ
youtube://channel/UCuAXFkgsw1L7xaCfnd5JJOw
```

Every verb takes a URI and does one thing:

| Verb | What it does |
|------|--------------|
| `resolve <input> [--on <scheme>]` | normalize any id, URL, or URI to its canonical URI |
| `url <uri>` | the live https location (inverse of `resolve`) |
| `get <uri>` | dereference: fetch and print the record |
| `cat <uri>` | print just the record's body (Markdown for text, JSON otherwise) |
| `ls <uri> [-n N]` | list the members of a collection URI |
| `links <uri>` | print the outbound link URIs, the graph edges |
| `open <uri>` | open the URI's live URL in a browser |
| `domains` | list the sites this binary can address |

And the graph verbs work over the on-disk **URI tree** under `$HOME/data`,
where a record's file path is its URI:

| Verb | What it does |
|------|--------------|
| `export <uri> [--follow N] [--md]` | materialize a resource and its links to the data tree |
| `import <path>` | read an exported record file back |
| `ll [<uri-prefix>]` | list what is already on disk under a prefix |
| `graph <uri> [--depth N] [--format dot\|json]` | walk links and print the subgraph |
| `serve [--addr :7777]` | a dereference server: HTTP GET on a URI returns its record |
| `mcp` | the same namespace as an MCP tool set for agents |

## Walkthrough

Resolve a messy URL to its URI, then dereference it:

```bash
$ ant resolve "https://www.goodreads.com/book/show/2767052-the-hunger-games"
goodreads://book/2767052

$ ant get goodreads://book/2767052
{
  "@id": "goodreads://book/2767052",
  "@type": "goodreads/book",
  "@fetched": "2026-06-14T09:31:00Z",
  "@links": {
    "author": ["goodreads://author/153394"]
  },
  "data": { ... }
}
```

Cross a site boundary without changing tools — the same `resolve`/`get` work on
X:

```bash
$ ant resolve 20 --on x
x://status/20
$ ant url x://user/nasa
https://x.com/nasa
```

Walk the graph and write it to disk as the URI tree:

```bash
$ ant export goodreads://author/153394 --follow 1 --to ./data
$ ant ll goodreads:// --data ./data
goodreads://author/153394
goodreads://book/2767052
...
$ ant graph goodreads://author/153394 --depth 1 --format dot | dot -Tsvg > graph.svg
```

Serve the namespace as dereferenceable linked data:

```bash
$ ant serve --addr :7777 &
$ curl -s localhost:7777/goodreads://book/2767052 | jq .['@type']
"goodreads/book"
$ curl -s 'localhost:7777/resolve?input=https://x.com/nasa'
{"uri":"x://user/nasa"}
```

## Adding a site

`ant` learns a new site the way `database/sql` learns a new database — a blank
import that registers a driver. The site CLI exposes a `Domain`, and `ant`
links it in with one line in [`cli/root.go`](cli/root.go):

```go
import (
    _ "github.com/tamnd/goodread-cli/goodread"
    _ "github.com/tamnd/wikipedia-cli/wiki"
    _ "github.com/tamnd/x-cli/x"
    _ "github.com/tamnd/ytb-cli/youtube"
)
```

The driver declares its records with struct tags — `kit:"id"` marks the
identifier, `kit:"link,kind=goodreads/author"` marks an edge, `kit:"body"`
marks the prose — and the verbs above work for free. See
[`8000_uri_drivers.md`](https://github.com/tamnd/any-cli) for the mechanism.

## Configuration

| Flag | Env | Default | Meaning |
|------|-----|---------|---------|
| `--data` | `ANT_DATA` | `$HOME/data` | root of the on-disk URI tree |
| `--output` / `-o` | | `json` | `json` or `md` (render bodies as Markdown) |

## Development

```
cmd/ant/   thin main, wires cli.Root into fang
cli/                 the cobra command tree, one file per verb group
ant/                 the library: the Engine over a kit.Host of registered domains
docs/                tago documentation site
```

```bash
make build      # ./bin/ant
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
