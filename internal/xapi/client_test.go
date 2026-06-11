package xapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestDoRejectsAbsoluteAndSchemeRelativePathsBeforeRequest(t *testing.T) {
	var safeServerRequests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&safeServerRequests, 1)
	}))
	defer server.Close()

	var httpRequests int32
	client := &Client{
		AccessToken: "access-123",
		BaseURL:     server.URL,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&httpRequests, 1)
			return nil, fmt.Errorf("unexpected HTTP request to %s", r.URL)
		})},
	}

	for _, path := range []string{"https://example.test/2/users/me", "//example.test/2/users/me"} {
		if _, err := client.Do(context.Background(), http.MethodGet, path, nil); err == nil {
			t.Fatalf("Do(%q) error = nil, want rejection", path)
		}
	}

	if got := atomic.LoadInt32(&httpRequests); got != 0 {
		t.Fatalf("HTTP requests = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&safeServerRequests); got != 0 {
		t.Fatalf("safe server requests = %d, want 0", got)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
