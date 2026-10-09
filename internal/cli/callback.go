package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

const maxCallbackErrorMessageLength = 512

//go:embed callback.html
var callbackHTML string

var callbackPage = template.Must(template.New("callback").Parse(callbackHTML))

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
			writeCallbackPage(w, http.StatusBadRequest, err.Error())
			return
		}

		if message := callbackErrorMessage(
			query.Get("error"),
			query.Get("error_description"),
			expectedState,
			query.Get("code"),
		); message != "" {
			err := errors.New(message)
			sendCallbackResult(results, callbackResult{Err: err})
			writeCallbackPage(w, http.StatusBadRequest, message)
			return
		}

		code := query.Get("code")
		if code == "" {
			err := errors.New("missing code")
			sendCallbackResult(results, callbackResult{Err: err})
			writeCallbackPage(w, http.StatusBadRequest, err.Error())
			return
		}

		sendCallbackResult(results, callbackResult{Code: code})
		writeCallbackPage(w, http.StatusOK, "")
	})

	return mux
}

func writeCallbackPage(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	_ = callbackPage.Execute(w, message)
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
