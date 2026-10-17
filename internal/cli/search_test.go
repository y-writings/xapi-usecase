//nolint:lll // JSON response fixtures are more readable as complete lines.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/y-writings/xapi-usecase/internal/output"
	"github.com/y-writings/xapi-usecase/internal/xapi"
)

func installSearchServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	old := newXAPIClient
	newXAPIClient = func(token string) *xapi.Client {
		return &xapi.Client{AccessToken: token, BaseURL: server.URL, HTTPClient: server.Client()}
	}
	t.Cleanup(func() { newXAPIClient = old })
}

func runSearch(t *testing.T, args ...string) (int, []byte, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"search"}, args...), &stdout, &stderr, getenvNone)
	return code, stdout.Bytes(), stderr.String()
}

func TestSearchCollectsPostsWithAndWithoutLinksAndAuthors(t *testing.T) {
	installSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("query"); got != "(AI tools) lang:ja" {
			t.Fatalf("query = %q", got)
		}
		if got := r.URL.Query().Get("post.fields"); got != "created_at,entities,note_post" {
			t.Fatalf("post.fields = %q", got)
		}
		if got := r.URL.Query().Get("expansions"); got != "author_id" {
			t.Fatalf("expansions = %q", got)
		}
		if got := r.URL.Query().Get("user.fields"); got != "name,username" {
			t.Fatalf("user.fields = %q", got)
		}
		writeResponse(t, w, `{"data":[`+
			`{"id":"1","author_id":"42","text":"linked","created_at":"2026-10-16T00:00:00Z","entities":{"urls":[{"url":"https://t.co/a","expanded_url":"https://example.com/a","unwound_url":"https://example.com/final"}]}},`+
			`{"id":"2","author_id":"42","text":"no link","created_at":"2026-10-16T01:00:00Z","entities":{"urls":[]}}],`+
			`"includes":{"users":[{"id":"42","name":"Alice","username":"alice"}]},"meta":{"result_count":2}}`)
	})
	code, data, stderr := runSearch(t, "--query", "AI tools", "--lang", "ja", "--limit", "10", "--bearer-token", "token")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	validateSearchJSON(t, data)
	var out output.SearchOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "complete" || out.PostCount != 2 || len(out.Posts) != 2 {
		t.Fatalf("output = %+v", out)
	}
	if out.Posts[0].Author.Username != "alice" || out.Posts[0].URL != "https://x.com/alice/status/1" {
		t.Fatalf("post = %+v", out.Posts[0])
	}
	if len(out.Posts[0].ExternalURLs) != 1 || out.Posts[0].ExternalURLs[0].ExpandedURL != "https://example.com/final" || out.Posts[0].ExternalURLs[0].ShortURL != "https://t.co/a" {
		t.Fatalf("external_urls = %+v", out.Posts[0].ExternalURLs)
	}
	if out.Posts[1].ExternalURLs == nil || len(out.Posts[1].ExternalURLs) != 0 {
		t.Fatalf("linkless external_urls = %#v", out.Posts[1].ExternalURLs)
	}
}

func TestSearchDeduplicatesPostsAcrossPagesAndMarksLimit(t *testing.T) {
	requests := 0
	installSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			writeResponse(t, w, `{"data":[{"id":"1","text":"one","entities":{"urls":[]}}],"meta":{"result_count":1,"next_token":"next"}}`)
			return
		}
		if r.URL.Query().Get("next_token") != "next" {
			t.Fatalf("next_token = %q", r.URL.Query().Get("next_token"))
		}
		writeResponse(t, w, `{"data":[{"id":"1","text":"duplicate","entities":{"urls":[]}},{"id":"2","text":"two","entities":{"urls":[]}}],"meta":{"result_count":2,"next_token":"more"}}`)
	})
	code, data, stderr := runSearch(t, "--query", "x", "--limit", "2", "--bearer-token", "token")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var out output.SearchOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "limit_reached" || out.PostCount != 2 || len(out.Posts) != 2 || out.Posts[1].ID != "2" {
		t.Fatalf("output = %+v", out)
	}
}

func TestSearchUsesLongFormTextAndURLs(t *testing.T) {
	installSearchServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResponse(t, w, `{"data":[{"id":"long","text":"truncated","entities":{"urls":[]},"note_post":{"text":"complete","entities":{"urls":[{"url":"https://t.co/l","expanded_url":"https://example.com/long"}]}}}],"meta":{"result_count":1}}`)
	})
	code, data, stderr := runSearch(t, "--query", "long", "--limit", "1", "--bearer-token", "token")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var out output.SearchOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Posts[0].Text != "complete" || out.Posts[0].ExternalURLs[0].ExpandedURL != "https://example.com/long" {
		t.Fatalf("post = %+v", out.Posts[0])
	}
}

func TestSearchPartialErrorKeepsUsablePosts(t *testing.T) {
	installSearchServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResponse(t, w, `{"data":[{"id":"1","text":"usable","entities":{"urls":[]}}],"errors":[{"title":"Partial","detail":"missing"}],"meta":{"result_count":1}}`)
	})
	code, data, _ := runSearch(t, "--query", "partial", "--limit", "2", "--bearer-token", "token")
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
	validateSearchJSON(t, data)
	var out output.SearchOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "incomplete" || out.IncompleteReason != "api_error" || out.PostCount != 1 {
		t.Fatalf("output = %+v", out)
	}
}

func TestSearchOutputFileContainsOnlyJSON(t *testing.T) {
	installSearchServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResponse(t, w, `{"data":[],"meta":{"result_count":0}}`)
	})
	path := filepath.Join(t.TempDir(), "search.json")
	code, stdout, stderr := runSearch(t, "--query", "none", "--limit", "1", "--bearer-token", "token", "--output", path)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	validateSearchJSON(t, data)
	if !strings.HasPrefix(string(stdout), "Saved 0 posts to ") || strings.Contains(string(stdout), `"status"`) {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestSearchRejectsInvalidConditions(t *testing.T) {
	for _, args := range [][]string{{}, {"--query", "x", "--start-time", "2026-10-17T00:00:00Z", "--end-time", "2026-10-16T00:00:00Z"}} {
		code, _, stderr := runSearch(t, args...)
		if code != 2 || stderr == "" {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr)
		}
	}
}
