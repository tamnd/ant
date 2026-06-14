---
title: "CLI"
description: "Every verb, with the flags that matter."
weight: 10
---

```
ant <command> <uri> [flags]
```

Run `ant <command> --help` for the full flag list on any command.

## Global flags

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--data` | `ANT_DATA` | `$HOME/data` | root of the on-disk URI tree |
| `--output` / `-o` | | `json` | `json`, or `md` to render record bodies as Markdown |

## Reading the live web

| Command | What it does |
|---|---|
| `resolve <input> [--on <scheme>]` | normalize any id, URL, or URI to its canonical URI |
| `url <uri>` | the live https location for a URI (inverse of `resolve`) |
| `get <uri>` | dereference a URI: fetch and print the record envelope |
| `cat <uri>` | print just the record's body (Markdown for text, JSON otherwise) |
| `ls <uri> [-n N]` | list the members of a collection URI |
| `links <uri>` | print the outbound link URIs, the graph edges |
| `open <uri>` | open the URI's live URL in a browser |
| `domains` | list the registered domains this binary can address |

## The on-disk URI tree

A record's file path under `--data` is its URI:
`$HOME/data/goodreads/book/2767052.json`.

| Command | What it does |
|---|---|
| `export <uri> [--follow N] [--to DIR] [--md]` | materialize a resource and its links to the data tree |
| `import <path>` | read an exported record file back as a record |
| `ll [<uri-prefix>]` | list what is already on disk under a URI prefix |
| `graph <uri> [--depth N] [--format dot\|json]` | walk links to depth N and print the subgraph |

## Serving the namespace

| Command | What it does |
|---|---|
| `serve [--addr :7777]` | dereference server: HTTP GET on a URI path returns its record; `/resolve`, `/url`, `/ls`, `/links` endpoints |
| `tui [<uri>]` | full-screen terminal browser over the namespace: follow links, list members, walk the graph, browse the cache, all keyboard-driven |
| `mcp` | the same namespace as an MCP tool set over stdio: get/ls/links/url/resolve/domains |
| `version` | print the version and exit |

## The record envelope

`get` returns a stable envelope around the site's data:

```json
{
  "@id": "goodreads://book/2767052",
  "@type": "goodreads/book",
  "@fetched": "2026-06-14T09:31:00Z",
  "@links": { "author": ["goodreads://author/153394"] },
  "data": { }
}
```

`@links` are grouped by the field they came from, so the same shape drives
`links`, `export --follow`, and `graph`.
