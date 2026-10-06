package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/y-writings/xapi-usecase/internal/tokenstore"
	"github.com/y-writings/xapi-usecase/internal/xapi"
	"github.com/y-writings/xapi-usecase/internal/xoauth"
)

const tokenRefreshSkew = 5 * time.Minute

var (
	timeNow          = time.Now
	defaultTokenPath = tokenstore.DefaultPath
	newOAuthClient   = xoauth.NewClient
	newXAPIClient    = func(accessToken string) *xapi.Client {
		return &xapi.Client{AccessToken: accessToken}
	}
)

type authenticatedClient struct {
	api     *xapi.Client
	refresh func() error
}

func newAuthenticatedClient(
	ctx context.Context,
	tokenFile, clientID, bearerToken string,
) (*authenticatedClient, error) {
	if bearerToken != "" {
		return &authenticatedClient{api: newXAPIClient(bearerToken)}, nil
	}
	if tokenFile == "" {
		path, err := defaultTokenPath()
		if err != nil {
			return nil, err
		}
		tokenFile = path
	}
	token, err := tokenstore.Load(tokenFile)
	if err != nil {
		return nil, err
	}

	client := &authenticatedClient{}
	client.refresh = func() error {
		if clientID == "" {
			return commandLineError(fmt.Sprintf(
				"client ID is required to refresh access token; set %s or pass --client-id",
				clientIDEnv,
			))
		}
		if token.RefreshToken == "" {
			return errors.New("refresh token is required to refresh access token")
		}

		refreshed, err := newOAuthClient(clientID).Refresh(ctx, token)
		if err != nil {
			return err
		}
		if err := tokenstore.Save(tokenFile, refreshed); err != nil {
			return err
		}
		token = refreshed
		client.api = newXAPIClient(token.AccessToken)
		return nil
	}
	if !token.ExpiresAt.After(timeNow().Add(tokenRefreshSkew)) {
		if err := client.refresh(); err != nil {
			return nil, err
		}
	} else {
		client.api = newXAPIClient(token.AccessToken)
	}
	return client, nil
}

func (c *authenticatedClient) run(call func(*xapi.Client) error) error {
	err := call(c.api)
	var httpError xapi.HTTPError
	if c.refresh == nil || !errors.As(err, &httpError) ||
		httpError.StatusCode != http.StatusUnauthorized {
		return err
	}
	if err := c.refresh(); err != nil {
		return err
	}
	return call(c.api)
}
