package xoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	AuthorizeEndpoint = "https://x.com/i/oauth2/authorize"
	TokenEndpoint     = "https://api.x.com/2/oauth2/token"
	DefaultScope      = "tweet.read users.read bookmark.read offline.access"
	challengeMethod   = "S256"
	maxErrorBodyBytes = 4 * 1024
)

type Client struct {
	ClientID          string
	HTTPClient        *http.Client
	AuthorizeEndpoint string
	TokenEndpoint     string
	Now               func() time.Time
}

type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scope        string
	ExpiresAt    time.Time
}

type HTTPError struct {
	StatusCode int
	Status     string
	Path       string
	Body       string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("token endpoint %s returned %s: %s", e.Path, e.Status, e.Body)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
}

func NewClient(clientID string) *Client {
	return &Client{ClientID: clientID}
}

func (c *Client) AuthURL(redirectURI, state, codeChallenge string) (string, error) {
	if c.ClientID == "" {
		return "", errors.New("client ID is required")
	}
	if redirectURI == "" {
		return "", errors.New("redirect URI is required")
	}
	if state == "" {
		return "", errors.New("state is required")
	}
	if codeChallenge == "" {
		return "", errors.New("code challenge is required")
	}

	endpoint, err := url.Parse(c.authorizeEndpoint())
	if err != nil {
		return "", err
	}

	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", c.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", DefaultScope)
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", challengeMethod)
	endpoint.RawQuery = query.Encode()

	return endpoint.String(), nil
}

func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", c.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	return c.postTokenForm(ctx, form, "")
}

func (c *Client) Refresh(ctx context.Context, current Token) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", c.ClientID)
	form.Set("refresh_token", current.RefreshToken)

	return c.postTokenForm(ctx, form, current.RefreshToken)
}

func (c *Client) postTokenForm(ctx context.Context, form url.Values, fallbackRefreshToken string) (Token, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return Token{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
		if readErr != nil {
			return Token{}, readErr
		}

		return Token{}, HTTPError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
			Path:       request.URL.Path,
			Body:       string(body),
		}
	}

	var tokenJSON tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&tokenJSON); err != nil {
		return Token{}, err
	}
	if tokenJSON.AccessToken == "" {
		return Token{}, errors.New("access token is required in token response")
	}

	refreshToken := tokenJSON.RefreshToken
	if refreshToken == "" {
		refreshToken = fallbackRefreshToken
	}

	token := Token{
		AccessToken:  tokenJSON.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    tokenJSON.TokenType,
		Scope:        tokenJSON.Scope,
	}
	if tokenJSON.ExpiresIn > 0 {
		token.ExpiresAt = c.now().Add(time.Duration(tokenJSON.ExpiresIn) * time.Second)
	}

	return token, nil
}

func (c *Client) authorizeEndpoint() string {
	if c.AuthorizeEndpoint != "" {
		return c.AuthorizeEndpoint
	}

	return AuthorizeEndpoint
}

func (c *Client) tokenEndpoint() string {
	if c.TokenEndpoint != "" {
		return c.TokenEndpoint
	}

	return TokenEndpoint
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return http.DefaultClient
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}

	return time.Now()
}
