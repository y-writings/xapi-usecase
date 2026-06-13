package xoauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	exchangedTokenJSON = `{"access_token":"access-123","refresh_token":"refresh-123",` +
		`"token_type":"bearer",` +
		`"scope":"tweet.read users.read bookmark.read offline.access",` +
		`"expires_in":7200}`

	refreshedTokenWithoutRotationJSON = `{"access_token":"new-access",` +
		`"token_type":"bearer",` +
		`"scope":"tweet.read users.read bookmark.read offline.access",` +
		`"expires_in":3600}`

	refreshedTokenJSON = `{"access_token":"new-access","refresh_token":"new-refresh",` +
		`"token_type":"bearer",` +
		`"scope":"tweet.read users.read bookmark.read offline.access",` +
		`"expires_in":3600}`
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

		assertFormBody(t, r, map[string]string{
			"grant_type":    "authorization_code",
			"client_id":     "client-123",
			"code":          "code-123",
			"redirect_uri":  "http://127.0.0.1:8765/callback",
			"code_verifier": "verifier-123",
		})

		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, exchangedTokenJSON)
	}))
	defer server.Close()

	client := &Client{
		ClientID:      "client-123",
		HTTPClient:    server.Client(),
		TokenEndpoint: server.URL,
		Now:           func() time.Time { return fixedNow },
	}

	token, err := client.ExchangeCode(
		context.Background(),
		"code-123",
		"http://127.0.0.1:8765/callback",
		"verifier-123",
	)
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

		assertFormBody(t, r, map[string]string{
			"grant_type":    "refresh_token",
			"client_id":     "client-123",
			"refresh_token": "old-refresh",
		})

		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, refreshedTokenWithoutRotationJSON)
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
		writeResponse(t, w, refreshedTokenJSON)
	}))
	defer server.Close()

	client := &Client{
		ClientID:      "client-123",
		HTTPClient:    server.Client(),
		TokenEndpoint: server.URL,
	}

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

	_, err := client.ExchangeCode(
		context.Background(),
		"code-123",
		"http://127.0.0.1:8765/callback",
		"verifier-123",
	)
	if err == nil {
		t.Fatal("ExchangeCode() error = nil, want error")
	}

	message := err.Error()
	for _, want := range []string{
		"400 Bad Request",
		"/2/oauth2/token",
		`{"error":"invalid_request"}`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want to contain %q", message, want)
		}
	}
}

func writeResponse(t *testing.T, w io.Writer, body string) {
	t.Helper()

	if _, err := fmt.Fprint(w, body); err != nil {
		t.Fatalf("Fprint(response) error = %v", err)
	}
}

func assertFormBody(t *testing.T, r *http.Request, want map[string]string) {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("ReadAll(body) error = %v", err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("ParseQuery(body) error = %v", err)
	}
	if len(values) != len(want) {
		t.Fatalf("form key count = %d, want %d; form = %v", len(values), len(want), values)
	}
	for key, wantValue := range want {
		gotValues, ok := values[key]
		if !ok {
			t.Fatalf("form missing key %q; form = %v", key, values)
		}
		if len(gotValues) != 1 {
			t.Fatalf("form[%s] values = %v, want exactly one value", key, gotValues)
		}
		if gotValues[0] != wantValue {
			t.Fatalf("form[%s] = %q, want %q", key, gotValues[0], wantValue)
		}
	}
}
