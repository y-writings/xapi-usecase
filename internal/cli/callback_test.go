package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const callbackTestTimeout = 100 * time.Millisecond

func TestCallbackHandlerAcceptsMatchingStateAndCode(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := newCallbackHandler("state-123", results)

	request := httptest.NewRequest(http.MethodGet, "/callback?state=state-123&code=code-123", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	result := receiveCallbackResult(t, results)
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

	result := receiveCallbackResult(t, results)
	if result.Err == nil {
		t.Fatal("result.Err = nil, want error")
	}
}

func TestCallbackHandlerRejectsOAuthError(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := newCallbackHandler("state-123", results)

	request := httptest.NewRequest(
		http.MethodGet,
		"/callback?state=state-123&error=access_denied&error_description=nope",
		nil,
	)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "access_denied") {
		t.Fatalf("body = %q, want access_denied", body)
	}
	if !strings.Contains(body, "nope") {
		t.Fatalf("body = %q, want nope", body)
	}

	result := receiveCallbackResult(t, results)
	if result.Err == nil {
		t.Fatal("result.Err = nil, want error")
	}
	if !strings.Contains(result.Err.Error(), "access_denied") {
		t.Fatalf("result.Err = %q, want access_denied", result.Err.Error())
	}
	if !strings.Contains(result.Err.Error(), "nope") {
		t.Fatalf("result.Err = %q, want nope", result.Err.Error())
	}
}

func TestCallbackHandlerRedactsStateAndCodeFromOAuthError(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := newCallbackHandler("state-123", results)

	request := httptest.NewRequest(
		http.MethodGet,
		"/callback?state=state-123&code=code-123&"+
			"error=access_denied&"+
			"error_description=before-state-123-middle-code-123-after",
		nil,
	)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "state-123") {
		t.Fatalf("body = %q, want state redacted", body)
	}
	if strings.Contains(body, "code-123") {
		t.Fatalf("body = %q, want code redacted", body)
	}
	if !strings.Contains(body, "access_denied") {
		t.Fatalf("body = %q, want access_denied", body)
	}
	for _, want := range []string{"before", "middle", "after"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %s", body, want)
		}
	}

	result := receiveCallbackResult(t, results)
	if result.Err == nil {
		t.Fatal("result.Err = nil, want error")
	}
	errText := result.Err.Error()
	if strings.Contains(errText, "state-123") {
		t.Fatalf("result.Err = %q, want state redacted", errText)
	}
	if strings.Contains(errText, "code-123") {
		t.Fatalf("result.Err = %q, want code redacted", errText)
	}
	if !strings.Contains(errText, "access_denied") {
		t.Fatalf("result.Err = %q, want access_denied", errText)
	}
	for _, want := range []string{"before", "middle", "after"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("result.Err = %q, want %s", errText, want)
		}
	}
}

func TestCallbackHandlerDoesNotBlockWhenResultsChannelIsFull(t *testing.T) {
	results := make(chan callbackResult, 1)
	results <- callbackResult{Code: "already full"}
	handler := newCallbackHandler("state-123", results)

	request := httptest.NewRequest(http.MethodGet, "/callback?state=state-123&code=code-123", nil)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		handler.ServeHTTP(recorder, request)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(callbackTestTimeout):
		t.Fatal("handler blocked with full results channel")
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
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

	result := receiveCallbackResult(t, results)
	if result.Err == nil {
		t.Fatal("result.Err = nil, want error")
	}
}

func receiveCallbackResult(t *testing.T, results <-chan callbackResult) callbackResult {
	t.Helper()

	select {
	case result := <-results:
		return result
	case <-time.After(callbackTestTimeout):
		t.Fatal("timed out waiting for callback result")
	}

	return callbackResult{}
}
