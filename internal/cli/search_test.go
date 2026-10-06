package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/y-writings/xapi-usecase/internal/xapi"
)

func TestSearchCollectsAndDeduplicatesExternalResources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2/tweets/search/recent" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "(domain knowledge) lang:ja" {
			t.Fatalf("query = %q", got)
		}
		if got := r.URL.Query().Get("post.fields"); got != "created_at,entities,note_post" {
			t.Fatalf("post.fields = %q", got)
		}
		writeResponse(t, w, `{"data":[`+
			`{"id":"1","text":"first","created_at":"2026-10-01T00:00:00Z",`+
			`"entities":{"urls":[{"url":"https://t.co/a",`+
			`"expanded_url":"https://example.com/article#part"},{"url":"https://t.co/a-duplicate",`+
			`"expanded_url":"https://example.com/article#part"},{"url":"https://t.co/x",`+
			`"expanded_url":"https://x.com/user/status/2"}]}},`+
			`{"id":"2","text":"second","created_at":"2026-10-02T00:00:00Z",`+
			`"entities":{"urls":[{"url":"https://t.co/b",`+
			`"expanded_url":"https://example.com/article"}]}}],"meta":{"result_count":2}}`)
	}))
	defer server.Close()
	old := newXAPIClient
	newXAPIClient = func(token string) *xapi.Client {
		return &xapi.Client{AccessToken: token, BaseURL: server.URL, HTTPClient: server.Client()}
	}
	t.Cleanup(func() { newXAPIClient = old })
	var stdout, stderr bytes.Buffer
	args := []string{
		"search", "--query", "domain knowledge", "--lang", "ja",
		"--limit", "2", "--bearer-token", "token",
	}
	code := Run(context.Background(), args, &stdout, &stderr, getenvNone)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var out searchOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.PostCount != 2 || out.ResourceCount != 1 || len(out.Resources[0].Sources) != 2 {
		t.Fatalf("output = %+v", out)
	}
	if out.Resources[0].Sources[0].PostID != "1" || out.Resources[0].Sources[1].PostID != "2" {
		t.Fatalf("sources = %+v, want one source each from posts 1 and 2", out.Resources[0].Sources)
	}
	if strings.Contains(out.Resources[0].URL, "#") {
		t.Fatalf("fragment not removed: %s", out.Resources[0].URL)
	}
}

func TestSearchCollectsLongFormPostURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("post.fields"); got != "created_at,entities,note_post" {
			t.Fatalf("post.fields = %q", got)
		}
		writeResponse(t, w, `{"data":[{"id":"long-1","text":"truncated text",`+
			`"created_at":"2026-10-01T00:00:00Z","entities":{"urls":[]},`+
			`"note_post":{"text":"complete long-form text with https://t.co/long",`+
			`"entities":{"urls":[{"url":"https://t.co/long",`+
			`"expanded_url":"https://example.com/long-form"}]}}}],`+
			`"meta":{"result_count":1}}`)
	}))
	defer server.Close()
	old := newXAPIClient
	newXAPIClient = func(token string) *xapi.Client {
		return &xapi.Client{AccessToken: token, BaseURL: server.URL, HTTPClient: server.Client()}
	}
	t.Cleanup(func() { newXAPIClient = old })
	var stdout, stderr bytes.Buffer
	args := []string{"search", "--query", "long form", "--limit", "1", "--bearer-token", "token"}
	if code := Run(context.Background(), args, &stdout, &stderr, getenvNone); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var out searchOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ResourceCount != 1 || out.Resources[0].URL != "https://example.com/long-form" {
		t.Fatalf("resources = %+v", out.Resources)
	}
	got := out.Resources[0].Sources[0]
	if got.PostID != "long-1" ||
		got.Text != "complete long-form text with https://t.co/long" ||
		got.ShortURL != "https://t.co/long" {
		t.Fatalf("source = %+v, want full long-form provenance", got)
	}
}

func TestSearchMarksHTTP200PartialErrorsIncomplete(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeResponse(t, w, `{"data":[{"id":"1","text":"usable result",`+
			`"entities":{"urls":[{"url":"https://t.co/a",`+
			`"expanded_url":"https://example.com/article"}]}}],`+
			`"errors":[{"title":"Partial response","detail":"some results unavailable"}],`+
			`"meta":{"result_count":1,"next_token":"next-page"}}`)
	}))
	defer server.Close()
	old := newXAPIClient
	newXAPIClient = func(token string) *xapi.Client {
		return &xapi.Client{AccessToken: token, BaseURL: server.URL, HTTPClient: server.Client()}
	}
	t.Cleanup(func() { newXAPIClient = old })
	var stdout, stderr bytes.Buffer
	args := []string{"search", "--query", "partial", "--limit", "2", "--bearer-token", "token"}
	if code := Run(context.Background(), args, &stdout, &stderr, getenvNone); code != 1 {
		t.Fatalf("code=%d stderr=%s, want non-zero for partial API data", code, stderr.String())
	}
	var out searchOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "incomplete" || out.IncompleteReason != "api_error" {
		t.Fatalf(
			"status = %q, reason = %q, want incomplete/api_error",
			out.Status,
			out.IncompleteReason,
		)
	}
	if out.PostCount != 1 || out.ResourceCount != 1 || out.Resources[0].Sources[0].PostID != "1" {
		t.Fatalf("partial output = %+v, want the usable data from the response", out)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want to stop after a partial response", requests)
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	var out, errout bytes.Buffer
	getenv := func(_ string) string { return "token" }
	code := Run(context.Background(), []string{"search"}, &out, &errout, getenv)
	if code != 2 || !strings.Contains(errout.String(), "--query is required") {
		t.Fatalf("code=%d stderr=%s", code, errout.String())
	}
}
