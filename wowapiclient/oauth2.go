package wowapiclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const _apiTokenURLFormat = "https://%s.battle.net/oauth/token"

// OAuth2Secrets are the secrets required to perform the oauth2 auth flow
type OAuth2Secrets struct {
	ClientID     string
	ClientSecret string
}

// GetHTTPClient sets up an HTTP Client which will automatically refresh a client oauth2
// token.
func GetHTTPClient(ctx context.Context, secrets OAuth2Secrets, region string) (*http.Client, error) {
	oauth2ClientConfig := clientcredentials.Config{
		ClientID:     secrets.ClientID,
		ClientSecret: secrets.ClientSecret,
		TokenURL:     fmt.Sprintf(_apiTokenURLFormat, region),
		AuthStyle:    oauth2.AuthStyleInHeader,
	}

	// Get an initial token to verify the client can complete the oauth2 flow
	_, err := oauth2ClientConfig.Token(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to perform 2 legged oauth2 client auth")
	}

	return oauth2ClientConfig.Client(ctx), nil
}
