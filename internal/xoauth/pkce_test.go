package xoauth

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestCodeChallengeS256UsesRFC7636Example(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	if got := CodeChallengeS256(verifier); got != want {
		t.Fatalf("CodeChallengeS256() = %q, want %q", got, want)
	}
}

func TestGenerateCodeVerifierReturnsPKCECompatibleValue(t *testing.T) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier() error = %v", err)
	}

	assertRandomTokenShape(t, verifier)
}

func TestGenerateStateReturnsRandomURLSafeValue(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}

	assertRandomTokenShape(t, state)
}

func TestAuthURLContainsRequiredQuery(t *testing.T) {
	client := NewClient("client-123")

	got, err := client.AuthURL("http://127.0.0.1:8765/callback", "state-123", "challenge-123")
	if err != nil {
		t.Fatalf("AuthURL() error = %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	if parsed.Scheme != "https" || parsed.Host != "x.com" || parsed.Path != "/i/oauth2/authorize" {
		t.Fatalf(
			"AuthURL endpoint = %s://%s%s, want https://x.com/i/oauth2/authorize",
			parsed.Scheme,
			parsed.Host,
			parsed.Path,
		)
	}

	query := parsed.Query()
	assertQueryValue(t, query, "response_type", "code")
	assertQueryValue(t, query, "client_id", "client-123")
	assertQueryValue(t, query, "redirect_uri", "http://127.0.0.1:8765/callback")
	assertQueryValue(t, query, "scope", DefaultScope)
	assertQueryValue(t, query, "state", "state-123")
	assertQueryValue(t, query, "code_challenge", "challenge-123")
	assertQueryValue(t, query, "code_challenge_method", "S256")
}

func TestAuthURLRequiresClientID(t *testing.T) {
	client := NewClient("")

	_, err := client.AuthURL("http://127.0.0.1:8765/callback", "state", "challenge")
	if err == nil {
		t.Fatal("AuthURL() error = nil, want error")
	}
}

func TestAuthURLRequiresRedirectURIStateAndCodeChallenge(t *testing.T) {
	client := NewClient("client-123")

	tests := []struct {
		name          string
		redirectURI   string
		state         string
		codeChallenge string
		wantError     string
	}{
		{
			name:          "missing redirect URI",
			redirectURI:   "",
			state:         "state",
			codeChallenge: "challenge",
			wantError:     "redirect URI is required",
		},
		{
			name:          "missing state",
			redirectURI:   "http://127.0.0.1:8765/callback",
			state:         "",
			codeChallenge: "challenge",
			wantError:     "state is required",
		},
		{
			name:          "missing code challenge",
			redirectURI:   "http://127.0.0.1:8765/callback",
			state:         "state",
			codeChallenge: "",
			wantError:     "code challenge is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.AuthURL(test.redirectURI, test.state, test.codeChallenge)
			if err == nil {
				t.Fatal("AuthURL() error = nil, want error")
			}

			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("AuthURL() error = %q, want %q", err.Error(), test.wantError)
			}
		})
	}
}

func assertRandomTokenShape(t *testing.T, value string) {
	t.Helper()

	if len(value) != 43 {
		t.Fatalf("len(value) = %d, want 43", len(value))
	}

	if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(value) {
		t.Fatalf("value = %q, want raw URL-safe base64", value)
	}
}

func assertQueryValue(t *testing.T, query url.Values, key string, want string) {
	t.Helper()

	if got := query.Get(key); got != want {
		t.Fatalf("query[%s] = %q, want %q", key, got, want)
	}
}
