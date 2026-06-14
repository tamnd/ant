---
title: "Output"
description: "The record envelope, JSON and Markdown, and the data tree on disk."
weight: 30
---

`ant` speaks JSON by default. Every record comes back in the same envelope, so
one shape drives `get`, `links`, `export`, and `graph` no matter which site the
record came from.

## The envelope

```json
{
  "@id": "goodreads://book/2767052",
  "@type": "goodreads/book",
  "@fetched": "2026-06-14T09:31:00Z",
  "@links": { "author": ["goodreads://author/153394"] },
  "data": { "title": "The Hunger Games", "...": "..." }
}
```

- `@id` is the canonical URI of the record.
- `@type` is `scheme/authority`, the site and record type.
- `@fetched` is when `ant` dereferenced it.
- `@links` groups the outbound edges by the field they came from. These are the
  same URIs `links` prints and `export --follow` walks.
- `data` is the site's record, untouched.

It pipes straight into `jq`:

```bash
ant get goodreads://book/2767052 | jq '.["@links"]'
ant ls x://user/nasa -n 5 | jq '.[].["@id"]'
```

## Markdown bodies

Records that carry prose tag it as the body. `cat` prints just that, and `-o md`
renders it as Markdown wherever a body exists, falling back to JSON otherwise:

```bash
ant cat x://status/20            # the post text
ant get goodreads://book/123 -o md   # the description as Markdown
```

`export --md` writes a companion `.md` file with JSON front-matter next to each
record that has a body.

## The data tree

`export` materializes records under `--data` (default `$HOME/data`) so that a
record's file path *is* its URI:

```
$HOME/data/goodreads/book/2767052.json
$HOME/data/goodreads/author/153394.json
$HOME/data/x/user/nasa.json
```

`ll` lists what is already there by reading the tree back into URIs, and
`import` reads one file back into the envelope. The tree is plain files: browse
it, grep it, or check it into git.
