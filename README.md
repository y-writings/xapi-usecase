# xapi-usecase

`xapi-usecase` is a pre-release CLI for X API v2 use cases. The command surface,
configuration, token format, and output are still subject to change before a stable
release.

## Requirements

- Go 1.26.3
- Approved X Developer account and X Developer App

For mise users, `.mise/config.toml` declares Go 1.26.3 for this project.

## X Developer Console Setup

Configure your X Developer App before running the CLI:

1. Enable OAuth 2.0.
2. Configure the app as a public/native client for this CLI flow.
3. Register the callback URL exactly as `http://127.0.0.1:8765/callback`.
4. If you run login with another `--port`, register the matching callback URL, such as `http://127.0.0.1:<port>/callback`.
5. Enable these scopes: `tweet.read`, `users.read`, `bookmark.read`, `offline.access`.
6. Copy the OAuth2 Client ID.

The CLI does not use or store a client secret.

## Login

Set the OAuth2 Client ID, then start the login flow:

```sh
export XAPI_USECASE_CLIENT_ID="your-client-id"
go run ./cmd/xapi-usecase auth login
```

The command prints an authorization URL. Open that URL manually in your browser,
approve the request, and let the browser redirect to the local callback server.

You can pass the client ID directly instead of using the environment variable:

```sh
go run ./cmd/xapi-usecase auth login --client-id "your-client-id"
```

## Login Options

- `--client-id`: OAuth2 Client ID. Overrides `XAPI_USECASE_CLIENT_ID`.
- `--token-file`: Path where the OAuth2 token JSON file is saved.
- `--port`: Local callback port. The registered callback URL must use the same port.
- `--timeout`: Maximum duration to wait for the browser callback.

## Token File

The login command saves a JSON token file with file permission `0600`.

The token file contains:

- `access_token`
- `refresh_token`
- `token_type`
- `scope`
- `expires_at`

The token file does not contain:

- `client_id`
- `client_secret`
- `code_verifier`
- `state`

## Bookmark Retrieval

After `auth login` saves a token, retrieve one page of bookmarked Posts:

```sh
go run ./cmd/xapi-usecase bookmarks list
```

The command resolves the authenticated user with `/2/users/me`, calls
`/2/users/{id}/bookmarks`, and prints the X API JSON response pretty-formatted
on stdout.

By default, the command requests:

```text
tweet.fields=created_at,author_id
```

Use pagination options to request a specific page size or continue from
`meta.next_token`:

```sh
go run ./cmd/xapi-usecase bookmarks list --max-results 10
go run ./cmd/xapi-usecase bookmarks list --pagination-token "next-token"
```

Use field and expansion options to request additional response data:

```sh
go run ./cmd/xapi-usecase bookmarks list \
  --tweet-fields "created_at,author_id,public_metrics" \
  --expansions "author_id" \
  --user-fields "username,name,verified"
```

## Bookmark Options

- `--token-file`: Path where the OAuth2 token JSON file is read and updated after refresh.
- `--client-id`: OAuth2 Client ID. Only required when the saved access token must be refreshed. Overrides `XAPI_USECASE_CLIENT_ID`.
- `--max-results`: Results per page, from `1` through `100`.
- `--pagination-token`: Page token from `meta.next_token`.
- `--tweet-fields`: Comma-separated `tweet.fields`. Defaults to `created_at,author_id`.
- `--expansions`: Comma-separated expansions.
- `--user-fields`: Comma-separated `user.fields`.
- `--media-fields`: Comma-separated `media.fields`.
- `--poll-fields`: Comma-separated `poll.fields`.
- `--place-fields`: Comma-separated `place.fields`.
- `--timeout`: Command timeout. Defaults to `30s`.

The required X OAuth scopes are `bookmark.read`, `tweet.read`, and `users.read`.
`offline.access` is required for automatic refresh.
