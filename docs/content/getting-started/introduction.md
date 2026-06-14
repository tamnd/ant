---
title: "Introduction"
description: "What ant is and how it is put together."
weight: 10
---

`ant` is a single binary that puts one URI namespace over the whole
`tamnd/*-cli` family. A book on Goodreads, an account on X — each is a short
URI, and `ant` dereferences it: it fetches the record, follows the typed links
out of it, and writes the slice of the graph you asked for to disk. There is
nothing to sign up for and nothing to run alongside it.

## The model

A resource URI is `scheme://authority/path`. The scheme picks the site, the
authority picks the record type, the path is the id:

```
goodreads://book/2767052
goodreads://author/153394
x://user/nasa
x://status/20
```

Every verb takes a URI and does one thing — `resolve` normalizes any id or URL
to its URI, `get` dereferences it, `links` lists its outbound edges, `export`
walks those edges and writes the records to disk as the URI tree, where a
record's file path *is* its URI.

## How it is built

- A **library package** (`ant`) holds the `Engine`: a thin layer over a
  `kit.Host` that knows every registered site. It dereferences URIs, follows
  links, and materializes the URI tree.
- A set of **domain drivers** teach `ant` each site. A driver is a blank import,
  the way `database/sql` learns a database — one line links the site CLI in as a
  Go library, and its records become addressable.
- A **command tree** (`cli`) wraps the engine in subcommands with shared output
  formats and flags, and one **`cmd/ant`** entry point ties them together.

## Scope

`ant` is a read-only front door over data the sites already serve publicly. It
reads that data and shapes it for you. That narrow scope keeps it a single small
binary with no database, no daemon, and no setup.

Next: [install it](/getting-started/installation/), then take the
[quick start](/getting-started/quick-start/).
