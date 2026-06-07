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

## Current Limitations

Bookmark retrieval is not implemented yet.
