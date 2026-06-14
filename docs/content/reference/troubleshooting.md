---
title: "Troubleshooting"
description: "The handful of things that trip people up, and how to fix each one."
weight: 40
---

Most of these come down to network reality or how a site serves its data, not a
bug in `ant`.

## "Blocked: page requires sign-in" or a WAF challenge

Some sites gate their pages behind a sign-in wall or a WAF that turns away
datacenter IPs. `ant` surfaces that as an error rather than guessing, and an
`export` records it per-URI under `errors` while still writing everything it
could reach. Run from a residential IP, or supply the session the site CLI
accepts. This is the site refusing the request, not a defect.

## A 429 or a burst of 5xx

That is the site asking you to slow down. Each domain driver paces its own
requests, but a hard limit still means backing off: narrow your `--follow`
depth, space out repeated runs, and retry later.

## resolve cannot place a bare id

A bare id or `@handle` is ambiguous until you name the site:

```bash
ant resolve 20            # error: which site?
ant resolve 20 --on x     # x://status/20
```

A full URL or URI never needs `--on`, because the scheme or host names the site.

## "no domain for scheme"

The URI names a site this binary was not built with. Run `ant domains` to see
what it can address. Adding a site is a one-line blank import in `cli/root.go`,
then a rebuild.

## The binary is not on your PATH

`go install` puts the binary in `$(go env GOPATH)/bin` (usually `~/go/bin`), and
a release archive leaves it wherever you unpacked it. If your shell cannot find
`ant`, add that directory to your `PATH`. See
[installation](/getting-started/installation/).
