# Command output contracts

This page identifies where each command writes output, which project owns its
shape, and how exit status relates to that output.

| Command | stdout | stderr | Exit 0 | Exit 1 | Exit 2 | Contract |
|---|---|---|---|---|---|---|
| `search` | JSON, or a save confirmation with `--output` | Errors | Complete collection or explicit CLI-limit stop | Incomplete collection or runtime/write failure | Invalid arguments | [Generated search reference](reference/search-output.md) |
| `user-posts` | JSON, or a save confirmation with `--output` | Errors | Complete collection or explicit CLI-limit stop | Incomplete collection or runtime/write failure | Invalid arguments | [Generated user-posts reference](reference/user-posts-output.md) |
| `bookmarks list` | Pretty-printed X API JSON | Errors | Request and output succeeded, even if an HTTP 200 body contains upstream `errors` | Runtime/API failure | Invalid arguments | [X API response](https://docs.x.com/x-api/bookmarks/get-bookmarks) (pass-through) |
| `auth login` | Browser URL and token-save confirmation text | Errors | Token saved | Login, callback, exchange, or write failure | Invalid arguments | Human-readable text; no JSON contract |

## Search lifecycle

`search` owns the post-oriented format in `schemas/search.schema.json`; its Go model
and field reference are generated from that schema. Argument validation, token
loading, and a failed preflight refresh happen before collection, so they produce no
JSON. After the first API request is attempted, a failure writes an `incomplete`
document and exits 1. Successful earlier pages and usable `data` accompanying HTTP
200 partial `errors` are retained. An initial failure and a successful zero-result
search both contain `posts: []`, but their statuses differ.

`complete` means X returned no next page among the results accessible to this request;
it is not a completeness guarantee for X as a whole or even the approximately
seven-day Recent Search window. `limit_reached` means the CLI stopped at `--limit`
while more data was available. Both successful statuses exit 0. Authentication or
access denial, rate limiting, other API errors, and transport/request failures use
distinct `incomplete_reason` values and exit 1.

Without `--output`, the JSON document is written to stdout. With `--output FILE`,
only JSON is written to the mode-`0600` file and stdout receives a human-readable
save confirmation. Diagnostics go to stderr and are never mixed into JSON.

## User Posts lifecycle

`user-posts` owns the format defined by `schemas/user-posts.schema.json`. Invalid
arguments, token loading, and a failed preflight refresh happen before collection
and do not produce JSON. Once username resolution is attempted, failures produce
an `incomplete` document. The `authentication_or_access_denied`, `rate_limited`,
`api_error`, and `request_failed` reasons distinguish terminal categories. A
later-page failure or an HTTP 200 partial error retains posts already received.

`complete` means X returned no next page within the upstream-accessible timeline.
It does not assert that posts exist beyond X's most recent-3,200-post window or
that the caller can see protected posts. `limit_reached` means the CLI stopped at
the requested limit while more response data or a next-page token existed. Both
successful statuses exit 0; `incomplete` exits 1. Zero results are represented by
`post_count: 0` and `posts: []`.

As with `search`, `--output FILE` writes only JSON to a mode-`0600` file and prints
a save confirmation to stdout. Without it, the JSON document is printed to stdout.

## Bookmarks variability

`bookmarks list` is a formatted pass-through of the upstream X API response, not a
project-owned JSON shape. Requested fields and expansions change its keys. Consult
the current X API documentation for field meanings and constraints.

## Regenerating owned output artifacts

The repository pins Go and validation tooling in `.mise/config.toml`. After editing
a schema, run:

```sh
mise run generate
mise run generate:check
```

`generate` runs the repository generator (`go run ./tools/generate-output`) and
updates the checked-in Go types and Markdown references. `generate:check`, which is
part of `mise run check`, regenerates and fails on tracked differences or untracked
generated artifacts. Add future project-owned formats by adding a schema, extending
the generator, checking in both generated artifacts, and adding schema unit and CLI
integration validation. Generation tools are not needed to build or run the CLI.

The validator is `github.com/santhosh-tekuri/jsonschema/v6` v6.0.2 and compiles the
declared JSON Schema Draft 2020-12 document. The current contract intentionally does
not use `format`: upstream timestamps can be absent and are represented as empty
strings, so timestamp syntax is documented rather than asserted by the validator.
