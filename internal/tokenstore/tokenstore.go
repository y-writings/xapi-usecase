package tokenstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

type savedToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresAt    string `json:"expires_at"`
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "xapi-usecase", "token.json"), nil
}

func Save(path string, token xoauth.Token) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}

	data, err := json.Marshal(savedToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Scope:        token.Scope,
		ExpiresAt:    token.ExpiresAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(dir, ".token-*.tmp")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return err
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	removeTemp = false
	return os.Chmod(path, 0o600)
}
