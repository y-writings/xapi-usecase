package tokenstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

func TestSaveWritesJSONWith0600Permission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "token.json")
	expiresAt := time.Date(2026, 6, 7, 12, 34, 56, 0, time.UTC)

	token := xoauth.Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-123",
		TokenType:    "bearer",
		Scope:        "tweet.read users.read bookmark.read offline.access",
		ExpiresAt:    expiresAt,
	}

	if err := Save(path, token); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file permission = %o, want 600", got)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(parent) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permission = %o, want 700", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var saved map[string]string
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	assertSavedValue(t, saved, "access_token", "access-123")
	assertSavedValue(t, saved, "refresh_token", "refresh-123")
	assertSavedValue(t, saved, "token_type", "bearer")
	assertSavedValue(t, saved, "scope", "tweet.read users.read bookmark.read offline.access")
	assertSavedValue(t, saved, "expires_at", "2026-06-07T12:34:56Z")

	for _, forbidden := range []string{"client_id", "client_secret", "code_verifier", "state"} {
		if _, ok := saved[forbidden]; ok {
			t.Fatalf("saved JSON contains forbidden field %q", forbidden)
		}
	}
}

func TestSaveTightensExistingPermissionsAndReplacesToken(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "existing")
	path := filepath.Join(dir, "token.json")
	expiresAt := time.Date(2026, 6, 7, 13, 14, 15, 0, time.UTC)

	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("Chmod(parent) error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"access_token":"old-access","refresh_token":"old-refresh"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod(file) error = %v", err)
	}

	token := xoauth.Token{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		TokenType:    "bearer",
		Scope:        "tweet.read users.read bookmark.read offline.access",
		ExpiresAt:    expiresAt,
	}

	if err := Save(path, token); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(parent) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permission = %o, want 700", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file permission = %o, want 600", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var saved map[string]string
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	assertSavedValue(t, saved, "access_token", "new-access")
	assertSavedValue(t, saved, "refresh_token", "new-refresh")
	assertSavedValue(t, saved, "token_type", "bearer")
	assertSavedValue(t, saved, "scope", "tweet.read users.read bookmark.read offline.access")
	assertSavedValue(t, saved, "expires_at", "2026-06-07T13:14:15Z")
}

func TestDefaultPathUsesUserConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() error = %v", err)
	}

	want := filepath.Join(configDir, "xapi-usecase", "token.json")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestLoadReadsSavedToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	writeTokenJSON(t, path, `{"access_token":"access-123","refresh_token":"refresh-123","token_type":"bearer","scope":"tweet.read users.read bookmark.read offline.access","expires_at":"2026-06-07T12:34:56Z"}`)

	token, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if token.AccessToken != "access-123" {
		t.Fatalf("AccessToken = %q, want access-123", token.AccessToken)
	}
	if token.RefreshToken != "refresh-123" {
		t.Fatalf("RefreshToken = %q, want refresh-123", token.RefreshToken)
	}
	if token.TokenType != "bearer" {
		t.Fatalf("TokenType = %q, want bearer", token.TokenType)
	}
	if token.Scope != "tweet.read users.read bookmark.read offline.access" {
		t.Fatalf("Scope = %q, want default scope", token.Scope)
	}
	wantExpiresAt := time.Date(2026, 6, 7, 12, 34, 56, 0, time.UTC)
	if !token.ExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("ExpiresAt = %s, want %s", token.ExpiresAt, wantExpiresAt)
	}
}

func TestLoadIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	writeTokenJSON(t, path, `{"access_token":"access-123","refresh_token":"refresh-123","token_type":"bearer","scope":"tweet.read","expires_at":"2026-06-07T12:34:56Z","unknown":"ignored"}`)

	if _, err := Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsMissingOrEmptyAccessToken(t *testing.T) {
	for _, test := range []struct {
		name string
		json string
	}{
		{name: "missing", json: `{"expires_at":"2026-06-07T12:34:56Z"}`},
		{name: "empty", json: `{"access_token":"","expires_at":"2026-06-07T12:34:56Z"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token.json")
			writeTokenJSON(t, path, test.json)

			if _, err := Load(path); err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestLoadRejectsMissingEmptyOrInvalidExpiresAt(t *testing.T) {
	for _, test := range []struct {
		name string
		json string
	}{
		{name: "missing", json: `{"access_token":"access-123"}`},
		{name: "empty", json: `{"access_token":"access-123","expires_at":""}`},
		{name: "invalid", json: `{"access_token":"access-123","expires_at":"not-a-time"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token.json")
			writeTokenJSON(t, path, test.json)

			if _, err := Load(path); err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func writeTokenJSON(t *testing.T, path string, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func assertSavedValue(t *testing.T, saved map[string]string, key string, want string) {
	t.Helper()

	if got := saved[key]; got != want {
		t.Fatalf("saved[%s] = %q, want %q", key, got, want)
	}
}
