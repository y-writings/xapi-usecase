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

The `search` command uses an app bearer token instead of the saved OAuth token. Set
`XAPI_USECASE_BEARER_TOKEN` or pass `--bearer-token`.

The CLI does not use or store a client secret.

## Quick Start

Authenticate, then retrieve one page of bookmarked Posts:

```sh
export XAPI_USECASE_CLIENT_ID="your-client-id"
go run ./cmd/xapi-usecase auth login
go run ./cmd/xapi-usecase bookmarks list
```

## Search external resources

Search recent X posts, extract their expanded external URLs (including long-form
post URLs), deduplicate resources and source posts, and retain provenance:

```sh
export XAPI_USECASE_BEARER_TOKEN="your-bearer-token"
go run ./cmd/xapi-usecase search \
  --query '業務ドメイン 知識共有' --lang ja --limit 200 \
  --start-time 2026-10-01T00:00:00Z --output results.json
```

`--query` accepts any X search expression. `--lang` adds an X language operator;
`--start-time` and `--end-time` are RFC 3339 timestamps. The CLI pages until
`--limit` posts (maximum 1,000) have been examined or the API has no next page.
Each resource lists a source post at most once, even if that post repeats the URL.
Only HTTP(S) links outside `x.com` and `twitter.com` are collected. Expanded or
unwound destinations supplied by X are preferred, while the original shortened URL
is retained on each source.

Before grouping, URLs are normalized by lowercasing the host, removing its terminal
DNS dot and the URL fragment, normalizing numeric ports (omitting HTTP port 80 and
HTTPS port 443), and using `/` for empty paths. Non-default ports, non-empty paths,
and query strings remain part of resource identity.

The command always emits a JSON document when an API request fails after collection
starts. `status` is then `incomplete`, and `incomplete_reason` distinguishes
`authentication_or_access_denied`, `rate_limited`, `api_error`, and
`request_failed`. An HTTP 200 response containing both `data` and `errors` is
also `incomplete` with reason `api_error`; usable data from that response is
included, and pagination stops. It exits non-zero so partial output cannot be
mistaken for a successful run. A successful zero-result search has
`status: "complete"`, zero counts, and an empty `resources` array.

Example (abbreviated):

```json
{
  "status": "complete",
  "search": {"query": "業務ドメイン 知識共有", "language": "ja", "limit": 200},
  "retrieved_at": "2026-10-06T12:00:00Z",
  "post_count": 2,
  "resource_count": 1,
  "resources": [{
    "url": "https://speakerdeck.com/example/domain-modeling",
    "sources": [{
      "post_id": "123",
      "post_url": "https://x.com/i/status/123",
      "text": "参考資料です",
      "created_at": "2026-10-05T09:00:00Z",
      "short_url": "https://t.co/example"
    }]
  }]
}
```

This uses X API v2 recent search (`GET /2/tweets/search/recent`). Availability,
lookback window, monthly post cap, rate limits, and supported search operators depend
on the X Developer plan attached to the app. The CLI cannot request posts outside
that plan's recent-search window; X returns an API error for unsupported dates or
operators. It does not crawl linked pages or summarize their contents. Consult the
[X recent search documentation](https://docs.x.com/x-api/posts/recent-search) for
the current plan-specific restrictions.

## Documentation

- [Authentication](docs/auth.md): X Developer Console setup, `auth login`, login
  options, and token file contents.
- [Bookmarks List](docs/bookmarks.md): `bookmarks list`, pagination, field and
  expansion options, required scopes, and refresh behavior.
