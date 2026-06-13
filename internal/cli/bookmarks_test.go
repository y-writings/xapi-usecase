package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/y-writings/xapi-usecase/internal/tokenstore"
	"github.com/y-writings/xapi-usecase/internal/xapi"
	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

const (
	userMeJSON = `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`

	emptyBookmarksJSON = `{"data":[],"meta":{"result_count":0}}`

	bookmarkWithOnePostJSON = `{"data":[{"id":"1501258597237342208","text":"hello"}],` +
		`"meta":{"result_count":1}}`

	newAccessTokenJSON = `{"access_token":"new-access","refresh_token":"new-refresh",` +
		`"token_type":"bearer",` +
		`"scope":"tweet.read users.read bookmark.read offline.access",` +
		`"expires_in":3600}`
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

	code := Run(
		context.Background(),
		[]string{"auth", "login", "--help"},
		&stdout,
		&stderr,
		getenvNone,
	)

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

	code := Run(
		context.Background(),
		[]string{"bookmarks", "list", "--help"},
		&stdout,
		&stderr,
		getenvNone,
	)

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
			writeResponse(t, w, userMeJSON)
		case "/2/users/2244994945/bookmarks":
			assertAuthorization(t, r, "Bearer access-123")
			if got := r.URL.Query().Get("tweet.fields"); got != "created_at,author_id" {
				t.Fatalf("tweet.fields = %q, want created_at,author_id", got)
			}
			w.Header().Set("Content-Type", "application/json")
			writeResponse(t, w, bookmarkWithOnePostJSON)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, stdout, stderr := runBookmarksListForTest(
		t,
		[]string{"--token-file", tokenFile},
		getenvNone,
	)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	want := `{
  "data": [
    {
      "id": "1501258597237342208",
      "text": "hello"
    }
  ],
  "meta": {
    "result_count": 1
  }
}
`
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestBookmarksListRejectsInvalidMaxResults(t *testing.T) {
	for _, value := range []string{"0", "101"} {
		t.Run(value, func(t *testing.T) {
			code, _, stderr := runBookmarksListForTest(
				t,
				[]string{"--max-results", value},
				getenvNone,
			)

			if code != 2 {
				t.Fatalf("code = %d, want 2", code)
			}
			if !strings.Contains(stderr, "--max-results must be between 1 and 100") {
				t.Fatalf("stderr = %q, want max-results error", stderr)
			}
		})
	}
}

func TestBookmarksListAcceptsMinimumMaxResults(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken: "access-123",
		ExpiresAt:   fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			writeResponse(t, w, userMeJSON)
		case "/2/users/2244994945/bookmarks":
			if got := r.URL.Query().Get("max_results"); got != "1" {
				t.Fatalf("max_results = %q, want 1", got)
			}
			writeResponse(t, w, emptyBookmarksJSON)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(
		t,
		[]string{"--token-file", tokenFile, "--max-results", "1"},
		getenvNone,
	)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
}

func TestBookmarksListSendsCustomQueryFlags(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken: "access-123",
		ExpiresAt:   fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			writeResponse(t, w, userMeJSON)
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
			writeResponse(t, w, emptyBookmarksJSON)
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
			writeResponse(t, w, newAccessTokenJSON)
		case "/2/users/me":
			assertAuthorization(t, r, "Bearer new-access")
			writeResponse(t, w, userMeJSON)
		case "/2/users/2244994945/bookmarks":
			assertAuthorization(t, r, "Bearer new-access")
			writeResponse(t, w, emptyBookmarksJSON)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(
		t,
		[]string{"--token-file", tokenFile},
		clientIDEnvGetter,
	)

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
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    fixedNow.Add(4 * time.Minute),
	}); err != nil {
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

func TestBookmarksListRefreshesAndRetriesMeAfterUnauthorized(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    fixedNow.Add(time.Hour),
	}); err != nil {
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
			writeResponse(t, w, userMeJSON)
		case "/2/oauth2/token":
			writeResponse(t, w, newAccessTokenJSON)
		case "/2/users/2244994945/bookmarks":
			assertAuthorization(t, r, "Bearer new-access")
			writeResponse(t, w, emptyBookmarksJSON)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(
		t,
		[]string{"--token-file", tokenFile},
		clientIDEnvGetter,
	)

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
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	bookmarkRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			assertAuthorization(t, r, "Bearer old-access")
			writeResponse(t, w, userMeJSON)
		case "/2/users/2244994945/bookmarks":
			bookmarkRequests++
			if bookmarkRequests == 1 {
				assertAuthorization(t, r, "Bearer old-access")
				http.Error(w, `{"title":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			assertAuthorization(t, r, "Bearer new-access")
			writeResponse(t, w, emptyBookmarksJSON)
		case "/2/oauth2/token":
			writeResponse(t, w, newAccessTokenJSON)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(
		t,
		[]string{"--token-file", tokenFile},
		clientIDEnvGetter,
	)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if bookmarkRequests != 2 {
		t.Fatalf("bookmark requests = %d, want 2", bookmarkRequests)
	}
}

func TestBookmarksListDoesNotRefreshOrRetryAfterForbidden(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-123",
		ExpiresAt:    fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	bookmarkRequests := 0
	refreshRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			writeResponse(t, w, userMeJSON)
		case "/2/users/2244994945/bookmarks":
			bookmarkRequests++
			http.Error(w, `{"title":"Forbidden"}`, http.StatusForbidden)
		case "/2/oauth2/token":
			refreshRequests++
			writeResponse(t, w, newAccessTokenJSON)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, _, stderr := runBookmarksListForTest(
		t,
		[]string{"--token-file", tokenFile},
		clientIDEnvGetter,
	)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if bookmarkRequests != 1 {
		t.Fatalf("bookmark requests = %d, want 1", bookmarkRequests)
	}
	if refreshRequests != 0 {
		t.Fatalf("refresh requests = %d, want 0", refreshRequests)
	}
	if !strings.Contains(stderr, "403 Forbidden") {
		t.Fatalf("stderr = %q, want 403 Forbidden", stderr)
	}
}

func TestBookmarksListExitsZeroWhenSuccessfulJSONContainsErrors(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken: "access-123",
		ExpiresAt:   fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			writeResponse(t, w, userMeJSON)
		case "/2/users/2244994945/bookmarks":
			writeResponse(t, w, `{"errors":[{"title":"partial"}],"meta":{"result_count":0}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	code, stdout, stderr := runBookmarksListForTest(
		t,
		[]string{"--token-file", tokenFile},
		getenvNone,
	)

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
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken: "access-123",
		ExpiresAt:   fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2/users/me":
			writeResponse(t, w, userMeJSON)
		case "/2/users/2244994945/bookmarks":
			writeResponse(t, w, `not-json`)
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
		return &xapi.Client{
			AccessToken: accessToken,
			BaseURL:     server.URL,
			HTTPClient:  server.Client(),
		}
	}
}

func writeResponse(t *testing.T, w io.Writer, body string) {
	t.Helper()

	if _, err := fmt.Fprint(w, body); err != nil {
		t.Fatalf("Fprint(response) error = %v", err)
	}
}

func clientIDEnvGetter(key string) string {
	if key == clientIDEnv {
		return "client-123"
	}
	return ""
}

func assertAuthorization(t *testing.T, r *http.Request, want string) {
	t.Helper()

	if got := r.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization = %q, want %s", got, want)
	}
}

func getenvNone(string) string {
	return ""
}
