//nolint:lll // Keeping CLI arguments and JSON fixtures on one line makes failures readable.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/y-writings/xapi-usecase/internal/output"
	"github.com/y-writings/xapi-usecase/internal/xapi"
)

func TestUserPostsResolvesUserPagesAndKeepsAllPostText(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/2/users/by/username/example":
			if r.URL.Query().Get("user.fields") != "name,username" {
				t.Fatalf("user.fields = %q", r.URL.Query().Get("user.fields"))
			}
			writeResponse(t, w, `{"data":{"id":"42","name":"Example User","username":"example"}}`)
		case "/2/users/42/tweets":
			q := r.URL.Query()
			if q.Get("start_time") != "2026-09-01T00:00:00Z" || q.Get("end_time") != "2026-10-01T00:00:00Z" {
				t.Fatalf("time query = %v", q)
			}
			if q.Get("exclude") != "replies,retweets" || q.Get("tweet.fields") != "created_at,note_tweet" {
				t.Fatalf("post query = %v", q)
			}
			if q.Get("pagination_token") == "" {
				writeResponse(t, w, `{"data":[{"id":"1","text":"no links","created_at":"2026-09-03T00:00:00Z"}],"meta":{"next_token":"page-2"}}`)
				return
			}
			writeResponse(t, w, `{"data":[{"id":"2","text":"short","created_at":"2026-09-02T00:00:00Z","note_tweet":{"text":"complete long post"}}],"meta":{}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	useUserPostsTestClient(t, server)

	var stdout, stderr bytes.Buffer
	args := []string{"user-posts", "--username", "@example", "--start-time", "2026-09-01T00:00:00Z", "--end-time", "2026-10-01T00:00:00Z", "--limit", "3", "--exclude-replies", "--exclude-reposts", "--bearer-token", "token"}
	if code := Run(context.Background(), args, &stdout, &stderr, getenvNone); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	validateUserPostsJSON(t, stdout.Bytes())
	var got output.UserPostsOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if requests != 3 || got.Status != "complete" || got.PostCount != 2 || len(got.Posts) != 2 {
		t.Fatalf("requests = %d, output = %+v", requests, got)
	}
	if got.Posts[0].Text != "no links" || got.Posts[1].Text != "complete long post" || got.Posts[1].URL != "https://x.com/example/status/2" {
		t.Fatalf("posts = %+v", got.Posts)
	}
}

func TestUserPostsMarksLimitAndPartialFailure(t *testing.T) {
	for _, tc := range []struct {
		name, secondResponse, status, reason string
		wantCode                             int
	}{
		{"limit", `{"data":[{"id":"2","text":"two"}],"meta":{"next_token":"more"}}`, "limit_reached", "", 0},
		{"partial error", `{"data":[{"id":"2","text":"two"}],"errors":[{"title":"partial"}],"meta":{"next_token":"more"}}`, "incomplete", "api_error", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/2/users/by/username/example":
					writeResponse(t, w, `{"data":{"id":"42","username":"example"}}`)
				case "/2/users/42/tweets":
					if r.URL.Query().Get("pagination_token") == "" {
						writeResponse(t, w, `{"data":[{"id":"1","text":"one"}],"meta":{"next_token":"next"}}`)
					} else {
						writeResponse(t, w, tc.secondResponse)
					}
				}
			}))
			defer server.Close()
			useUserPostsTestClient(t, server)
			var stdout, stderr bytes.Buffer
			args := []string{"user-posts", "--username", "example", "--start-time", "2026-09-01T00:00:00Z", "--end-time", "2026-10-01T00:00:00Z", "--limit", "2", "--bearer-token", "token"}
			if code := Run(context.Background(), args, &stdout, &stderr, getenvNone); code != tc.wantCode {
				t.Fatalf("code = %d, stderr = %s", code, stderr.String())
			}
			validateUserPostsJSON(t, stdout.Bytes())
			var got output.UserPostsOutput
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.status || got.IncompleteReason != tc.reason || got.PostCount != 2 {
				t.Fatalf("output = %+v", got)
			}
		})
	}
}

func TestUserPostsClassifiesLookupFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"title":"Too Many Requests"}`, http.StatusTooManyRequests)
	}))
	defer server.Close()
	useUserPostsTestClient(t, server)
	var stdout, stderr bytes.Buffer
	args := []string{"user-posts", "--username", "example", "--start-time", "2026-09-01T00:00:00Z", "--end-time", "2026-10-01T00:00:00Z", "--bearer-token", "token"}
	if code := Run(context.Background(), args, &stdout, &stderr, getenvNone); code != 1 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	var got output.UserPostsOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "incomplete" || got.IncompleteReason != "rate_limited" || got.Posts == nil {
		t.Fatalf("output = %+v", got)
	}
}

func TestUserPostsValidatesTimeRange(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"user-posts", "--username", "example", "--start-time", "2026-10-01T00:00:00Z", "--end-time", "2026-09-01T00:00:00Z"}
	if code := Run(context.Background(), args, &stdout, &stderr, getenvNone); code != 2 || !strings.Contains(stderr.String(), "must be before") {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
}

func useUserPostsTestClient(t *testing.T, server *httptest.Server) {
	t.Helper()
	old := newXAPIClient
	newXAPIClient = func(token string) *xapi.Client {
		return &xapi.Client{AccessToken: token, BaseURL: server.URL, HTTPClient: server.Client()}
	}
	t.Cleanup(func() { newXAPIClient = old })
}
