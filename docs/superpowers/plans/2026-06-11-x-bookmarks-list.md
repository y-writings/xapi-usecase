# X Bookmarks List Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `xapi-usecase bookmarks list` to read the saved OAuth token, refresh it when needed, fetch the authenticated user's bookmarked Posts, and print the X API response as pretty JSON.

**Architecture:** Keep orchestration in `internal/cli`, token persistence in `internal/tokenstore`, OAuth refresh in `internal/xoauth`, and bearer-token HTTP behavior in `internal/xapi`. The CLI loads and refreshes tokens, resolves `/2/users/me`, calls `/2/users/{id}/bookmarks`, and pretty-prints raw JSON without modeling the full X API response.

**Tech Stack:** Go 1.26.3, standard library only, `testing`, `httptest`, existing internal packages.

---

Repository instruction: do not create commits unless the user explicitly asks. This plan uses verification checkpoints instead of commit steps.

## File Structure

- Modify `internal/tokenstore/tokenstore.go`: add `Load(path) (xoauth.Token, error)`.
- Modify `internal/tokenstore/tokenstore_test.go`: add token loading tests.
- Modify `internal/xapi/client.go`: add `BookmarkOptions` and `BookmarksRaw`.
- Modify `internal/xapi/client_test.go`: add bookmark request tests.
- Modify `internal/cli/cli.go`: route `bookmarks list`, improve top-level and command-specific usage.
- Create `internal/cli/bookmarks.go`: parse bookmark flags, load/refresh token, call X API, retry once on `401`, pretty-print JSON.
- Create `internal/cli/bookmarks_test.go`: test bookmark CLI behavior with `httptest` and package-level test seams.
- Modify `README.md`: document bookmark retrieval usage and flags.

## Task 1: Tokenstore Load

**Files:**
- Modify: `internal/tokenstore/tokenstore_test.go`
- Modify: `internal/tokenstore/tokenstore.go`

- [ ] **Step 1: Write failing tests for token loading**

Append this code to `internal/tokenstore/tokenstore_test.go` before `assertSavedValue`:

```go
func TestLoadReadsSavedToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	writeTokenJSON(t, path, `{"access_token":"access-123","refresh_token":"refresh-123","token_type":"bearer","scope":"tweet.read users.read bookmark.read offline.access","expires_at":"2026-06-07T12:34:56Z"}`)

	token, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if token.AccessToken != "access-123" {
		t.Fatalf("AccessToken = %q, want access-123", token.AccessToken)
	}
	if token.RefreshToken != "refresh-123" {
		t.Fatalf("RefreshToken = %q, want refresh-123", token.RefreshToken)
	}
	if token.TokenType != "bearer" {
		t.Fatalf("TokenType = %q, want bearer", token.TokenType)
	}
	if token.Scope != "tweet.read users.read bookmark.read offline.access" {
		t.Fatalf("Scope = %q, want default scope", token.Scope)
	}
	wantExpiresAt := time.Date(2026, 6, 7, 12, 34, 56, 0, time.UTC)
	if !token.ExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("ExpiresAt = %s, want %s", token.ExpiresAt, wantExpiresAt)
	}
}

func TestLoadIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	writeTokenJSON(t, path, `{"access_token":"access-123","refresh_token":"refresh-123","token_type":"bearer","scope":"tweet.read","expires_at":"2026-06-07T12:34:56Z","unknown":"ignored"}`)

	if _, err := Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsMissingOrEmptyAccessToken(t *testing.T) {
	for _, test := range []struct {
		name string
		json string
	}{
		{name: "missing", json: `{"expires_at":"2026-06-07T12:34:56Z"}`},
		{name: "empty", json: `{"access_token":"","expires_at":"2026-06-07T12:34:56Z"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token.json")
			writeTokenJSON(t, path, test.json)

			if _, err := Load(path); err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestLoadRejectsMissingEmptyOrInvalidExpiresAt(t *testing.T) {
	for _, test := range []struct {
		name string
		json string
	}{
		{name: "missing", json: `{"access_token":"access-123"}`},
		{name: "empty", json: `{"access_token":"access-123","expires_at":""}`},
		{name: "invalid", json: `{"access_token":"access-123","expires_at":"not-a-time"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token.json")
			writeTokenJSON(t, path, test.json)

			if _, err := Load(path); err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func writeTokenJSON(t *testing.T, path string, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
```

- [ ] **Step 2: Run the tokenstore tests and verify failure**

Run: `go test ./internal/tokenstore -run TestLoad -count=1`

Expected: FAIL with `undefined: Load`.

- [ ] **Step 3: Implement `tokenstore.Load`**

In `internal/tokenstore/tokenstore.go`, add `fmt` to the import list and add this function after `DefaultPath`:

```go
func Load(path string) (xoauth.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return xoauth.Token{}, err
	}

	var saved savedToken
	if err := json.Unmarshal(data, &saved); err != nil {
		return xoauth.Token{}, err
	}
	if saved.AccessToken == "" {
		return xoauth.Token{}, fmt.Errorf("access token is required in token file")
	}
	if saved.ExpiresAt == "" {
		return xoauth.Token{}, fmt.Errorf("expires_at is required in token file")
	}

	expiresAt, err := time.Parse(time.RFC3339, saved.ExpiresAt)
	if err != nil {
		return xoauth.Token{}, fmt.Errorf("parse expires_at: %w", err)
	}

	return xoauth.Token{
		AccessToken:  saved.AccessToken,
		RefreshToken: saved.RefreshToken,
		TokenType:    saved.TokenType,
		Scope:        saved.Scope,
		ExpiresAt:    expiresAt,
	}, nil
}
```

- [ ] **Step 4: Run the tokenstore tests and verify pass**

Run: `go test ./internal/tokenstore -count=1`

Expected: PASS.

- [ ] **Step 5: Checkpoint**

Run: `git diff -- internal/tokenstore/tokenstore.go internal/tokenstore/tokenstore_test.go`

Expected: diff contains only `Load` and its tests.

## Task 2: Raw Bookmarks X API Client

**Files:**
- Modify: `internal/xapi/client_test.go`
- Modify: `internal/xapi/client.go`

- [ ] **Step 1: Write failing tests for bookmark requests**

Append this code to `internal/xapi/client_test.go` before `roundTripFunc`:

```go
func TestBookmarksRawSendsExpectedQueryAndReturnsRawJSON(t *testing.T) {
	const rawResponse = `{"data":[{"id":"1","text":"hello"}],"meta":{"result_count":1}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/2/users/2244994945/bookmarks" {
			t.Fatalf("path = %q, want /2/users/2244994945/bookmarks", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-123" {
			t.Fatalf("Authorization = %q, want Bearer access-123", got)
		}

		query := r.URL.Query()
		assertQueryValue(t, query, "max_results", "10")
		assertQueryValue(t, query, "pagination_token", "next-token")
		assertQueryValue(t, query, "tweet.fields", "created_at,author_id")
		assertQueryValue(t, query, "expansions", "author_id")
		assertQueryValue(t, query, "user.fields", "username,name")
		assertQueryValue(t, query, "media.fields", "url")
		assertQueryValue(t, query, "poll.fields", "options")
		assertQueryValue(t, query, "place.fields", "full_name")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, rawResponse)
	}))
	defer server.Close()

	client := &Client{AccessToken: "access-123", BaseURL: server.URL, HTTPClient: server.Client()}

	got, err := client.BookmarksRaw(context.Background(), "2244994945", BookmarkOptions{
		MaxResults:      10,
		PaginationToken: "next-token",
		TweetFields:     "created_at,author_id",
		Expansions:      "author_id",
		UserFields:      "username,name",
		MediaFields:     "url",
		PollFields:      "options",
		PlaceFields:     "full_name",
	})
	if err != nil {
		t.Fatalf("BookmarksRaw() error = %v", err)
	}
	if string(got) != rawResponse {
		t.Fatalf("BookmarksRaw() = %q, want %q", string(got), rawResponse)
	}
}

func TestBookmarksRawOmitsEmptyOptionalQueryParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.RawQuery; got != "" {
			t.Fatalf("RawQuery = %q, want empty", got)
		}
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer server.Close()

	client := &Client{AccessToken: "access-123", BaseURL: server.URL, HTTPClient: server.Client()}

	if _, err := client.BookmarksRaw(context.Background(), "2244994945", BookmarkOptions{}); err != nil {
		t.Fatalf("BookmarksRaw() error = %v", err)
	}
}

func assertQueryValue(t *testing.T, query map[string][]string, key string, want string) {
	t.Helper()

	values := query[key]
	if len(values) != 1 || values[0] != want {
		t.Fatalf("query[%s] = %v, want [%s]", key, values, want)
	}
}
```

- [ ] **Step 2: Run xapi tests and verify failure**

Run: `go test ./internal/xapi -run TestBookmarksRaw -count=1`

Expected: FAIL with `undefined: BookmarkOptions` or `client.BookmarksRaw undefined`.

- [ ] **Step 3: Implement raw bookmark retrieval**

In `internal/xapi/client.go`, add `errors` to the imports. Add this type and method after `User`:

```go
type BookmarkOptions struct {
	MaxResults      int
	PaginationToken string
	TweetFields     string
	Expansions      string
	UserFields      string
	MediaFields     string
	PollFields      string
	PlaceFields     string
}

func (c *Client) BookmarksRaw(ctx context.Context, userID string, options BookmarkOptions) ([]byte, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	query := url.Values{}
	if options.MaxResults > 0 {
		query.Set("max_results", fmt.Sprintf("%d", options.MaxResults))
	}
	if options.PaginationToken != "" {
		query.Set("pagination_token", options.PaginationToken)
	}
	if options.TweetFields != "" {
		query.Set("tweet.fields", options.TweetFields)
	}
	if options.Expansions != "" {
		query.Set("expansions", options.Expansions)
	}
	if options.UserFields != "" {
		query.Set("user.fields", options.UserFields)
	}
	if options.MediaFields != "" {
		query.Set("media.fields", options.MediaFields)
	}
	if options.PollFields != "" {
		query.Set("poll.fields", options.PollFields)
	}
	if options.PlaceFields != "" {
		query.Set("place.fields", options.PlaceFields)
	}

	path := "/2/users/" + url.PathEscape(userID) + "/bookmarks"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	response, err := c.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	return io.ReadAll(response.Body)
}
```

- [ ] **Step 4: Run xapi tests and verify pass**

Run: `go test ./internal/xapi -count=1`

Expected: PASS.

- [ ] **Step 5: Checkpoint**

Run: `git diff -- internal/xapi/client.go internal/xapi/client_test.go`

Expected: diff contains `BookmarkOptions`, `BookmarksRaw`, and tests.

## Task 3: CLI Dispatch And Usage

**Files:**
- Modify: `internal/cli/cli.go`
- Create: `internal/cli/bookmarks.go`
- Create: `internal/cli/bookmarks_test.go`

- [ ] **Step 1: Write failing tests for help routing**

Create `internal/cli/bookmarks_test.go` with this initial content:

```go
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunPrintsTopLevelHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), []string{"--help"}, &stdout, &stderr, getenvNone)

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, want := range []string{"xapi-usecase auth login", "xapi-usecase bookmarks list"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunPrintsAuthLoginHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), []string{"auth", "login", "--help"}, &stdout, &stderr, getenvNone)

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "--port") {
		t.Fatalf("stdout = %q, want auth login options", stdout.String())
	}
}

func TestRunPrintsBookmarksListHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), []string{"bookmarks", "list", "--help"}, &stdout, &stderr, getenvNone)

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, want := range []string{"--max-results", "--pagination-token", "bookmark.read"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func getenvNone(string) string {
	return ""
}
```

- [ ] **Step 2: Run CLI help tests and verify failure**

Run: `go test ./internal/cli -run 'TestRunPrints.*Help' -count=1`

Expected: FAIL because `bookmarks list` is not routed and auth help prints top-level usage.

- [ ] **Step 3: Add bookmark command stub and usage routing**

Create `internal/cli/bookmarks.go` with this stub:

```go
package cli

import (
	"context"
	"errors"
	"flag"
	"io"
)

func bookmarksList(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) error {
	flags := flag.NewFlagSet("xapi-usecase bookmarks list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printBookmarksListUsage(stdout)
			return errHelpRequested
		}
		printBookmarksListUsage(stderr)
		return commandLineError(err.Error())
	}

	return errors.New("bookmarks list is not implemented")
}
```

Modify `Run` in `internal/cli/cli.go` to dispatch by command:

```go
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) int {
	if len(args) == 1 && args[0] == "--help" {
		printUsage(stdout)
		return 0
	}
	if len(args) < 2 {
		printUsage(stderr)
		return 2
	}

	var err error
	switch {
	case args[0] == "auth" && args[1] == "login":
		err = authLogin(ctx, args[2:], stdout, stderr, getenv)
	case args[0] == "bookmarks" && args[1] == "list":
		err = bookmarksList(ctx, args[2:], stdout, stderr, getenv)
	default:
		printUsage(stderr)
		return 2
	}

	if err != nil {
		if errors.Is(err, errHelpRequested) {
			return 0
		}
		fmt.Fprintf(stderr, "Error: %v\n", err)
		var commandLine commandLineError
		if errors.As(err, &commandLine) {
			return 2
		}
		return 1
	}

	return 0
}
```

Change the `flag.ErrHelp` and parse-error paths in `authLogin` from `printUsage(...)` to `printAuthLoginUsage(...)`:

```go
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printAuthLoginUsage(stdout)
			return errHelpRequested
		}
		printAuthLoginUsage(stderr)
		return commandLineError(err.Error())
	}
```

Replace `printUsage` in `internal/cli/cli.go` with these three functions:

```go
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  xapi-usecase auth login [--client-id CLIENT_ID] [--token-file PATH] [--port PORT] [--timeout DURATION]")
	fmt.Fprintln(w, "  xapi-usecase bookmarks list [--token-file PATH] [--max-results N] [--pagination-token TOKEN]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run a command with --help for command-specific options.")
}

func printAuthLoginUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  xapi-usecase auth login [--client-id CLIENT_ID] [--token-file PATH] [--port PORT] [--timeout DURATION]")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Environment:\n  %s  OAuth2 client ID used when --client-id is omitted\n", clientIDEnv)
}

func printBookmarksListUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  xapi-usecase bookmarks list [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --token-file PATH           OAuth2 token JSON file")
	fmt.Fprintln(w, "  --client-id CLIENT_ID       OAuth2 client ID used only when refresh is needed")
	fmt.Fprintln(w, "  --max-results N             results per page, 1 through 100")
	fmt.Fprintln(w, "  --pagination-token TOKEN    page token from meta.next_token")
	fmt.Fprintln(w, "  --tweet-fields FIELDS       comma-separated tweet.fields, defaults to created_at,author_id")
	fmt.Fprintln(w, "  --expansions EXPANSIONS     comma-separated expansions")
	fmt.Fprintln(w, "  --user-fields FIELDS        comma-separated user.fields")
	fmt.Fprintln(w, "  --media-fields FIELDS       comma-separated media.fields")
	fmt.Fprintln(w, "  --poll-fields FIELDS        comma-separated poll.fields")
	fmt.Fprintln(w, "  --place-fields FIELDS       comma-separated place.fields")
	fmt.Fprintln(w, "  --timeout DURATION          command timeout, defaults to 30s")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Environment:\n  %s  OAuth2 client ID used for refresh when --client-id is omitted\n", clientIDEnv)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Required scopes: bookmark.read, tweet.read, users.read. offline.access is required for refresh.")
}
```

- [ ] **Step 4: Run CLI help tests and verify pass**

Run: `go test ./internal/cli -run 'TestRunPrints.*Help' -count=1`

Expected: PASS.

- [ ] **Step 5: Checkpoint**

Run: `git diff -- internal/cli/cli.go internal/cli/bookmarks.go internal/cli/bookmarks_test.go`

Expected: diff contains command routing, usage functions, and help tests.

## Task 4: Bookmarks Happy Path And Pretty JSON

**Files:**
- Modify: `internal/cli/bookmarks.go`
- Modify: `internal/cli/bookmarks_test.go`

- [ ] **Step 1: Add a failing happy-path CLI test**

Add these imports to `internal/cli/bookmarks_test.go`:

```go
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"github.com/y-writings/xapi-usecase/internal/tokenstore"
	"github.com/y-writings/xapi-usecase/internal/xapi"
	"github.com/y-writings/xapi-usecase/internal/xoauth"
```

Append these helpers and test to `internal/cli/bookmarks_test.go`:

```go
func TestBookmarksListPrintsPrettyJSONForUnexpiredToken(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-123",
		TokenType:    "bearer",
		Scope:        xoauth.DefaultScope,
		ExpiresAt:    fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			assertAuthorization(t, r, "Bearer access-123")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`)
		case "/2/users/2244994945/bookmarks":
			assertAuthorization(t, r, "Bearer access-123")
			if got := r.URL.Query().Get("tweet.fields"); got != "created_at,author_id" {
				t.Fatalf("tweet.fields = %q, want created_at,author_id", got)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"1501258597237342208","text":"hello"}],"meta":{"result_count":1}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, stdout, stderr := runBookmarksListForTest(t, []string{"--token-file", tokenFile}, getenvNone)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	want := "{\n  \"data\": [\n    {\n      \"id\": \"1501258597237342208\",\n      \"text\": \"hello\"\n    }\n  ],\n  \"meta\": {\n    \"result_count\": 1\n  }\n}\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func runBookmarksListForTest(t *testing.T, args []string, getenv getenvFunc) (int, string, string) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := bookmarksList(context.Background(), args, &stdout, &stderr, getenv)
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	if errors.Is(err, errHelpRequested) {
		return 0, stdout.String(), stderr.String()
	}
	var commandLine commandLineError
	if errors.As(err, &commandLine) {
		fmt.Fprintf(&stderr, "Error: %v\n", err)
		return 2, stdout.String(), stderr.String()
	}
	fmt.Fprintf(&stderr, "Error: %v\n", err)
	return 1, stdout.String(), stderr.String()
}

func useTestClients(t *testing.T, server *httptest.Server, fixedNow time.Time) {
	t.Helper()

	originalNow := timeNow
	originalOAuthClient := newOAuthClient
	originalXAPIClient := newXAPIClient
	t.Cleanup(func() {
		timeNow = originalNow
		newOAuthClient = originalOAuthClient
		newXAPIClient = originalXAPIClient
	})

	timeNow = func() time.Time { return fixedNow }
	newOAuthClient = func(clientID string) *xoauth.Client {
		return &xoauth.Client{
			ClientID:      clientID,
			HTTPClient:    server.Client(),
			TokenEndpoint: server.URL + "/2/oauth2/token",
			Now:           func() time.Time { return fixedNow },
		}
	}
	newXAPIClient = func(accessToken string) *xapi.Client {
		return &xapi.Client{AccessToken: accessToken, BaseURL: server.URL, HTTPClient: server.Client()}
	}
}

func assertAuthorization(t *testing.T, r *http.Request, want string) {
	t.Helper()

	if got := r.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization = %q, want %s", got, want)
	}
}
```

Add `errors` to the import list in `internal/cli/bookmarks_test.go` because `runBookmarksListForTest` uses it.

- [ ] **Step 2: Run the happy-path test and verify failure**

Run: `go test ./internal/cli -run TestBookmarksListPrintsPrettyJSONForUnexpiredToken -count=1`

Expected: FAIL with `bookmarks list is not implemented`.

- [ ] **Step 3: Implement bookmark flag parsing, API calls, and pretty JSON**

Replace `internal/cli/bookmarks.go` with this content:

```go
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/y-writings/xapi-usecase/internal/tokenstore"
	"github.com/y-writings/xapi-usecase/internal/xapi"
	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

const (
	defaultBookmarksTimeout    = 30 * time.Second
	defaultBookmarkTweetFields = "created_at,author_id"
)

var (
	timeNow = time.Now
	newOAuthClient = xoauth.NewClient
	newXAPIClient = func(accessToken string) *xapi.Client {
		return &xapi.Client{AccessToken: accessToken}
	}
)

type bookmarksListOptions struct {
	ClientID        string
	TokenFile       string
	MaxResults      int
	PaginationToken string
	TweetFields     string
	Expansions      string
	UserFields      string
	MediaFields     string
	PollFields      string
	PlaceFields     string
	Timeout         time.Duration
}

func bookmarksList(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) error {
	options, err := parseBookmarksListOptions(args, stdout, stderr, getenv)
	if err != nil {
		return err
	}

	commandCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	token, err := tokenstore.Load(options.TokenFile)
	if err != nil {
		return err
	}

	client := newXAPIClient(token.AccessToken)
	me, err := client.Me(commandCtx)
	if err != nil {
		return err
	}

	raw, err := client.BookmarksRaw(commandCtx, me.Data.ID, xapi.BookmarkOptions{
		MaxResults:      options.MaxResults,
		PaginationToken: options.PaginationToken,
		TweetFields:     options.TweetFields,
		Expansions:      options.Expansions,
		UserFields:      options.UserFields,
		MediaFields:     options.MediaFields,
		PollFields:      options.PollFields,
		PlaceFields:     options.PlaceFields,
	})
	if err != nil {
		return err
	}

	return writePrettyJSON(stdout, raw)
}

func parseBookmarksListOptions(args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) (bookmarksListOptions, error) {
	options := bookmarksListOptions{
		ClientID:    getenv(clientIDEnv),
		TweetFields: defaultBookmarkTweetFields,
		Timeout:     defaultBookmarksTimeout,
	}

	flags := flag.NewFlagSet("xapi-usecase bookmarks list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.ClientID, "client-id", options.ClientID, "OAuth2 client ID used only when refresh is needed")
	flags.StringVar(&options.TokenFile, "token-file", options.TokenFile, "path to read and update the OAuth2 token")
	flags.IntVar(&options.MaxResults, "max-results", options.MaxResults, "results per page, 1 through 100")
	flags.StringVar(&options.PaginationToken, "pagination-token", options.PaginationToken, "page token from meta.next_token")
	flags.StringVar(&options.TweetFields, "tweet-fields", options.TweetFields, "comma-separated tweet.fields")
	flags.StringVar(&options.Expansions, "expansions", options.Expansions, "comma-separated expansions")
	flags.StringVar(&options.UserFields, "user-fields", options.UserFields, "comma-separated user.fields")
	flags.StringVar(&options.MediaFields, "media-fields", options.MediaFields, "comma-separated media.fields")
	flags.StringVar(&options.PollFields, "poll-fields", options.PollFields, "comma-separated poll.fields")
	flags.StringVar(&options.PlaceFields, "place-fields", options.PlaceFields, "comma-separated place.fields")
	flags.DurationVar(&options.Timeout, "timeout", options.Timeout, "command timeout")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printBookmarksListUsage(stdout)
			return bookmarksListOptions{}, errHelpRequested
		}
		printBookmarksListUsage(stderr)
		return bookmarksListOptions{}, commandLineError(err.Error())
	}
	if flags.NArg() > 0 {
		printBookmarksListUsage(stderr)
		return bookmarksListOptions{}, commandLineError(fmt.Sprintf("unexpected argument: %s", flags.Arg(0)))
	}
	if options.Timeout <= 0 {
		return bookmarksListOptions{}, commandLineError("--timeout must be greater than 0")
	}
	if options.TokenFile == "" {
		path, err := tokenstore.DefaultPath()
		if err != nil {
			return bookmarksListOptions{}, err
		}
		options.TokenFile = path
	}

	return options, nil
}

func writePrettyJSON(w io.Writer, raw []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return err
	}
	if _, err := pretty.WriteTo(w); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}
```

- [ ] **Step 4: Run the happy-path test and verify pass**

Run: `go test ./internal/cli -run TestBookmarksListPrintsPrettyJSONForUnexpiredToken -count=1`

Expected: PASS.

- [ ] **Step 5: Run all CLI tests and verify pass**

Run: `go test ./internal/cli -count=1`

Expected: PASS.

## Task 5: Bookmark CLI Flag Validation

**Files:**
- Modify: `internal/cli/bookmarks_test.go`

- [ ] **Step 1: Add failing tests for max-results and custom query flags**

Append these tests to `internal/cli/bookmarks_test.go`:

```go
func TestBookmarksListRejectsInvalidMaxResults(t *testing.T) {
	for _, value := range []string{"0", "101"} {
		t.Run(value, func(t *testing.T) {
			code, _, stderr := runBookmarksListForTest(t, []string{"--max-results", value}, getenvNone)

			if code != 2 {
				t.Fatalf("code = %d, want 2", code)
			}
			if !strings.Contains(stderr, "--max-results must be between 1 and 100") {
				t.Fatalf("stderr = %q, want max-results error", stderr)
			}
		})
	}
}

func TestBookmarksListSendsCustomQueryFlags(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{AccessToken: "access-123", ExpiresAt: fixedNow.Add(time.Hour)}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			fmt.Fprint(w, `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`)
		case "/2/users/2244994945/bookmarks":
			query := r.URL.Query()
			for key, want := range map[string]string{
				"max_results":      "100",
				"pagination_token": "next-token",
				"tweet.fields":     "created_at,public_metrics,author_id",
				"expansions":       "author_id",
				"user.fields":      "username,verified",
				"media.fields":     "url",
				"poll.fields":      "options",
				"place.fields":     "full_name",
			} {
				if got := query.Get(key); got != want {
					t.Fatalf("query[%s] = %q, want %q", key, got, want)
				}
			}
			fmt.Fprint(w, `{"data":[],"meta":{"result_count":0}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(t, []string{
		"--token-file", tokenFile,
		"--max-results", "100",
		"--pagination-token", "next-token",
		"--tweet-fields", "created_at,public_metrics,author_id",
		"--expansions", "author_id",
		"--user-fields", "username,verified",
		"--media-fields", "url",
		"--poll-fields", "options",
		"--place-fields", "full_name",
	}, getenvNone)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
}
```

- [ ] **Step 2: Run the flag tests and verify failure**

Run: `go test ./internal/cli -run 'TestBookmarksList(RejectsInvalidMaxResults|SendsCustomQueryFlags)' -count=1`

Expected: FAIL because invalid `--max-results` values reach token loading instead of returning a command-line error.

- [ ] **Step 3: Implement max-results validation**

Add this constant to the const block in `internal/cli/bookmarks.go`:

```go
	maxBookmarkResultsInclusive = 100
```

Add this validation block after the `flags.NArg()` check and before the timeout validation:

```go
	if flagIsSet(flags, "max-results") && (options.MaxResults < 1 || options.MaxResults > maxBookmarkResultsInclusive) {
		return bookmarksListOptions{}, commandLineError("--max-results must be between 1 and 100")
	}
```

Add this helper before `writePrettyJSON`:

```go
func flagIsSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == name {
			set = true
		}
	})
	return set
}
```

- [ ] **Step 4: Run the flag tests and verify pass**

Run: `go test ./internal/cli -run 'TestBookmarksList(RejectsInvalidMaxResults|SendsCustomQueryFlags)' -count=1`

Expected: PASS.

- [ ] **Step 5: Checkpoint**

Run: `git diff -- internal/cli/bookmarks.go internal/cli/bookmarks_test.go`

Expected: diff contains parser validation and custom query flag tests.

## Task 6: Preemptive Token Refresh

**Files:**
- Modify: `internal/cli/bookmarks.go`
- Modify: `internal/cli/bookmarks_test.go`

- [ ] **Step 1: Add failing tests for refresh before API calls**

Append these tests to `internal/cli/bookmarks_test.go`:

```go
func TestBookmarksListRefreshesTokenExpiringWithinFiveMinutes(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		TokenType:    "bearer",
		Scope:        xoauth.DefaultScope,
		ExpiresAt:    fixedNow.Add(4 * time.Minute),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/oauth2/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := r.Form.Get("client_id"); got != "client-123" {
				t.Fatalf("client_id = %q, want client-123", got)
			}
			if got := r.Form.Get("refresh_token"); got != "old-refresh" {
				t.Fatalf("refresh_token = %q, want old-refresh", got)
			}
			fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"bearer","scope":"tweet.read users.read bookmark.read offline.access","expires_in":3600}`)
		case "/2/users/me":
			assertAuthorization(t, r, "Bearer new-access")
			fmt.Fprint(w, `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`)
		case "/2/users/2244994945/bookmarks":
			assertAuthorization(t, r, "Bearer new-access")
			fmt.Fprint(w, `{"data":[],"meta":{"result_count":0}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(t, []string{"--token-file", tokenFile}, func(key string) string {
		if key == clientIDEnv {
			return "client-123"
		}
		return ""
	})

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	saved, err := tokenstore.Load(tokenFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if saved.AccessToken != "new-access" {
		t.Fatalf("saved AccessToken = %q, want new-access", saved.AccessToken)
	}
	if saved.RefreshToken != "new-refresh" {
		t.Fatalf("saved RefreshToken = %q, want new-refresh", saved.RefreshToken)
	}
}

func TestBookmarksListRequiresClientIDOnlyWhenRefreshIsNeeded(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: fixedNow.Add(4 * time.Minute)}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s", r.URL.Path)
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(t, []string{"--token-file", tokenFile}, getenvNone)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "client ID is required to refresh access token") {
		t.Fatalf("stderr = %q, want client ID refresh error", stderr)
	}
}
```

- [ ] **Step 2: Run refresh tests and verify failure**

Run: `go test ./internal/cli -run 'TestBookmarksList(RefreshesTokenExpiringWithinFiveMinutes|RequiresClientIDOnlyWhenRefreshIsNeeded)' -count=1`

Expected: FAIL because old access token is still used and missing client ID is not checked before API calls.

- [ ] **Step 3: Implement preemptive refresh**

In `internal/cli/bookmarks.go`, add this constant to the existing const block:

```go
	bookmarkRefreshSkew          = 5 * time.Minute
```

After loading the token in `bookmarksList`, insert this block before creating the X API client:

```go
	if bookmarkTokenNeedsRefresh(token.ExpiresAt) {
		token, err = refreshBookmarkAccessToken(commandCtx, options, token)
		if err != nil {
			return err
		}
	}
```

Add these helper functions to `internal/cli/bookmarks.go`:

```go
func bookmarkTokenNeedsRefresh(expiresAt time.Time) bool {
	return !expiresAt.After(timeNow().Add(bookmarkRefreshSkew))
}

func refreshBookmarkAccessToken(ctx context.Context, options bookmarksListOptions, current xoauth.Token) (xoauth.Token, error) {
	if options.ClientID == "" {
		return xoauth.Token{}, commandLineError(fmt.Sprintf("client ID is required to refresh access token; set %s or pass --client-id", clientIDEnv))
	}
	if current.RefreshToken == "" {
		return xoauth.Token{}, errors.New("refresh token is required to refresh access token")
	}

	client := newOAuthClient(options.ClientID)
	refreshed, err := client.Refresh(ctx, current)
	if err != nil {
		return xoauth.Token{}, err
	}
	if err := tokenstore.Save(options.TokenFile, refreshed); err != nil {
		return xoauth.Token{}, err
	}

	return refreshed, nil
}
```

- [ ] **Step 4: Run refresh tests and verify pass**

Run: `go test ./internal/cli -run 'TestBookmarksList(RefreshesTokenExpiringWithinFiveMinutes|RequiresClientIDOnlyWhenRefreshIsNeeded)' -count=1`

Expected: PASS.

- [ ] **Step 5: Run all CLI tests**

Run: `go test ./internal/cli -count=1`

Expected: PASS.

## Task 7: One-Time Refresh Retry On 401

**Files:**
- Modify: `internal/cli/bookmarks.go`
- Modify: `internal/cli/bookmarks_test.go`

- [ ] **Step 1: Add failing tests for 401 retry**

Append these tests to `internal/cli/bookmarks_test.go`:

```go
func TestBookmarksListRefreshesAndRetriesMeAfterUnauthorized(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: fixedNow.Add(time.Hour)}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	meRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			meRequests++
			if meRequests == 1 {
				assertAuthorization(t, r, "Bearer old-access")
				http.Error(w, `{"title":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			assertAuthorization(t, r, "Bearer new-access")
			fmt.Fprint(w, `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`)
		case "/2/oauth2/token":
			fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"bearer","scope":"tweet.read users.read bookmark.read offline.access","expires_in":3600}`)
		case "/2/users/2244994945/bookmarks":
			assertAuthorization(t, r, "Bearer new-access")
			fmt.Fprint(w, `{"data":[],"meta":{"result_count":0}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(t, []string{"--token-file", tokenFile}, func(key string) string {
		if key == clientIDEnv {
			return "client-123"
		}
		return ""
	})

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if meRequests != 2 {
		t.Fatalf("me requests = %d, want 2", meRequests)
	}
}

func TestBookmarksListRefreshesAndRetriesBookmarksAfterUnauthorized(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: fixedNow.Add(time.Hour)}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	bookmarkRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			assertAuthorization(t, r, "Bearer old-access")
			fmt.Fprint(w, `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`)
		case "/2/users/2244994945/bookmarks":
			bookmarkRequests++
			if bookmarkRequests == 1 {
				assertAuthorization(t, r, "Bearer old-access")
				http.Error(w, `{"title":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			assertAuthorization(t, r, "Bearer new-access")
			fmt.Fprint(w, `{"data":[],"meta":{"result_count":0}}`)
		case "/2/oauth2/token":
			fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"bearer","scope":"tweet.read users.read bookmark.read offline.access","expires_in":3600}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(t, []string{"--token-file", tokenFile}, func(key string) string {
		if key == clientIDEnv {
			return "client-123"
		}
		return ""
	})

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if bookmarkRequests != 2 {
		t.Fatalf("bookmark requests = %d, want 2", bookmarkRequests)
	}
}
```

- [ ] **Step 2: Run 401 retry tests and verify failure**

Run: `go test ./internal/cli -run 'TestBookmarksListRefreshesAndRetries.*Unauthorized' -count=1`

Expected: FAIL because a `401` returns without retry.

- [ ] **Step 3: Implement 401 retry wrappers**

Add `net/http` to the imports in `internal/cli/bookmarks.go`.

Replace the direct `/2/users/me` and bookmarks calls in `bookmarksList` with this code:

```go
	client := newXAPIClient(token.AccessToken)
	refreshAfterUnauthorized := func() error {
		refreshed, err := refreshBookmarkAccessToken(commandCtx, options, token)
		if err != nil {
			return err
		}
		token = refreshed
		client = newXAPIClient(token.AccessToken)
		return nil
	}

	var me xapi.MeResponse
	if err := runBookmarkAPIWithRefreshRetry(refreshAfterUnauthorized, func() error {
		var err error
		me, err = client.Me(commandCtx)
		return err
	}); err != nil {
		return err
	}

	var raw []byte
	if err := runBookmarkAPIWithRefreshRetry(refreshAfterUnauthorized, func() error {
		var err error
		raw, err = client.BookmarksRaw(commandCtx, me.Data.ID, xapi.BookmarkOptions{
			MaxResults:      options.MaxResults,
			PaginationToken: options.PaginationToken,
			TweetFields:     options.TweetFields,
			Expansions:      options.Expansions,
			UserFields:      options.UserFields,
			MediaFields:     options.MediaFields,
			PollFields:      options.PollFields,
			PlaceFields:     options.PlaceFields,
		})
		return err
	}); err != nil {
		return err
	}
```

Add these helpers to `internal/cli/bookmarks.go`:

```go
func runBookmarkAPIWithRefreshRetry(refresh func() error, call func() error) error {
	err := call()
	if !isUnauthorizedXAPIError(err) {
		return err
	}
	if err := refresh(); err != nil {
		return err
	}
	return call()
}

func isUnauthorizedXAPIError(err error) bool {
	var httpError xapi.HTTPError
	return errors.As(err, &httpError) && httpError.StatusCode == http.StatusUnauthorized
}
```

- [ ] **Step 4: Run 401 retry tests and verify pass**

Run: `go test ./internal/cli -run 'TestBookmarksListRefreshesAndRetries.*Unauthorized' -count=1`

Expected: PASS.

- [ ] **Step 5: Run all CLI tests**

Run: `go test ./internal/cli -count=1`

Expected: PASS.

## Task 8: JSON Response Edge Cases

**Files:**
- Modify: `internal/cli/bookmarks_test.go`

- [ ] **Step 1: Add tests for JSON `errors` and invalid successful JSON**

Append these tests to `internal/cli/bookmarks_test.go`:

```go
func TestBookmarksListExitsZeroWhenSuccessfulJSONContainsErrors(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{AccessToken: "access-123", ExpiresAt: fixedNow.Add(time.Hour)}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			fmt.Fprint(w, `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`)
		case "/2/users/2244994945/bookmarks":
			fmt.Fprint(w, `{"errors":[{"title":"partial"}],"meta":{"result_count":0}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, stdout, stderr := runBookmarksListForTest(t, []string{"--token-file", tokenFile}, getenvNone)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "\"errors\"") {
		t.Fatalf("stdout = %q, want errors JSON", stdout)
	}
}

func TestBookmarksListReturnsErrorForInvalidSuccessfulJSON(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{AccessToken: "access-123", ExpiresAt: fixedNow.Add(time.Hour)}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			fmt.Fprint(w, `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`)
		case "/2/users/2244994945/bookmarks":
			fmt.Fprint(w, `not-json`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(t, []string{"--token-file", tokenFile}, getenvNone)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "invalid character") {
		t.Fatalf("stderr = %q, want invalid JSON error", stderr)
	}
}
```

- [ ] **Step 2: Run JSON edge tests**

Run: `go test ./internal/cli -run 'TestBookmarksList(ExitsZeroWhenSuccessfulJSONContainsErrors|ReturnsErrorForInvalidSuccessfulJSON)' -count=1`

Expected: PASS. The implementation from Task 4 already pretty-prints any valid HTTP 2xx JSON and errors on invalid JSON.

- [ ] **Step 3: Run all Go tests**

Run: `go test ./...`

Expected: PASS.

## Task 9: README Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README bookmark usage**

In `README.md`, replace the `## Current Limitations` section with this content:

````markdown
## Bookmark Retrieval

After `auth login` saves a token, retrieve one page of bookmarked Posts:

```sh
go run ./cmd/xapi-usecase bookmarks list
```

The command resolves the authenticated user with `/2/users/me`, calls `/2/users/{id}/bookmarks`, and prints the X API JSON response pretty-formatted on stdout.

By default, the command requests:

```text
tweet.fields=created_at,author_id
```

Use pagination options to request a specific page size or continue from `meta.next_token`:

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

The required X OAuth scopes are `bookmark.read`, `tweet.read`, and `users.read`. `offline.access` is required for automatic refresh.
````

- [ ] **Step 2: Run documentation-adjacent verification**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Check README diff**

Run: `git diff -- README.md`

Expected: README documents login-first bookmark retrieval, pagination, fields, refresh client ID behavior, and required scopes.

## Task 10: Final Verification

**Files:**
- All modified files

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run diff whitespace check**

Run: `git diff --check`

Expected: no output.

- [ ] **Step 3: Review changed files**

Run: `git status --short`

Expected: modified or new files are limited to:

```text
README.md
docs/superpowers/specs/2026-06-11-x-bookmarks-list-design.md
docs/superpowers/plans/2026-06-11-x-bookmarks-list.md
internal/cli/bookmarks.go
internal/cli/bookmarks_test.go
internal/cli/cli.go
internal/tokenstore/tokenstore.go
internal/tokenstore/tokenstore_test.go
internal/xapi/client.go
internal/xapi/client_test.go
```

- [ ] **Step 4: Review implementation against spec**

Confirm these requirements are implemented:

- `bookmarks list` exists and prints only JSON on stdout.
- Token file loading requires `access_token` and valid `expires_at`.
- Refresh happens within five minutes of expiry.
- Refresh uses `client_id` only when refresh is needed.
- `/2/users/me` resolves the authenticated user ID.
- Bookmark retrieval requests one page only.
- `401` from `/2/users/me` and bookmarks each gets one refresh retry.
- `403` is not retried.
- HTTP 2xx JSON containing `errors` exits `0`.
- Invalid HTTP 2xx JSON exits `1`.

- [ ] **Step 5: Report verification evidence**

In the final response, include the exact `go test ./...` and `git diff --check` results. Do not claim completion without these command outputs.
