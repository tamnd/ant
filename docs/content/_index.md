---
title: "ant"
description: "Every public record is a URI, and ant dereferences it."
heroTitle: "One address space over every site"
heroLead: "ant puts a single URI namespace over the tamnd site CLIs. Dereference a record, follow its links across sites, and write the graph to disk. One pure-Go binary, no API key."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

A public record is a URI: `goodreads://book/2767052`, `x://user/nasa`,
`wikipedia://page/Alan_Turing`. Hand one to `ant` and it fetches the record,
follows the typed links out of it, and
materializes the slice of the graph you asked for, regardless of which site each
name lives on.

```bash
ant get goodreads://book/2767052      # fetch and print the record
ant resolve "https://x.com/nasa"      # x://user/nasa
ant cat wikipedia://page/Alan_Turing  # the article text
ant links goodreads://book/2767052    # the outbound edges, as URIs
ant export x://user/nasa --follow 1   # write the subgraph to disk
```

The site CLIs are linked in as Go libraries, so `ant` is a single static binary
that already knows how to read every site it was built with. No daemon, nothing
to run alongside it.

## Where to go next

- New here? Read the [introduction](/getting-started/introduction/), then the
  [quick start](/getting-started/quick-start/).
- Installing? See [installation](/getting-started/installation/).
- Need every verb? The [CLI reference](/reference/cli/) is the full surface.
