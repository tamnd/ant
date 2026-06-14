---
title: "Crawl a slice of the graph to disk"
description: "Follow typed links across sites and materialize the URI tree."
weight: 10
---

`get` fetches one record. `export` fetches a record, follows its links, and
writes the whole reachable slice to disk as the URI tree, so a record's file
path *is* its URI. That tree is plain files you can grep, diff, or check into
git.

## Start from one URI

Pick a seed and see its outbound edges first:

```bash
$ ant links goodreads://author/153394
goodreads://book/2767052
goodreads://book/6148028
```

## Materialize it

`--follow N` walks links to depth N; `--to` chooses where to write (otherwise
`--data` / `$HOME/data`); `--md` drops a Markdown companion next to records that
carry prose:

```bash
$ ant export goodreads://author/153394 --follow 1 --to ./data --md
{
  "root": "./data",
  "written": [
    "./data/goodreads/author/153394.json",
    "./data/goodreads/book/2767052.json",
    "./data/goodreads/book/2767052.md"
  ],
  "skipped": [],
  "errors": {}
}
```

The report is honest: a URI a site refuses (a sign-in wall, a WAF) lands under
`errors` with the reason, and everything reachable is still written.

## Read the tree back

```bash
$ ant ll goodreads:// --data ./data
goodreads://author/153394
goodreads://book/2767052
$ ant import ./data/goodreads/book/2767052.json | jq '.["@type"]'
"goodreads/book"
```

## Draw it

`graph` walks the same edges and prints a subgraph as JSON or Graphviz `dot`:

```bash
$ ant graph goodreads://author/153394 --depth 1 --format dot | dot -Tsvg > graph.svg
```

Because links are typed and cross-site, a seed on one site can pull in records
from another wherever the data points across — the URI tree holds them all under
one root.
