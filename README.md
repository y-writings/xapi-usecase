# xapi-usecase

`xapi-usecase` is a pre-release CLI for X API v2 use cases. The command surface,
configuration, token format, and output are still subject to change before a stable
release.

## Requirements

- Go 1.26.3
- golangci-lint 2.12.2 for development checks
- Approved X Developer account and X Developer App

## X API Prerequisites

Before running the CLI, configure an approved X Developer App:

- Enable OAuth 2.0 for a public/native client.
- Register `http://127.0.0.1:8765/callback`, or the callback URL matching your
  custom `--port`.
- Enable these scopes: `tweet.read`, `users.read`, `bookmark.read`, `offline.access`.
- Copy the OAuth2 Client ID.

`bookmarks list`, `search`, and `user-posts` use the OAuth token saved by
`auth login`.

The CLI does not use or store a client secret.

## Quick Start

Authenticate, then retrieve one page of bookmarked Posts:

```sh
export XAPI_USECASE_CLIENT_ID="your-client-id"
go run ./cmd/xapi-usecase auth login
go run ./cmd/xapi-usecase bookmarks list
```

## Collect recent search posts

Collect recent posts as post-oriented JSON for later analysis, including posts with
no links. For example, research recent Grok Bot use cases in Japanese or OpenAI
Dots feedback in English:

```sh
go run ./cmd/xapi-usecase search \
  --query '@grok (使い方 OR 不具合 OR 評価)' --lang ja --limit 200 \
  --start-time 2026-10-10T00:00:00Z --end-time 2026-10-17T00:00:00Z \
  --output grok-posts.json

go run ./cmd/xapi-usecase search \
  --query '"OpenAI dots" (use case OR limitation OR feedback)' --lang en \
  --limit 500 --output dots-posts.json
```

Every item contains the post ID and URL, text, creation time, author ID/name/username,
and an `external_urls` array pairing the original shortened URL with the best expanded
destination returned by X. A post without a link has `external_urls: []`. Full
`note_post` text and its URLs are used when available. Posts are deduplicated by ID
across pages, and `post_count` always equals the number of elements in `posts`.

`--query` accepts any X search expression; `--lang` appends an X language operator.
`--start-time` is inclusive and `--end-time` is exclusive, and both are optional
RFC 3339 timestamps. The command pages until X returns no next page or the CLI's
`--limit` (1–1,000) stops it while more results are available. These are separate
constraints: Recent Search only covers approximately the preceding seven days;
the requested boundaries further narrow that window; `--limit` is a local output
cap; and the account's access level and rate limits can restrict what X returns.

`status: "complete"` only means every page made available by the upstream request
was consumed. It does not claim all posts on X were returned. `limit_reached` means
the local cap stopped collection while another result or page was available.
`incomplete` distinguishes authentication/access denial, rate limiting, API errors,
and request failures, while preserving posts from successful pages and usable data
in a partial response. Zero results are valid and produce `posts: []`.

Search uses the saved OAuth token by default. Use `--token-file` to select another
file. A token expiring within five minutes is refreshed before collection; a `401`
response triggers one refresh and retry of the failed page. Refresh requires
`--client-id` or `XAPI_USECASE_CLIENT_ID`. Alternatively, `--bearer-token` (or
`XAPI_USECASE_BEARER_TOKEN`) selects an explicit bearer token, takes precedence over
saved OAuth, and is not refreshed. Required scopes are `tweet.read` and `users.read`,
plus `offline.access` for refresh.

The command uses `GET /2/tweets/search/recent`; it does not use Full-Archive Search,
fetch linked-page contents, classify, summarize, or rank posts. See the generated
[JSON field reference](docs/reference/search-output.md), the [command output guide](docs/output.md),
and X's [Recent Search documentation](https://docs.x.com/x-api/posts/search-recent-posts).

## Retrieve a user's posts

Resolve an X username and retrieve posts in an RFC 3339 time interval with the
User Posts API:

```sh
go run ./cmd/xapi-usecase user-posts \
  --username XDevelopers \
  --start-time 2026-09-01T00:00:00Z \
  --end-time 2026-10-01T00:00:00Z \
  --limit 500 --output posts.json
```

The start boundary is inclusive and the end boundary is exclusive, matching the
upstream API. The CLI rejects malformed timestamps and ranges where the start is
not earlier than the end. It resolves `--username` with
`GET /2/users/by/username/:username`, then pages
`GET /2/users/:id/tweets` until X has no next page or `--limit` is reached.
`status: "limit_reached"` means the CLI stopped while another result was
available, so the file must not be read as every post in the interval.

Posts are included whether or not they contain an external link. Each item records
its ID, canonical URL, text, and creation time; when X supplies long-form
`note_post` text, that full text is used. Replies and reposts are included by
default. Pass `--exclude-replies` and/or `--exclude-reposts` to ask X to omit them.

User Posts is not Recent Search and the CLI therefore imposes no seven-day
lookback rule. X limits the timeline to the user's most recent 3,200 posts; date
bounds do not make older posts available. The CLI's `--limit` (1–3,200) is a
separate local collection cap. API access level, protected-post visibility, and
rate limits can further restrict results. Authentication/access denial, rate
limiting, other API errors, and request failures are distinguished in incomplete
JSON output. Data from successful earlier pages and usable data accompanying an
upstream partial error are retained.

Authentication and refresh precedence are the same as for `search`: saved OAuth
is the default, while `--bearer-token` or `XAPI_USECASE_BEARER_TOKEN` selects an
explicit bearer token. The API calls require `tweet.read` and `users.read`; reading
protected posts additionally depends on the authentication mode and caller's
permission.

See the generated [user-posts JSON field reference](docs/reference/user-posts-output.md)
for the owned output contract and [command output guide](docs/output.md) for stream
and exit-status behavior.

## Development

Go dependencies are managed with `go.mod` and `go.sum`. Dependency source is not
committed to the repository. Run `mise run check` for formatting, generated-file,
lint, and test checks.

Nix fetches dependencies separately and verifies them using `vendorHash` in
`flake.nix`. After changing dependencies, set it to `pkgs.lib.fakeHash`, run
`nix build .# --no-link`, replace it with the reported `got` hash, and build again.

## Documentation

- [Authentication](docs/auth.md): X Developer Console setup, `auth login`, login
  options, and token file contents.
- [Bookmarks List](docs/bookmarks.md): `bookmarks list`, pagination, field and
  expansion options, required scopes, and refresh behavior.
- [Command output contracts](docs/output.md): output destinations, formats, schema
  ownership, exit statuses, and generation workflow.
