---
title: "Add a new site"
description: "Teach ant a site with a one-line driver import."
weight: 20
---

`ant` learns a site the way `database/sql` learns a database: a blank import
that registers a driver. The site CLI ships a `Domain`, and `ant` links it in.

## The one line

In [`cli/root.go`](https://github.com/tamnd/ant/blob/main/cli/root.go), the
drivers are blank imports:

```go
import (
    _ "github.com/tamnd/goodread-cli/goodread"
    _ "github.com/tamnd/x-cli/x"
)
```

Each import runs the driver's `init()`, which calls `kit.Register`. After a
rebuild, every verb works for that site. Confirm with:

```bash
ant domains
```

## What a driver provides

A driver is a small package in the site CLI. It declares its records with struct
tags so the engine knows how to address and link them:

```go
type Book struct {
    ID       string `json:"id"        kit:"id"`
    Title    string `json:"title"`
    Blurb    string `json:"blurb"     kit:"body"`
    AuthorID string `json:"author_id" kit:"link,kind=goodreads/author"`
}
```

- `kit:"id"` marks the identifier that becomes the URI path.
- `kit:"link,kind=<scheme>/<type>"` marks an outbound edge. Edges can point at
  any site, which is what makes the graph cross-site.
- `kit:"body"` marks the prose `cat` and `-o md` render.

The driver also maps a URL or bare id to a URI (`resolve`) and back (`url`), and
points each record type at the site operation that fetches it. The site CLI
keeps working as its own binary, untouched — the driver is purely additive.

There is nothing to wire beyond the import: the same `get`, `links`, `export`,
`graph`, `serve`, and `mcp` verbs cover the new site the moment it registers.
