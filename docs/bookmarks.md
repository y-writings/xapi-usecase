# Bookmarks List

Run [`auth login`](auth.md) first so the CLI has a saved OAuth2 token.

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

- `--token-file`: Path where the OAuth2 token JSON file is read and updated after
  refresh.
- `--client-id`: OAuth2 Client ID. Only required when the saved access token must be
  refreshed. Overrides `XAPI_USECASE_CLIENT_ID`.
- `--max-results`: Results per page, from `1` through `100`.
- `--pagination-token`: Page token from `meta.next_token`.
- `--tweet-fields`: Comma-separated `tweet.fields`. Defaults to
  `created_at,author_id`.
- `--expansions`: Comma-separated expansions.
- `--user-fields`: Comma-separated `user.fields`.
- `--media-fields`: Comma-separated `media.fields`.
- `--poll-fields`: Comma-separated `poll.fields`.
- `--place-fields`: Comma-separated `place.fields`.
- `--timeout`: Command timeout. Defaults to `30s`.

The required X OAuth scopes are `bookmark.read`, `tweet.read`, and `users.read`.
`offline.access` is required for automatic refresh.
