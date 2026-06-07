# X OAuth2 PKCE Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standard-library Go CLI foundation for X API v2 OAuth 2.0 Authorization Code Flow with PKCE, local login, secure token storage, and bearer-token API requests.

**Architecture:** Initialize a Go module and split responsibilities across `cmd/xapi-usecase`, `internal/cli`, `internal/xoauth`, `internal/tokenstore`, and `internal/xapi`. Keep OAuth protocol logic, callback handling, token persistence, API request handling, and CLI orchestration independently testable.

**Tech Stack:** Go `1.26.3`, standard library only, `net/http/httptest` for HTTP tests.

**Repository Rule:** Do not commit unless the user explicitly asks for a commit. Use diff/status checkpoints instead of commit steps.

---

## File Structure

- Create: `go.mod` for module `github.com/y-writings/xapi-usecase` and Go `1.26.3`.
- Modify: `.mise/config.toml` to declare Go `1.26.3` under `[tools]`.
- Create: `cmd/xapi-usecase/main.go` as the thin process entry point.
- Create: `internal/cli/cli.go` for command parsing and `auth login` orchestration.
- Create: `internal/cli/callback.go` for the callback HTTP handler and callback result type.
- Create: `internal/cli/callback_test.go` for callback validation tests.
- Create: `internal/xoauth/pkce.go` for verifier, challenge, and state generation.
- Create: `internal/xoauth/client.go` for auth URL, token exchange, refresh, token types, and OAuth HTTP errors.
- Create: `internal/xoauth/pkce_test.go` for PKCE/state/auth URL tests.
- Create: `internal/xoauth/client_test.go` for token exchange/refresh/error tests.
- Create: `internal/tokenstore/tokenstore.go` for JSON token persistence and permissions.
- Create: `internal/tokenstore/tokenstore_test.go` for save format and permission tests.
- Create: `internal/xapi/client.go` for bearer-token API requests, HTTP errors, and `/2/users/me`.
- Create: `internal/xapi/client_test.go` for bearer header, error body, and `Me` tests.
- Create: `README.md` for X Developer Console setup and CLI usage.

## Task 1: Module And Tooling

**Files:**
- Create: `go.mod`
- Modify: `.mise/config.toml`

- [ ] **Step 1: Create `go.mod`**

Create `go.mod` with exactly:

```go
module github.com/y-writings/xapi-usecase

go 1.26.3
```

- [ ] **Step 2: Modify `.mise/config.toml`**

Replace `.mise/config.toml` with:

```toml
# This TOML file contains project-specific settings.
# Add project-specific information here.

[tools]
go = "1.26.3"
```

- [ ] **Step 3: Run module sanity check**

Run: `go test ./...`

Expected if Go is available: command succeeds with no packages or reports no packages to test.

Expected in the current environment if Go is still missing: failure containing `command not found: go`. Record this and continue writing code; final verification must report the exact failure if unresolved.

## Task 2: PKCE, State, And Authorization URL

**Files:**
- Create: `internal/xoauth/pkce_test.go`
- Create: `internal/xoauth/pkce.go`
- Create: `internal/xoauth/client.go`

- [ ] **Step 1: Write failing PKCE and auth URL tests**

Create `internal/xoauth/pkce_test.go`:

```go
package xoauth

import (
	"net/url"
	"regexp"
	"testing"
)

func TestCodeChallengeS256UsesRFC7636Example(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	if got := CodeChallengeS256(verifier); got != want {
		t.Fatalf("CodeChallengeS256() = %q, want %q", got, want)
	}
}

func TestGenerateCodeVerifierReturnsPKCECompatibleValue(t *testing.T) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() error = %v", err)
	}

	assertRandomTokenShape(t, verifier)
}

func TestGenerateStateReturnsRandomURLSafeValue(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}

	assertRandomTokenShape(t, state)
}

func TestAuthURLContainsRequiredQuery(t *testing.T) {
	client := NewClient("client-123")

	got, err := client.AuthURL("http://127.0.0.1:8765/callback", "state-123", "challenge-123")
	if err != nil {
		t.Fatalf("AuthURL() error = %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if parsed.Scheme != "https" || parsed.Host != "x.com" || parsed.Path != "/i/oauth2/authorize" {
		t.Fatalf("AuthURL endpoint = %s://%s%s, want https://x.com/i/oauth2/authorize", parsed.Scheme, parsed.Host, parsed.Path)
	}

	query := parsed.Query()
	assertQueryValue(t, query, "response_type", "code")
	assertQueryValue(t, query, "client_id", "client-123")
	assertQueryValue(t, query, "redirect_uri", "http://127.0.0.1:8765/callback")
	assertQueryValue(t, query, "scope", DefaultScope)
	assertQueryValue(t, query, "state", "state-123")
	assertQueryValue(t, query, "code_challenge", "challenge-123")
	assertQueryValue(t, query, "code_challenge_method", "S256")
}

func TestAuthURLRequiresClientID(t *testing.T) {
	client := NewClient("")

	_, err := client.AuthURL("http://127.0.0.1:8765/callback", "state", "challenge")
	if err == nil {
		t.Fatal("AuthURL() error = nil, want error")
	}
}

func assertRandomTokenShape(t *testing.T, value string) {
	t.Helper()

	if len(value) != 43 {
		t.Fatalf("len(value) = %d, want 43", len(value))
	}

	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(value) {
		t.Fatalf("value = %q, want raw URL-safe base64", value)
	}
}

func assertQueryValue(t *testing.T, query url.Values, key string, want string) {
	t.Helper()

	if got := query.Get(key); got != want {
		t.Fatalf("query[%s] = %q, want %q", key, got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/xoauth`

Expected if Go is available: FAIL because `CodeChallengeS256`, `GenerateCodeVerifier`, `GenerateState`, `NewClient`, and `DefaultScope` do not exist.

Expected if Go is missing: failure containing `command not found: go`.

- [ ] **Step 3: Implement PKCE primitives**

Create `internal/xoauth/pkce.go`:

```go
package xoauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const randomTokenBytes = 32

func GenerateCodeVerifier() (string, error) {
	return randomToken()
}

func GenerateState() (string, error) {
	return randomToken()
}

func CodeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	bytes := make([]byte, randomTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
```

- [ ] **Step 4: Implement client constants and auth URL**

Create `internal/xoauth/client.go` with the auth URL portion first:

```go
package xoauth

import (
	"errors"
	"net/http"
	"net/url"
	"time"
)

const (
	AuthorizeEndpoint = "https://x.com/i/oauth2/authorize"
	TokenEndpoint     = "https://api.x.com/2/oauth2/token"
	DefaultScope      = "tweet.read users.read bookmark.read offline.access"
	challengeMethod   = "S256"
)

type Client struct {
	ClientID          string
	HTTPClient        *http.Client
	AuthorizeEndpoint string
	TokenEndpoint     string
	Now               func() time.Time
}

func NewClient(clientID string) *Client {
	return &Client{ClientID: clientID}
}

func (c *Client) AuthURL(redirectURI string, state string, codeChallenge string) (string, error) {
	if c.ClientID == "" {
		return "", errors.New("client_id is required")
	}

	endpoint := c.authorizeEndpoint()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	query := parsed.Query()
	query.Set("response_type", "code")
	query.Set("client_id", c.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", DefaultScope)
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", challengeMethod)
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func (c *Client) authorizeEndpoint() string {
	if c.AuthorizeEndpoint != "" {
		return c.AuthorizeEndpoint
	}

	return AuthorizeEndpoint
}

func (c *Client) tokenEndpoint() string {
	if c.TokenEndpoint != "" {
		return c.TokenEndpoint
	}

	return TokenEndpoint
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return http.DefaultClient
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}

	return time.Now()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/xoauth`

Expected if Go is available: PASS.

Expected if Go is missing: failure containing `command not found: go`.

## Task 3: Token Exchange And Refresh

**Files:**
- Modify: `internal/xoauth/client.go`
- Create: `internal/xoauth/client_test.go`

- [ ] **Step 1: Write failing token HTTP tests**

Create `internal/xoauth/client_test.go`:

```go
package xoauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExchangeCodeSendsExpectedFormAndParsesToken(t *testing.T) {
	fixedNow := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q, want application/x-www-form-urlencoded", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}

		assertFormValue(t, r, "grant_type", "authorization_code")
		assertFormValue(t, r, "client_id", "client-123")
		assertFormValue(t, r, "code", "code-123")
		assertFormValue(t, r, "redirect_uri", "http://127.0.0.1:8765/callback")
		assertFormValue(t, r, "code_verifier", "verifier-123")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"access-123","refresh_token":"refresh-123","token_type":"bearer","scope":"tweet.read users.read bookmark.read offline.access","expires_in":7200}`)
	}))
	defer server.Close()

	client := &Client{
		ClientID:      "client-123",
		HTTPClient:    server.Client(),
		TokenEndpoint: server.URL,
		Now:           func() time.Time { return fixedNow },
	}

	token, err := client.ExchangeCode(context.Background(), "code-123", "http://127.0.0.1:8765/callback", "verifier-123")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
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
	if token.Scope != DefaultScope {
		t.Fatalf("Scope = %q, want %q", token.Scope, DefaultScope)
	}
	if want := fixedNow.Add(2 * time.Hour); !token.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %s, want %s", token.ExpiresAt, want)
	}
}

func TestRefreshSendsExpectedFormAndPreservesRefreshTokenWhenOmitted(t *testing.T) {
	fixedNow := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q, want application/x-www-form-urlencoded", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}

		assertFormValue(t, r, "grant_type", "refresh_token")
		assertFormValue(t, r, "client_id", "client-123")
		assertFormValue(t, r, "refresh_token", "old-refresh")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"new-access","token_type":"bearer","scope":"tweet.read users.read bookmark.read offline.access","expires_in":3600}`)
	}))
	defer server.Close()

	client := &Client{
		ClientID:      "client-123",
		HTTPClient:    server.Client(),
		TokenEndpoint: server.URL,
		Now:           func() time.Time { return fixedNow },
	}

	current := Token{RefreshToken: "old-refresh"}
	token, err := client.Refresh(context.Background(), current)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if token.AccessToken != "new-access" {
		t.Fatalf("AccessToken = %q, want new-access", token.AccessToken)
	}
	if token.RefreshToken != "old-refresh" {
		t.Fatalf("RefreshToken = %q, want old-refresh", token.RefreshToken)
	}
	if want := fixedNow.Add(time.Hour); !token.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %s, want %s", token.ExpiresAt, want)
	}
}

func TestRefreshUsesRotatedRefreshTokenWhenReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"bearer","scope":"tweet.read users.read bookmark.read offline.access","expires_in":3600}`)
	}))
	defer server.Close()

	client := &Client{ClientID: "client-123", HTTPClient: server.Client(), TokenEndpoint: server.URL}

	token, err := client.Refresh(context.Background(), Token{RefreshToken: "old-refresh"})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if token.RefreshToken != "new-refresh" {
		t.Fatalf("RefreshToken = %q, want new-refresh", token.RefreshToken)
	}
}

func TestTokenEndpointHTTPErrorIncludesStatusPathAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	client := &Client{
		ClientID:      "client-123",
		HTTPClient:    server.Client(),
		TokenEndpoint: server.URL + "/2/oauth2/token",
	}

	_, err := client.ExchangeCode(context.Background(), "code-123", "http://127.0.0.1:8765/callback", "verifier-123")
	if err == nil {
		t.Fatal("ExchangeCode() error = nil, want error")
	}

	message := err.Error()
	for _, want := range []string{"400 Bad Request", "/2/oauth2/token", `{"error":"invalid_request"}`} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want to contain %q", message, want)
		}
	}
}

func assertFormValue(t *testing.T, r *http.Request, key string, want string) {
	t.Helper()

	if got := r.Form.Get(key); got != want {
		t.Fatalf("form[%s] = %q, want %q", key, got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/xoauth`

Expected if Go is available: FAIL because `Token`, `ExchangeCode`, and `Refresh` do not exist.

Expected if Go is missing: failure containing `command not found: go`.

- [ ] **Step 3: Extend `internal/xoauth/client.go`**

Replace `internal/xoauth/client.go` with:

```go
package xoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	AuthorizeEndpoint = "https://x.com/i/oauth2/authorize"
	TokenEndpoint     = "https://api.x.com/2/oauth2/token"
	DefaultScope      = "tweet.read users.read bookmark.read offline.access"
	challengeMethod   = "S256"
	maxErrorBodyBytes = 4 * 1024
)

type Client struct {
	ClientID          string
	HTTPClient        *http.Client
	AuthorizeEndpoint string
	TokenEndpoint     string
	Now               func() time.Time
}

type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scope        string
	ExpiresAt    time.Time
}

type HTTPError struct {
	StatusCode int
	Status     string
	Path       string
	Body       string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
}

func NewClient(clientID string) *Client {
	return &Client{ClientID: clientID}
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("x oauth request failed: %s %s", e.Status, e.Path)
	}

	return fmt.Sprintf("x oauth request failed: %s %s: %s", e.Status, e.Path, e.Body)
}

func (c *Client) AuthURL(redirectURI string, state string, codeChallenge string) (string, error) {
	if c.ClientID == "" {
		return "", errors.New("client_id is required")
	}

	endpoint := c.authorizeEndpoint()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	query := parsed.Query()
	query.Set("response_type", "code")
	query.Set("client_id", c.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", DefaultScope)
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", challengeMethod)
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func (c *Client) ExchangeCode(ctx context.Context, code string, redirectURI string, codeVerifier string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", c.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	response, err := c.doTokenRequest(ctx, form)
	if err != nil {
		return Token{}, err
	}

	return c.tokenFromResponse(response, ""), nil
}

func (c *Client) Refresh(ctx context.Context, current Token) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", c.ClientID)
	form.Set("refresh_token", current.RefreshToken)

	response, err := c.doTokenRequest(ctx, form)
	if err != nil {
		return Token{}, err
	}

	return c.tokenFromResponse(response, current.RefreshToken), nil
}

func (c *Client) doTokenRequest(ctx context.Context, form url.Values) (tokenResponse, error) {
	if c.ClientID == "" {
		return tokenResponse{}, errors.New("client_id is required")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return tokenResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return tokenResponse{}, oauthHTTPError(response)
	}

	var decoded tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return tokenResponse{}, err
	}
	if decoded.AccessToken == "" {
		return tokenResponse{}, errors.New("token response missing access_token")
	}

	return decoded, nil
}

func (c *Client) tokenFromResponse(response tokenResponse, fallbackRefreshToken string) Token {
	refreshToken := response.RefreshToken
	if refreshToken == "" {
		refreshToken = fallbackRefreshToken
	}

	var expiresAt time.Time
	if response.ExpiresIn > 0 {
		expiresAt = c.now().Add(time.Duration(response.ExpiresIn) * time.Second)
	}

	return Token{
		AccessToken:  response.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    response.TokenType,
		Scope:        response.Scope,
		ExpiresAt:    expiresAt,
	}
}

func oauthHTTPError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes+1))
	if len(body) > maxErrorBodyBytes {
		body = body[:maxErrorBodyBytes]
	}

	path := ""
	if response.Request != nil && response.Request.URL != nil {
		path = response.Request.URL.Path
	}

	return &HTTPError{
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Path:       path,
		Body:       strings.TrimSpace(string(body)),
	}
}

func (c *Client) authorizeEndpoint() string {
	if c.AuthorizeEndpoint != "" {
		return c.AuthorizeEndpoint
	}

	return AuthorizeEndpoint
}

func (c *Client) tokenEndpoint() string {
	if c.TokenEndpoint != "" {
		return c.TokenEndpoint
	}

	return TokenEndpoint
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return http.DefaultClient
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}

	return time.Now()
}
```

- [ ] **Step 4: Run token tests**

Run: `go test ./internal/xoauth`

Expected if Go is available: PASS.

Expected if Go is missing: failure containing `command not found: go`.

## Task 4: Token Store

**Files:**
- Create: `internal/tokenstore/tokenstore_test.go`
- Create: `internal/tokenstore/tokenstore.go`

- [ ] **Step 1: Write failing token store tests**

Create `internal/tokenstore/tokenstore_test.go`:

```go
package tokenstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

func TestSaveWritesJSONWith0600Permission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "token.json")
	expiresAt := time.Date(2026, 6, 7, 12, 34, 56, 0, time.UTC)

	token := xoauth.Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-123",
		TokenType:    "bearer",
		Scope:        "tweet.read users.read bookmark.read offline.access",
		ExpiresAt:    expiresAt,
	}

	if err := Save(path, token); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file permission = %o, want 600", got)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(parent) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permission = %o, want 700", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var saved map[string]string
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	assertSavedValue(t, saved, "access_token", "access-123")
	assertSavedValue(t, saved, "refresh_token", "refresh-123")
	assertSavedValue(t, saved, "token_type", "bearer")
	assertSavedValue(t, saved, "scope", "tweet.read users.read bookmark.read offline.access")
	assertSavedValue(t, saved, "expires_at", "2026-06-07T12:34:56Z")

	for _, forbidden := range []string{"client_id", "client_secret", "code_verifier", "state"} {
		if _, ok := saved[forbidden]; ok {
			t.Fatalf("saved JSON contains forbidden field %q", forbidden)
		}
	}
}

func TestDefaultPathUsesUserConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	want := filepath.Join(dir, "xapi-usecase", "token.json")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func assertSavedValue(t *testing.T, saved map[string]string, key string, want string) {
	t.Helper()

	if got := saved[key]; got != want {
		t.Fatalf("saved[%s] = %q, want %q", key, got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tokenstore`

Expected if Go is available: FAIL because `Save` and `DefaultPath` do not exist.

Expected if Go is missing: failure containing `command not found: go`.

- [ ] **Step 3: Implement token store**

Create `internal/tokenstore/tokenstore.go`:

```go
package tokenstore

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

const (
	dirPermission  = 0o700
	filePermission = 0o600
)

type storedToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresAt    string `json:"expires_at"`
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "xapi-usecase", "token.json"), nil
}

func Save(path string, token xoauth.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPermission); err != nil {
		return err
	}

	payload := storedToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Scope:        token.Scope,
		ExpiresAt:    token.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermission)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}

	return os.Chmod(path, filePermission)
}
```

- [ ] **Step 4: Run token store tests**

Run: `go test ./internal/tokenstore`

Expected if Go is available: PASS.

Expected if Go is missing: failure containing `command not found: go`.

## Task 5: X API Bearer Client And `/2/users/me`

**Files:**
- Create: `internal/xapi/client_test.go`
- Create: `internal/xapi/client.go`

- [ ] **Step 1: Write failing X API client tests**

Create `internal/xapi/client_test.go`:

```go
package xapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoAttachesBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/2/users/me" {
			t.Fatalf("path = %q, want /2/users/me", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-123" {
			t.Fatalf("Authorization = %q, want Bearer access-123", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &Client{AccessToken: "access-123", BaseURL: server.URL, HTTPClient: server.Client()}

	response, err := client.Do(context.Background(), http.MethodGet, "/2/users/me", nil)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("StatusCode = %d, want 204", response.StatusCode)
	}
}

func TestDoHTTPErrorIncludesStatusPathAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"title":"Forbidden"}`, http.StatusForbidden)
	}))
	defer server.Close()

	client := &Client{AccessToken: "access-123", BaseURL: server.URL, HTTPClient: server.Client()}

	_, err := client.Do(context.Background(), http.MethodGet, "/2/users/me", nil)
	if err == nil {
		t.Fatal("Do() error = nil, want error")
	}

	message := err.Error()
	for _, want := range []string{"403 Forbidden", "/2/users/me", `{"title":"Forbidden"}`} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want to contain %q", message, want)
		}
	}
}

func TestMeDecodesAuthenticatedUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/2/users/me" {
			t.Fatalf("path = %q, want /2/users/me", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"id":"2244994945","name":"X Dev","username":"TwitterDev"}}`)
	}))
	defer server.Close()

	client := &Client{AccessToken: "access-123", BaseURL: server.URL, HTTPClient: server.Client()}

	response, err := client.Me(context.Background())
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}

	if response.Data.ID != "2244994945" {
		t.Fatalf("Data.ID = %q, want 2244994945", response.Data.ID)
	}
	if response.Data.Name != "X Dev" {
		t.Fatalf("Data.Name = %q, want X Dev", response.Data.Name)
	}
	if response.Data.Username != "TwitterDev" {
		t.Fatalf("Data.Username = %q, want TwitterDev", response.Data.Username)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/xapi`

Expected if Go is available: FAIL because `Client`, `Do`, and `Me` do not exist.

Expected if Go is missing: failure containing `command not found: go`.

- [ ] **Step 3: Implement X API client**

Create `internal/xapi/client.go`:

```go
package xapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	BaseURL           = "https://api.x.com"
	maxErrorBodyBytes = 4 * 1024
)

type Client struct {
	AccessToken string
	BaseURL     string
	HTTPClient  *http.Client
}

type HTTPError struct {
	StatusCode int
	Status     string
	Path       string
	Body       string
}

type MeResponse struct {
	Data User `json:"data"`
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("x api request failed: %s %s", e.Status, e.Path)
	}

	return fmt.Sprintf("x api request failed: %s %s: %s", e.Status, e.Path, e.Body)
}

func (c *Client) Do(ctx context.Context, method string, path string, body io.Reader) (*http.Response, error) {
	requestURL := strings.TrimRight(c.baseURL(), "/") + "/" + strings.TrimLeft(path, "/")
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.AccessToken)

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		defer response.Body.Close()
		return nil, apiHTTPError(response)
	}

	return response, nil
}

func (c *Client) Me(ctx context.Context) (MeResponse, error) {
	response, err := c.Do(ctx, http.MethodGet, "/2/users/me", nil)
	if err != nil {
		return MeResponse{}, err
	}
	defer response.Body.Close()

	var decoded MeResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return MeResponse{}, err
	}

	return decoded, nil
}

func apiHTTPError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes+1))
	if len(body) > maxErrorBodyBytes {
		body = body[:maxErrorBodyBytes]
	}

	path := ""
	if response.Request != nil && response.Request.URL != nil {
		path = response.Request.URL.Path
	}

	return &HTTPError{
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Path:       path,
		Body:       strings.TrimSpace(string(body)),
	}
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}

	return BaseURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return http.DefaultClient
}
```

- [ ] **Step 4: Run X API client tests**

Run: `go test ./internal/xapi`

Expected if Go is available: PASS.

Expected if Go is missing: failure containing `command not found: go`.

## Task 6: Callback Handler

**Files:**
- Create: `internal/cli/callback_test.go`
- Create: `internal/cli/callback.go`

- [ ] **Step 1: Write failing callback tests**

Create `internal/cli/callback_test.go`:

```go
package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallbackHandlerAcceptsMatchingStateAndCode(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := newCallbackHandler("state-123", results)

	request := httptest.NewRequest(http.MethodGet, "/callback?state=state-123&code=code-123", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	result := <-results
	if result.Err != nil {
		t.Fatalf("result.Err = %v, want nil", result.Err)
	}
	if result.Code != "code-123" {
		t.Fatalf("result.Code = %q, want code-123", result.Code)
	}
}

func TestCallbackHandlerRejectsStateMismatch(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := newCallbackHandler("state-123", results)

	request := httptest.NewRequest(http.MethodGet, "/callback?state=wrong&code=code-123", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "state mismatch") {
		t.Fatalf("body = %q, want state mismatch", recorder.Body.String())
	}

	result := <-results
	if result.Err == nil {
		t.Fatal("result.Err = nil, want error")
	}
}

func TestCallbackHandlerRejectsOAuthError(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := newCallbackHandler("state-123", results)

	request := httptest.NewRequest(http.MethodGet, "/callback?state=state-123&error=access_denied&error_description=nope", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "access_denied") {
		t.Fatalf("body = %q, want access_denied", recorder.Body.String())
	}

	result := <-results
	if result.Err == nil {
		t.Fatal("result.Err = nil, want error")
	}
}

func TestCallbackHandlerRejectsMissingCode(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := newCallbackHandler("state-123", results)

	request := httptest.NewRequest(http.MethodGet, "/callback?state=state-123", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "missing code") {
		t.Fatalf("body = %q, want missing code", recorder.Body.String())
	}

	result := <-results
	if result.Err == nil {
		t.Fatal("result.Err = nil, want error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli`

Expected if Go is available: FAIL because `callbackResult` and `newCallbackHandler` do not exist.

Expected if Go is missing: failure containing `command not found: go`.

- [ ] **Step 3: Implement callback handler**

Create `internal/cli/callback.go`:

```go
package cli

import (
	"errors"
	"fmt"
	"net/http"
)

const maxCallbackErrorMessage = 512

type callbackResult struct {
	Code string
	Err  error
}

func newCallbackHandler(expectedState string, results chan<- callbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		if query.Get("state") != expectedState {
			sendCallbackResult(results, callbackResult{Err: errors.New("state mismatch")})
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		if query.Get("error") != "" || query.Get("error_description") != "" {
			message := callbackOAuthErrorMessage(query.Get("error"), query.Get("error_description"))
			sendCallbackResult(results, callbackResult{Err: errors.New(message)})
			http.Error(w, message, http.StatusBadRequest)
			return
		}

		code := query.Get("code")
		if code == "" {
			sendCallbackResult(results, callbackResult{Err: errors.New("missing code")})
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		sendCallbackResult(results, callbackResult{Code: code})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "Authentication complete. You can close this window.")
	})

	return mux
}

func sendCallbackResult(results chan<- callbackResult, result callbackResult) {
	select {
	case results <- result:
	default:
	}
}

func callbackOAuthErrorMessage(code string, description string) string {
	message := code
	if message == "" {
		message = "oauth error"
	}
	if description != "" {
		message += ": " + description
	}
	if len(message) > maxCallbackErrorMessage {
		return message[:maxCallbackErrorMessage]
	}

	return message
}
```

- [ ] **Step 4: Run callback tests**

Run: `go test ./internal/cli`

Expected if Go is available: PASS.

Expected if Go is missing: failure containing `command not found: go`.

## Task 7: CLI Entry Point And `auth login`

**Files:**
- Create: `cmd/xapi-usecase/main.go`
- Create: `internal/cli/cli.go`

- [ ] **Step 1: Implement process entry point**

Create `cmd/xapi-usecase/main.go`:

```go
package main

import (
	"context"
	"os"

	"github.com/y-writings/xapi-usecase/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}
```

- [ ] **Step 2: Implement CLI orchestration**

Create `internal/cli/cli.go`:

```go
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/y-writings/xapi-usecase/internal/tokenstore"
	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

const (
	clientIDEnv        = "XAPI_USECASE_CLIENT_ID"
	defaultCallbackIP  = "127.0.0.1"
	defaultCallbackPort = 8765
	defaultTimeout      = 5 * time.Minute
)

type getenvFunc func(string) string

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) int {
	if len(args) < 1 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "auth":
		return runAuth(ctx, args[1:], stdout, stderr, getenv)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runAuth(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "missing auth command")
		return 2
	}

	switch args[0] {
	case "login":
		return runAuthLogin(ctx, args[1:], stdout, stderr, getenv)
	default:
		fmt.Fprintf(stderr, "unknown auth command %q\n", args[0])
		return 2
	}
}

func runAuthLogin(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, getenv getenvFunc) int {
	flags := flag.NewFlagSet("auth login", flag.ContinueOnError)
	flags.SetOutput(stderr)

	clientID := flags.String("client-id", getenv(clientIDEnv), "X OAuth 2.0 client ID")
	tokenFile := flags.String("token-file", "", "token JSON file path")
	port := flags.Int("port", defaultCallbackPort, "local callback port")
	timeout := flags.Duration("timeout", defaultTimeout, "callback wait timeout")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *clientID == "" {
		fmt.Fprintf(stderr, "client ID is required; set %s or pass --client-id\n", clientIDEnv)
		return 2
	}
	if *port <= 0 || *port > 65535 {
		fmt.Fprintf(stderr, "invalid --port %d\n", *port)
		return 2
	}
	if *timeout <= 0 {
		fmt.Fprintln(stderr, "--timeout must be greater than zero")
		return 2
	}

	path := *tokenFile
	if path == "" {
		defaultPath, err := tokenstore.DefaultPath()
		if err != nil {
			fmt.Fprintf(stderr, "resolve token path: %v\n", err)
			return 1
		}
		path = defaultPath
	}

	if err := login(ctx, loginConfig{ClientID: *clientID, TokenFile: path, Port: *port, Timeout: *timeout}, stdout); err != nil {
		fmt.Fprintf(stderr, "auth login failed: %v\n", err)
		return 1
	}

	return 0
}

type loginConfig struct {
	ClientID  string
	TokenFile string
	Port      int
	Timeout   time.Duration
}

func login(ctx context.Context, config loginConfig, stdout io.Writer) error {
	redirectURI := callbackURL(config.Port)

	verifier, err := xoauth.GenerateCodeVerifier()
	if err != nil {
		return err
	}
	state, err := xoauth.GenerateState()
	if err != nil {
		return err
	}
	challenge := xoauth.CodeChallengeS256(verifier)

	oauthClient := xoauth.NewClient(config.ClientID)
	authURL, err := oauthClient.AuthURL(redirectURI, state, challenge)
	if err != nil {
		return err
	}

	results := make(chan callbackResult, 1)
	server := &http.Server{Handler: newCallbackHandler(state, results)}
	listener, err := net.Listen("tcp", net.JoinHostPort(defaultCallbackIP, strconv.Itoa(config.Port)))
	if err != nil {
		return err
	}
	defer server.Close()

	serverErrors := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	fmt.Fprintln(stdout, "Open this URL in your browser:")
	fmt.Fprintln(stdout, authURL)

	waitCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	select {
	case result := <-results:
		shutdownServer(server)
		if result.Err != nil {
			return result.Err
		}

		exchangeCtx, exchangeCancel := context.WithTimeout(ctx, 30*time.Second)
		defer exchangeCancel()
		token, err := oauthClient.ExchangeCode(exchangeCtx, result.Code, redirectURI, verifier)
		if err != nil {
			return err
		}
		if err := tokenstore.Save(config.TokenFile, token); err != nil {
			return err
		}

		fmt.Fprintf(stdout, "Token saved to %s\n", config.TokenFile)
		return nil
	case err := <-serverErrors:
		return err
	case <-waitCtx.Done():
		shutdownServer(server)
		return waitCtx.Err()
	}
}

func callbackURL(port int) string {
	return "http://" + net.JoinHostPort(defaultCallbackIP, strconv.Itoa(port)) + "/callback"
}

func shutdownServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: xapi-usecase auth login [--client-id CLIENT_ID] [--token-file PATH] [--port PORT] [--timeout DURATION]")
}
```

- [ ] **Step 3: Run all package tests**

Run: `go test ./...`

Expected if Go is available: PASS for all packages created so far.

Expected if Go is missing: failure containing `command not found: go`.

- [ ] **Step 4: Run CLI help smoke check**

Run: `go run ./cmd/xapi-usecase`

Expected if Go is available: exit code `2` and usage text containing `xapi-usecase auth login`.

Expected if Go is missing: failure containing `command not found: go`.

## Task 8: README Documentation

**Files:**
- Create: `README.md`

- [ ] **Step 1: Create README**

Create `README.md`:

```markdown
# xapi-usecase

Pre-release CLI experiments for X API v2 use cases.

## Requirements

- Go 1.26.3
- An approved X Developer account and App

This repository declares Go 1.26.3 in `.mise/config.toml` for users who manage tools with mise.

## X Developer Console Setup

In the X Developer Console for your App:

- Enable OAuth 2.0.
- Configure the App as a public/native client for this CLI flow.
- Register the callback URL exactly as `http://127.0.0.1:8765/callback` unless you plan to pass a different `--port`.
- Enable these scopes: `tweet.read`, `users.read`, `bookmark.read`, `offline.access`.
- Copy the OAuth 2.0 Client ID from the App keys and tokens screen.

The CLI does not use a client secret and never stores one.

## Login

Set your client ID:

```sh
export XAPI_USECASE_CLIENT_ID="your-client-id"
```

Run the local OAuth login flow:

```sh
go run ./cmd/xapi-usecase auth login
```

The command starts a temporary callback server on `127.0.0.1:8765`, prints an authorization URL, and waits for the browser callback. Open the printed URL in your browser, approve the requested scopes, and return to the CLI after the callback completes.

You can pass the client ID directly instead of using the environment variable:

```sh
go run ./cmd/xapi-usecase auth login --client-id "your-client-id"
```

## Options

- `--client-id`: OAuth 2.0 Client ID. Overrides `XAPI_USECASE_CLIENT_ID`.
- `--token-file`: Token JSON output path. Defaults to `os.UserConfigDir()/xapi-usecase/token.json`.
- `--port`: Local callback port. Defaults to `8765`.
- `--timeout`: Callback wait timeout. Defaults to `5m`.

If you change `--port`, register the matching callback URL in X Developer Console. X requires the callback URL to match exactly.

## Token File

The token file is JSON and is written with file permission `0600`. It contains `access_token`, `refresh_token`, `token_type`, `scope`, and `expires_at`.

The token file does not contain `client_id`, `client_secret`, `code_verifier`, or `state`.

## Bookmark Retrieval

Bookmark retrieval is not implemented yet. This pass only builds the OAuth PKCE login and API client foundation needed to call authenticated X API v2 endpoints.
```

- [ ] **Step 2: Verify README content manually**

Check that README includes:

- OAuth 2.0 enablement.
- Public/native client expectation.
- Exact callback URL.
- All required scopes.
- `client_id` via env var and flag.
- `auth login` command.
- Token path and permissions.
- No bookmark command examples.

## Task 9: Formatting And Full Verification

**Files:**
- All Go files created in previous tasks.
- `README.md`
- `go.mod`
- `.mise/config.toml`

- [ ] **Step 1: Format Go files**

Run: `gofmt -w cmd/xapi-usecase/main.go internal/cli/cli.go internal/cli/callback.go internal/cli/callback_test.go internal/xoauth/pkce.go internal/xoauth/client.go internal/xoauth/pkce_test.go internal/xoauth/client_test.go internal/tokenstore/tokenstore.go internal/tokenstore/tokenstore_test.go internal/xapi/client.go internal/xapi/client_test.go`

Expected if Go is available: command succeeds with no output.

Expected if Go is missing: failure containing `command not found: gofmt`.

- [ ] **Step 2: Run full test suite**

Run: `go test ./...`

Expected if Go is available: PASS for every package.

Expected if Go is missing: failure containing `command not found: go`.

- [ ] **Step 3: Inspect working tree**

Run: `git status --short`

Expected: only intentional files from this plan are listed.

- [ ] **Step 4: Inspect final diff**

Run: `git diff`

Expected: diff matches this plan and contains no secrets or generated token files.

## Self-Review

- Spec coverage: This plan covers module initialization, Go 1.26.3 configuration, PKCE verifier/challenge/state generation, auth URL generation, token exchange, refresh, bearer-token API requests, local `auth login`, callback state validation, token JSON saving with `0600`, README documentation, and final `go test ./...` verification.
- Non-goals preserved: No bookmark retrieval command, no client secret support, no OAuth 1.0a, no compatibility shims, no CLI framework, and no `golang.org/x/oauth2` dependency.
- Red-flag scan: No incomplete markers or vague edge-case instructions remain.
- Type consistency: `xoauth.Token`, `tokenstore.Save`, `xapi.Client`, `cli.Run`, and callback helper names are consistent across tests and implementation steps.
