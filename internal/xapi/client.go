package xapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	BaseURL           = "https://api.x.com"
	maxErrorBodyBytes = 4 * 1024
)

type Client struct {
	AccessToken string
	BaseURL     string
	HTTPClient  *http.Client
}

type HTTPError struct {
	StatusCode int
	Status     string
	Path       string
	Body       string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("X API %s returned %s: %s", e.Path, e.Status, e.Body)
}

type MeResponse struct {
	Data User `json:"data"`
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type BookmarkOptions struct {
	MaxResults      int
	PaginationToken string
	TweetFields     string
	Expansions      string
	UserFields      string
	MediaFields     string
	PollFields      string
	PlaceFields     string
}

func (c *Client) Do(
	ctx context.Context,
	method string,
	path string,
	body io.Reader,
) (*http.Response, error) {
	requestURL, err := c.resolveURL(path)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.AccessToken)

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return response, nil
	}

	defer func() {
		_ = response.Body.Close()
	}()
	bodyBytes, err := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
	if err != nil {
		return nil, err
	}

	return nil, HTTPError{
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Path:       request.URL.Path,
		Body:       string(bodyBytes),
	}
}

func (c *Client) Me(ctx context.Context) (MeResponse, error) {
	response, err := c.Do(ctx, http.MethodGet, "/2/users/me", nil)
	if err != nil {
		return MeResponse{}, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	var me MeResponse
	if err := json.NewDecoder(response.Body).Decode(&me); err != nil {
		return MeResponse{}, err
	}

	return me, nil
}

func (c *Client) BookmarksRaw(
	ctx context.Context,
	userID string,
	options BookmarkOptions,
) ([]byte, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	query := url.Values{}
	if options.MaxResults > 0 {
		query.Set("max_results", fmt.Sprintf("%d", options.MaxResults))
	}
	if options.PaginationToken != "" {
		query.Set("pagination_token", options.PaginationToken)
	}
	if options.TweetFields != "" {
		query.Set("tweet.fields", options.TweetFields)
	}
	if options.Expansions != "" {
		query.Set("expansions", options.Expansions)
	}
	if options.UserFields != "" {
		query.Set("user.fields", options.UserFields)
	}
	if options.MediaFields != "" {
		query.Set("media.fields", options.MediaFields)
	}
	if options.PollFields != "" {
		query.Set("poll.fields", options.PollFields)
	}
	if options.PlaceFields != "" {
		query.Set("place.fields", options.PlaceFields)
	}

	path := "/2/users/" + url.PathEscape(userID) + "/bookmarks"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	response, err := c.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	return io.ReadAll(response.Body)
}

func (c *Client) resolveURL(path string) (string, error) {
	baseURL, err := url.Parse(c.baseURL())
	if err != nil {
		return "", err
	}

	pathURL, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	if pathURL.IsAbs() || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("x API path must be relative to base URL: %s", path)
	}

	return baseURL.ResolveReference(pathURL).String(), nil
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}

	return BaseURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return http.DefaultClient
}
