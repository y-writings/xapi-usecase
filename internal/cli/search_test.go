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
		writeResponse(t, w, `{"data":[`+
			`{"id":"1","text":"first","created_at":"2026-10-01T00:00:00Z",`+
			`"entities":{"urls":[{"url":"https://t.co/a",`+
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
	if strings.Contains(out.Resources[0].URL, "#") {
		t.Fatalf("fragment not removed: %s", out.Resources[0].URL)
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
