package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/y-writings/xapi-usecase/internal/output"
	"github.com/y-writings/xapi-usecase/internal/tokenstore"
	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

func TestSearchUsesSavedLoginTokenWithoutClientID(t *testing.T) {
	fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := tokenstore.Save(tokenFile, xoauth.Token{
		AccessToken: "saved-access",
		ExpiresAt:   fixedNow.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	originalPath := defaultTokenPath
	defaultTokenPath = func() (string, error) { return tokenFile, nil }
	t.Cleanup(func() { defaultTokenPath = originalPath })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2/tweets/search/recent" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer saved-access" {
			t.Errorf("Authorization = %q, want saved login token", got)
		}
		writeResponse(t, w, `{"data":[],"meta":{"result_count":0}}`)
	}))
	defer server.Close()
	useTestClients(t, server, fixedNow)

	var stdout, stderr bytes.Buffer
	args := []string{"search", "--query", "test", "--limit", "1"}
	if code := Run(context.Background(), args, &stdout, &stderr, getenvNone); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	validateSearchJSON(t, stdout.Bytes())
	var out output.SearchOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "complete" || out.PostCount != 0 || out.Resources == nil {
		t.Fatalf("output = %+v, want complete empty search", out)
	}
}

func TestSearchExplicitBearerDoesNotReadOrRefreshSavedToken(t *testing.T) {
	cases := []struct {
		name, flagToken, wantToken string
	}{
		{"environment", "", "env-token"},
		{"flag overrides environment", "flag-token", "flag-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			originalPath := defaultTokenPath
			defaultTokenPath = func() (string, error) {
				t.Error("explicit bearer must not resolve the saved token path")
				return "", fmt.Errorf("unexpected saved token access")
			}
			t.Cleanup(func() { defaultTokenPath = originalPath })

			var mu sync.Mutex
			requests := 0
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				requests++
				if r.URL.Path != "/2/tweets/search/recent" {
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer "+tc.wantToken {
					t.Errorf("Authorization = %q, want explicit bearer %q", got, tc.wantToken)
				}
				http.Error(w, `{"title":"Unauthorized"}`, http.StatusUnauthorized)
			})
			server := httptest.NewServer(handler)
			defer server.Close()
			useTestClients(t, server, time.Now())

			args := []string{"search", "--query", "test", "--limit", "1"}
			if tc.flagToken != "" {
				args = append(args, "--bearer-token", tc.flagToken,
					"--token-file", filepath.Join(t.TempDir(), "missing.json"))
			}
			getenv := func(key string) string {
				if key == bearerTokenEnv {
					return "env-token"
				}
				return ""
			}
			var stdout, stderr bytes.Buffer
			if code := Run(context.Background(), args, &stdout, &stderr, getenv); code != 1 {
				t.Fatalf("code = %d, stderr = %s, want API failure", code, stderr.String())
			}
			var out output.SearchOutput
			if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Status != "incomplete" ||
				out.IncompleteReason != "authentication_or_access_denied" {
				t.Fatalf("output = %+v, want explicit bearer authentication failure", out)
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 1 {
				t.Fatalf("requests = %d, want one request without refresh or fallback", requests)
			}
		})
	}
}

func TestSearchRefreshesOnlyFailedPageAndPreservesResults(t *testing.T) {
	cases := []struct {
		name, reason, stderr string
	}{
		{"recovery", "", ""},
		{"retry unauthorized", "authentication_or_access_denied", "401 Unauthorized"},
		{"refresh rejected", "request_failed", "503 Service Unavailable"},
		{"save failed", "request_failed", "rename"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixedNow := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
			tokenFile := filepath.Join(t.TempDir(), "token.json")
			if err := tokenstore.Save(tokenFile, xoauth.Token{
				AccessToken: "old-access", RefreshToken: "old-refresh",
				ExpiresAt: fixedNow.Add(time.Hour),
			}); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var cursors []string
			refreshes := 0
			pageTwoAttempts, pageThreeAttempts := 0, 0
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.URL.Path == "/2/oauth2/token" {
					refreshes++
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					wantRefresh := "old-refresh"
					if refreshes == 2 {
						wantRefresh = "refresh-1"
					}
					wantClientID := "client-123"
					if tc.name == "recovery" {
						wantClientID = "flag-client"
					}
					if r.Form.Get("refresh_token") != wantRefresh ||
						r.Form.Get("client_id") != wantClientID {
						t.Errorf("refresh form = %v, want token %q and client %q",
							r.Form, wantRefresh, wantClientID)
					}
					if tc.name == "refresh rejected" {
						http.Error(w, "refresh unavailable", http.StatusServiceUnavailable)
						return
					}
					if tc.name == "save failed" {
						if err := os.Remove(tokenFile); err != nil {
							t.Error(err)
						}
						if err := os.Mkdir(tokenFile, 0o700); err != nil {
							t.Error(err)
						}
					}
					writeResponse(t, w, fmt.Sprintf(
						`{"access_token":"access-%d","refresh_token":"refresh-%d",`+
							`"expires_in":3600}`,
						refreshes, refreshes,
					))
					return
				}
				if r.URL.Path != "/2/tweets/search/recent" {
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				cursor := r.URL.Query().Get("next_token")
				cursors = append(cursors, cursor)
				wantAccess := "old-access"
				if refreshes > 0 {
					wantAccess = fmt.Sprintf("access-%d", refreshes)
					saved, err := tokenstore.Load(tokenFile)
					if err != nil || saved.AccessToken != wantAccess {
						t.Errorf("refreshed access token must be saved before retry: %v", err)
					}
				}
				if got := r.Header.Get("Authorization"); got != "Bearer "+wantAccess {
					t.Errorf("Authorization = %q, want current access token %q", got, wantAccess)
				}
				id, next := "1", "page-two"
				switch cursor {
				case "":
				case "page-two":
					pageTwoAttempts++
					if pageTwoAttempts == 1 || tc.name == "retry unauthorized" {
						http.Error(w, `{"title":"Unauthorized"}`, http.StatusUnauthorized)
						return
					}
					id, next = "2", "page-three"
				case "page-three":
					pageThreeAttempts++
					if pageThreeAttempts == 1 {
						http.Error(w, `{"title":"Unauthorized"}`, http.StatusUnauthorized)
						return
					}
					id, next = "3", ""
				default:
					t.Errorf("unexpected cursor: %q", cursor)
				}
				writeResponse(t, w, fmt.Sprintf(`{"data":[{"id":%q,"entities":{"urls":[`+
					`{"url":"https://t.co/link","expanded_url":"https://example.com/%s"}]}}],`+
					`"meta":{"next_token":%q}}`, id, id, next))
			})
			server := httptest.NewServer(handler)
			defer server.Close()
			useTestClients(t, server, fixedNow)

			args := []string{"search", "--query", "test", "--limit", "3", "--token-file", tokenFile}
			if tc.name == "recovery" {
				args = append(args, "--client-id", "flag-client")
			}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), args, &stdout, &stderr, clientIDEnvGetter)
			wantCode, wantCount, wantStatus := 0, 3, "complete"
			wantCursors := []string{"", "page-two", "page-two", "page-three", "page-three"}
			wantRefreshes := 2
			if tc.reason != "" {
				wantCode, wantCount, wantStatus = 1, 1, "incomplete"
				wantCursors, wantRefreshes = []string{"", "page-two"}, 1
				if tc.name == "retry unauthorized" {
					wantCursors = append(wantCursors, "page-two")
				}
			}
			if code != wantCode || !strings.Contains(stderr.String(), tc.stderr) {
				t.Fatalf("code = %d, stderr = %s, want code %d and %q",
					code, stderr.String(), wantCode, tc.stderr)
			}
			validateSearchJSON(t, stdout.Bytes())
			var out output.SearchOutput
			if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Status != wantStatus || out.IncompleteReason != tc.reason ||
				out.PostCount != wantCount || out.ResourceCount != wantCount ||
				len(out.Resources) != wantCount {
				t.Fatalf("output = %+v, want %s/%s with %d results",
					out, wantStatus, tc.reason, wantCount)
			}
			for i, resource := range out.Resources {
				id := fmt.Sprint(i + 1)
				if resource.URL != "https://example.com/"+id || len(resource.Sources) != 1 ||
					resource.Sources[0].PostID != id {
					t.Errorf("resource %d = %+v, want retained post %s exactly once",
						i, resource, id)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if !slices.Equal(cursors, wantCursors) || refreshes != wantRefreshes {
				t.Fatalf("cursors = %v, refreshes = %d, want %v and %d",
					cursors, refreshes, wantCursors, wantRefreshes)
			}
			if tc.name == "recovery" {
				saved, err := tokenstore.Load(tokenFile)
				if err != nil || saved.RefreshToken != "refresh-2" {
					t.Fatalf("latest rotated refresh token not saved: %v", err)
				}
			}
			if tc.name == "refresh rejected" {
				saved, err := tokenstore.Load(tokenFile)
				if err != nil || saved.AccessToken != "old-access" ||
					saved.RefreshToken != "old-refresh" {
					t.Fatalf("failed refresh must leave the saved credentials unchanged: %v", err)
				}
			}
		})
	}
}
