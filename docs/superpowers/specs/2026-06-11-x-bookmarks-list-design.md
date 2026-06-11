# X Bookmarks List Design

## Context

`xapi-usecase` is a pre-release Go CLI for X API v2 use cases. It already implements OAuth 2.0 Authorization Code Flow with PKCE through `xapi-usecase auth login`, stores tokens in a JSON file, and has a small authenticated X API client with `/2/users/me` support.

The next product goal is to retrieve Posts bookmarked by the authenticated user. The relevant X API endpoint is:

```text
GET /2/users/{id}/bookmarks
```

The `{id}` must match the authenticated user. The endpoint requires OAuth 2.0 user context with `bookmark.read`, `tweet.read`, and `users.read` scopes.

## Goals

- Add `xapi-usecase bookmarks list`.
- Read the token saved by `auth login`.
- Refresh the token automatically when it is expired, within five minutes of expiry, or after a `401 Unauthorized` response.
- Resolve the authenticated user ID through `/2/users/me` before retrieving bookmarks.
- Retrieve one page of bookmarks from `/2/users/{id}/bookmarks`.
- Print the X API response as pretty JSON on stdout.
- Keep stdout reserved for bookmark JSON only.
- Keep CLI, token storage, OAuth, and X API client responsibilities separated.
- Cover the new behavior with unit tests.
- Update README usage documentation.

## Non-Goals

- Do not auto-fetch all pages.
- Do not add `--all`.
- Do not implement bookmark folders or folder bookmark lookup.
- Do not implement bookmark creation or deletion.
- Do not add text, table, or JSONL output formats.
- Do not store `client_id` in the token file.
- Do not pre-validate saved token scopes.
- Do not model the full bookmark response as Go structs.

## Architecture

The implementation keeps the existing package boundaries:

```text
cmd/xapi-usecase/main.go
internal/cli
internal/tokenstore
internal/xoauth
internal/xapi
```

`cmd/xapi-usecase/main.go` remains a thin entry point.

`internal/cli` owns command parsing and orchestration. It adds `bookmarks list`, reads the token file, decides whether refresh is needed, calls `/2/users/me`, calls the bookmarks endpoint, and writes pretty JSON to stdout.

`internal/tokenstore` owns token persistence. It adds `Load(path) (xoauth.Token, error)` to read the JSON written by the existing `Save` function.

`internal/xoauth` already owns token refresh. `bookmarks list` reuses the existing `Client.Refresh` method and does not add new OAuth behavior.

`internal/xapi` remains a thin bearer-token X API v2 HTTP client. It adds `BookmarksRaw(ctx context.Context, userID string, options BookmarkOptions) ([]byte, error)` without learning about token files, refresh tokens, or `client_id`.

Token refresh and token file updates stay in `internal/cli`, not `internal/xapi`, because refresh depends on CLI inputs, token file persistence, and command-level retry policy.

## CLI Contract

The new command is:

```text
xapi-usecase bookmarks list
```

Supported flags:

- `--token-file PATH`: token JSON file to read and update after refresh. Defaults to `tokenstore.DefaultPath()`.
- `--client-id CLIENT_ID`: OAuth2 client ID used only when refresh is needed. Defaults to `XAPI_USECASE_CLIENT_ID`.
- `--max-results N`: optional page size. Must be between `1` and `100` when provided.
- `--pagination-token TOKEN`: optional next-page token from `meta.next_token`.
- `--tweet-fields FIELDS`: optional comma-separated `tweet.fields`. Defaults to `created_at,author_id`.
- `--expansions EXPANSIONS`: optional comma-separated expansions.
- `--user-fields FIELDS`: optional comma-separated `user.fields`.
- `--media-fields FIELDS`: optional comma-separated `media.fields`.
- `--poll-fields FIELDS`: optional comma-separated `poll.fields`.
- `--place-fields FIELDS`: optional comma-separated `place.fields`.
- `--timeout DURATION`: command-level timeout. Defaults to `30s`.

`bookmarks list` prints only pretty JSON to stdout. It does not print refresh success messages. Errors are printed to stderr through the existing `Run` error path.

Exit behavior:

- Help exits `0`.
- Command-line and flag errors exit `2`.
- Runtime errors such as token read failures, refresh failures, X API failures, and invalid JSON responses exit `1`.
- HTTP 2xx bookmark responses exit `0`, even when the response JSON includes an `errors` field.

## Data Flow

`bookmarks list` runs this sequence:

1. Parse flags.
2. Validate `--max-results` when it is provided.
3. Resolve the token file path.
4. Load the token through `tokenstore.Load`.
5. Require `access_token` and a valid `expires_at`.
6. If `expires_at` is at or before `now + 5m`, refresh the token.
7. When refresh happens, save the refreshed token back to the same token file.
8. Create an `xapi.Client` with the current access token.
9. Call `/2/users/me` to resolve the authenticated user ID.
10. Call `/2/users/{id}/bookmarks` with requested query parameters.
11. Pretty print the raw JSON response to stdout.

Both `/2/users/me` and `/2/users/{id}/bookmarks` use the same `--timeout` derived context. Refresh also uses this command-level context.

## Refresh And Retry

Refresh is attempted before API calls when the saved token expires within five minutes. This avoids starting a command with a token likely to expire during execution.

Refresh requires:

- `refresh_token` from the token file.
- `client_id` from `--client-id` or `XAPI_USECASE_CLIENT_ID`.

`client_id` is required only when refresh is needed. A valid unexpired access token can be used without providing `client_id`.

If `/2/users/me` or the bookmarks endpoint returns `401 Unauthorized`, the command refreshes the token if possible, saves it, and retries only the failed request one time. If the retry also fails, the X API error is returned. `403 Forbidden` is not retried because it is more likely to indicate missing scope or insufficient access.

Refresh success is silent. Refresh failure is reported as a runtime error.

## X API Request Shape

`internal/xapi` adds a small options type for bookmark query parameters. The bookmarks helper builds this path:

```text
/2/users/{id}/bookmarks
```

It appends only the query parameters selected by CLI flags:

- `max_results`
- `pagination_token`
- `tweet.fields`
- `expansions`
- `user.fields`
- `media.fields`
- `poll.fields`
- `place.fields`

`tweet.fields` defaults to `created_at,author_id`. The other field and expansion parameters are omitted when not specified.

The CLI validates only `--max-results` because the X API publishes a simple numeric boundary. It does not validate field or expansion names; the X API remains the source of truth for allowed values.

The bookmarks helper returns raw JSON bytes. It does not decode into `Tweet`, `Meta`, or `Includes` structs. This preserves `data`, `includes`, `meta.next_token`, partial-success `errors`, and future response fields.

## Token Handling

`tokenstore.Load` reads the same JSON shape written by `tokenstore.Save`:

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "token_type": "bearer",
  "scope": "tweet.read users.read bookmark.read offline.access",
  "expires_at": "2026-06-11T12:34:56Z"
}
```

Load behavior:

- Unknown JSON fields are ignored.
- `access_token` is required.
- `expires_at` is required and must parse as RFC3339.
- `refresh_token` is required only when refresh is needed.
- `token_type` and `scope` are loaded but not used for preflight rejection.

If a refresh response omits `refresh_token`, the existing `xoauth.Client.Refresh` behavior preserves the previous refresh token.

## Usage And Help

The current single usage function is split into command-specific help:

- `printUsage`: top-level overview listing `auth login` and `bookmarks list`.
- `printAuthLoginUsage`: existing login usage and environment details.
- `printBookmarksListUsage`: bookmark flags, required scopes, token file behavior, and refresh `client_id` behavior.

Expected help behavior:

- `xapi-usecase --help` exits `0`.
- `xapi-usecase auth login --help` exits `0`.
- `xapi-usecase bookmarks list --help` exits `0`.
- Unknown commands and invalid flags exit `2`.

## Error Handling

X API non-2xx responses keep using `xapi.HTTPError`, which includes status, path, and up to 4 KiB of response body. This preserves useful API details without adding response-specific error parsing.

The pretty-print step decodes the raw JSON response only to reformat it. If decoding or indentation fails, the command exits with a runtime error because the API returned invalid JSON for a successful bookmark request.

The command does not redact response bodies from X API failures. Existing `xapi.HTTPError` behavior already includes response bodies for debuggability. Bookmark requests send the access token only in the `Authorization` header, not in paths, query strings, or request bodies.

## Tests

Add unit tests for token loading:

- `Load` reads a saved token JSON into `xoauth.Token`.
- `Load` ignores unknown fields.
- `Load` rejects missing or empty `access_token`.
- `Load` rejects missing, empty, or invalid `expires_at`.

Add unit tests for bookmark API requests:

- `BookmarksRaw` calls `/2/users/{id}/bookmarks`.
- `BookmarksRaw` attaches expected query parameters.
- `BookmarksRaw` returns raw JSON bytes.
- Existing `Do` HTTP error behavior covers non-2xx conversion.

Add CLI tests for `bookmarks list` orchestration:

- Valid unexpired token calls `/2/users/me`, then bookmarks, and prints pretty JSON.
- `--max-results` accepts `1` and `100`.
- `--max-results` rejects values outside `1..100` with exit `2`.
- Token expiring within five minutes is refreshed, saved, and then used.
- Refresh requires `client_id` only when refresh is needed.
- `/2/users/me` returning `401` triggers one refresh and retry.
- Bookmarks returning `401` triggers one refresh and retry.
- HTTP 2xx response containing JSON `errors` exits `0`.
- Invalid successful JSON exits `1`.

Verification command:

```sh
go test ./...
```

## Documentation

Update `README.md` to document:

- `xapi-usecase bookmarks list`.
- That `auth login` must be run first.
- Required scopes: `bookmark.read`, `tweet.read`, `users.read`, and `offline.access` for refresh.
- The default one-page behavior.
- `--max-results` and `--pagination-token`.
- `--tweet-fields`, `--expansions`, and related field flags.
- That stdout is pretty JSON from the X API response.
- That `--client-id` or `XAPI_USECASE_CLIENT_ID` is needed only when refresh is required.

## Verification

After implementation, run:

```sh
go test ./...
```
