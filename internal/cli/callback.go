package cli

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const maxCallbackErrorMessageLength = 512

type callbackResult struct {
	Code string
	Err  error
}

func newCallbackHandler(expectedState string, results chan<- callbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("state") != expectedState {
			err := errors.New("state mismatch")
			sendCallbackResult(results, callbackResult{Err: err})
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if message := callbackErrorMessage(query.Get("error"), query.Get("error_description"), expectedState, query.Get("code")); message != "" {
			err := errors.New(message)
			sendCallbackResult(results, callbackResult{Err: err})
			http.Error(w, message, http.StatusBadRequest)
			return
		}

		code := query.Get("code")
		if code == "" {
			err := errors.New("missing code")
			sendCallbackResult(results, callbackResult{Err: err})
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		sendCallbackResult(results, callbackResult{Code: code})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Authorization complete. You can close this window.\n"))
	})

	return mux
}

func sendCallbackResult(results chan<- callbackResult, result callbackResult) {
	select {
	case results <- result:
	default:
	}
}

func callbackErrorMessage(oauthError, description, expectedState, receivedCode string) string {
	if oauthError == "" && description == "" {
		return ""
	}

	var message string
	if oauthError != "" && description != "" {
		message = fmt.Sprintf("%s: %s", oauthError, description)
	} else if oauthError != "" {
		message = oauthError
	} else {
		message = description
	}
	if expectedState != "" {
		message = strings.ReplaceAll(message, expectedState, "[redacted]")
	}
	if receivedCode != "" {
		message = strings.ReplaceAll(message, receivedCode, "[redacted]")
	}

	if len(message) > maxCallbackErrorMessageLength {
		return message[:maxCallbackErrorMessageLength]
	}

	return message
}
