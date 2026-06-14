---
title: "Quick start"
description: "Dereference your first URI."
weight: 30
---

Once `ant` is on your `PATH`, see which sites this binary can address:

```bash
ant domains
```

## Resolve, then dereference

Turn a messy URL into its canonical URI, then fetch the record:

```bash
$ ant resolve "https://www.goodreads.com/book/show/2767052-the-hunger-games"
goodreads://book/2767052

$ ant get goodreads://book/2767052
{
  "@id": "goodreads://book/2767052",
  "@type": "goodreads/book",
  "@fetched": "2026-06-14T09:31:00Z",
  "@links": { "author": ["goodreads://author/153394"] },
  "data": { ... }
}
```

`resolve` also takes a bare id when you say which site it belongs to:

```bash
$ ant resolve 20 --on x
x://status/20
$ ant url x://user/nasa
https://x.com/nasa
```

## Follow the links

`links` prints a record's outbound edges as URIs, and `export` walks them and
writes each record to disk as the URI tree, where the file path is the URI:

```bash
$ ant links goodreads://book/2767052
goodreads://author/153394

$ ant export goodreads://author/153394 --follow 1 --to ./data
$ ant ll goodreads:// --data ./data
goodreads://author/153394
goodreads://book/2767052
```

## Serve the namespace

Expose the whole namespace as dereferenceable linked data over HTTP, or as an
MCP tool set for agents:

```bash
$ ant serve --addr :7777 &
$ curl -s 'localhost:7777/resolve?input=https://x.com/nasa'
{"uri":"x://user/nasa"}
$ ant mcp     # speaks MCP over stdio: get/ls/links/resolve/url/domains
```

Every command's full flag list is one `--help` away, e.g. `ant export --help`.
