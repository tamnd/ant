---
title: "Configuration"
description: "Environment variables and global flags."
weight: 20
---

`ant` needs almost no configuration. The few knobs it has are global flags.

## Global flags

| Flag | Meaning |
|---|---|
| `--data` | root of the on-disk URI tree (default `$HOME/data`) |
| `--output` / `-o` | `json` (default), or `md` to render record bodies as Markdown |
| `--help` | help for any command |
| `--version` | print the version |

## Environment variables

| Variable | Meaning |
|---|---|
| `ANT_DATA` | default for `--data`, the root of the URI tree |

Each domain driver may read its own environment for the site it speaks to (for
example a session cookie or token). Those are documented with the site CLI the
driver comes from; `ant` passes the environment through unchanged.
