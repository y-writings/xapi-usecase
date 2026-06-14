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

The CLI does not use or store a client secret.

## Quick Start

Authenticate, then retrieve one page of bookmarked Posts:

```sh
export XAPI_USECASE_CLIENT_ID="your-client-id"
go run ./cmd/xapi-usecase auth login
go run ./cmd/xapi-usecase bookmarks list
```

## Documentation

- [Authentication](docs/auth.md): X Developer Console setup, `auth login`, login
  options, and token file contents.
- [Bookmarks List](docs/bookmarks.md): `bookmarks list`, pagination, field and
  expansion options, required scopes, and refresh behavior.
