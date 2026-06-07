# X OAuth2 PKCE Design

## Context

This repository is a pre-release Go CLI named `xapi-usecase`. It currently has no Go module and no existing Go implementation. The first product goal is to authenticate as the current X user so a later command can retrieve that user's X bookmarks through X API v2.

The bookmarks endpoint `GET /2/users/{id}/bookmarks` requires a User Access Token for the authenticated user. This design implements the OAuth 2.0 Authorization Code Flow with PKCE foundation first. Bookmark retrieval itself is out of scope for this implementation pass.

## Goals

- Initialize the Go module as `github.com/y-writings/xapi-usecase` if it is not initialized yet.
- Target Go `1.26.3` in `go.mod` and `.mise/config.toml`.
- Implement X OAuth 2.0 Authorization Code Flow with PKCE for a public/native client.
- Keep dependencies to the standard library for this pass.
- Keep CLI, OAuth, API client, and token storage responsibilities separated.
- Provide unit tests for PKCE, auth URL generation, callback validation, token exchange, refresh, API request authorization, HTTP error handling, and token storage permissions.
- Document the required X Developer Console settings and CLI usage.

## Non-Goals

- Do not implement bookmark retrieval in this pass.
- Do not add OAuth 1.0a support.
- Do not support client secrets, confidential clients, or secret storage.
- Do not add compatibility shims, aliases, or legacy names.
- Do not introduce a CLI framework such as Cobra.
- Do not use `golang.org/x/oauth2` in this pass.

## Architecture

The code will be split into small internal packages:

```text
cmd/xapi-usecase/main.go
internal/cli
internal/xoauth
internal/xapi
internal/tokenstore
```

`cmd/xapi-usecase/main.go` is a thin entry point that passes `os.Args`, standard streams, and process context into `internal/cli`.

`internal/cli` owns command parsing and orchestration. It implements `xapi-usecase auth login`, starts the temporary callback server, prints the authorization URL, exchanges the received code for tokens, and writes the token file.

`internal/xoauth` owns OAuth and PKCE protocol behavior. It generates `code_verifier`, `code_challenge`, and `state`, builds the authorization URL, exchanges authorization codes for tokens, refreshes access tokens, and returns debuggable HTTP errors.

`internal/xapi` owns thin X API v2 HTTP behavior. It resolves API paths against `https://api.x.com`, attaches bearer tokens, turns non-2xx responses into errors with response bodies, and exposes a minimal `Me(ctx)` helper for `/2/users/me`.

`internal/tokenstore` owns JSON token persistence and file permissions.

## OAuth Configuration

The implementation targets X OAuth 2.0 public/native clients only. It accepts `client_id` but never accepts, stores, or transmits a client secret.

OAuth endpoints are constants:

```text
AuthorizeEndpoint = https://x.com/i/oauth2/authorize
TokenEndpoint     = https://api.x.com/2/oauth2/token
```

Default scopes are fixed for this pass:

```text
tweet.read users.read bookmark.read offline.access
```

The CLI does not expose scope customization yet.

## PKCE And State

`code_verifier` and `state` are each generated from 32 bytes of `crypto/rand` entropy and encoded with raw URL-safe base64. The resulting strings are 43 characters, satisfying the PKCE verifier length requirement and providing sufficient state entropy.

The S256 `code_challenge` is computed as:

```text
base64.RawURLEncoding(SHA-256(code_verifier))
```

The authorization URL contains:

```text
response_type=code
client_id=<client_id>
redirect_uri=<callback URL>
scope=tweet.read users.read bookmark.read offline.access
state=<state>
code_challenge=<S256 challenge>
code_challenge_method=S256
```

## Token Exchange And Refresh

Token exchange sends `application/x-www-form-urlencoded` to `https://api.x.com/2/oauth2/token` with:

```text
grant_type=authorization_code
client_id=<client_id>
code=<authorization code>
redirect_uri=<callback URL>
code_verifier=<code verifier>
```

Refresh sends:

```text
grant_type=refresh_token
client_id=<client_id>
refresh_token=<refresh token>
```

If a refresh response contains a new `refresh_token`, the stored refresh token is replaced. If it omits `refresh_token`, the existing refresh token is preserved.

Token endpoint HTTP errors include the response status, request URL path, and up to 4 KiB of response body. They do not include request form values such as `client_id`, `code`, `code_verifier`, or `refresh_token`.

## CLI Flow

The first CLI command is:

```text
xapi-usecase auth login
```

`client_id` is read from `XAPI_USECASE_CLIENT_ID` and can be overridden with `--client-id`.

The temporary callback server binds only to `127.0.0.1`. The default callback URL is:

```text
http://127.0.0.1:8765/callback
```

`--port` changes the port. If the selected port is already in use, the CLI fails instead of automatically choosing another port, because X Developer Console requires exact callback URL registration.

The command prints the authorization URL to stdout. It does not open a browser automatically.

The callback wait timeout defaults to 5 minutes and can be changed with `--timeout`. When a valid callback arrives, token exchange runs immediately so the short-lived authorization code is used promptly.

The callback validates `state` before token exchange. If state mismatches, if X returns `error` or `error_description`, or if `code` is missing, the callback responds with `400 Bad Request`, the CLI exits non-zero, and token exchange is not attempted. The CLI must not print received `code` or `state` values.

Login success means token exchange succeeded and the token was saved. `auth login` does not call `/2/users/me` as part of its success path.

## Token Storage

The default token path is:

```text
os.UserConfigDir()/xapi-usecase/token.json
```

`auth login --token-file` overrides the path.

The parent directory is created with `0700` when needed. The token file is written as JSON with permission `0600`.

Stored JSON fields are:

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "token_type": "bearer",
  "scope": "tweet.read users.read bookmark.read offline.access",
  "expires_at": "2026-06-07T12:34:56Z"
}
```

`expires_at` is derived from `expires_in` and stored as an RFC3339 timestamp. The token file never stores `client_id`, `client_secret`, `code_verifier`, or `state`.

## X API Client

`internal/xapi` uses `https://api.x.com` as the base URL. It attaches:

```text
Authorization: Bearer <access_token>
```

The common request helper handles request creation, path resolution, bearer token injection, and non-2xx error conversion with status and response body. JSON decoding stays with callers by default.

This pass includes a minimal `Me(ctx)` method for `/2/users/me` and a small response type. No CLI command exposes it yet.

## Tests

The implementation will add unit tests for:

- PKCE verifier and S256 challenge generation.
- State generation.
- Authorization URL query parameters.
- Callback success with matching state.
- Callback state mismatch.
- Callback OAuth error.
- Callback missing code.
- Token exchange with a mock HTTP server, including exact form values and content type.
- Refresh with a mock HTTP server, including exact form values, content type, and refresh token preservation when omitted.
- Bearer token API requests.
- HTTP error status and response body inclusion.
- Token JSON saving and file permission `0600`.

Callback tests use `httptest` against handlers or small helpers instead of opening a real OS port.

## Documentation

`README.md` will document:

- How to enable OAuth 2.0 in X Developer Console.
- That the app should be a public/native client for this implementation.
- Exact callback URL registration, defaulting to `http://127.0.0.1:8765/callback`.
- Required scopes: `tweet.read`, `users.read`, `bookmark.read`, `offline.access`.
- How to provide `client_id` through `XAPI_USECASE_CLIENT_ID` or `--client-id`.
- How to run `go run ./cmd/xapi-usecase auth login`.
- That the user must open the printed authorization URL manually.
- Where the token is saved and how `--token-file`, `--port`, and `--timeout` work.

The README will not document bookmark retrieval commands until they exist.

## Verification

After implementation, run:

```sh
go test ./...
```

At design time, `go` is not available on `PATH` in the current environment. If this remains true after implementation, report the exact command failure instead of claiming tests passed.
