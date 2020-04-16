package fetchrealms

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/ZymoticB/wowauctiondata/wowapiclient"
	"github.com/pkg/errors"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

const (
	_targetName         = "fetch-realms"
	_secretFetchTimeout = 5 * time.Second

	_clientIDSecretName     = "projects/13595582905/secrets/blizzard-oauth-client-id/versions/latest"
	_clientSecretSecretName = "projects/13595582905/secrets/blizzard-oauth-client-secret/version/latest"

	_region = "us"
)

// PubSubContainer is a container for the inbound pubsub message which is actually base64
// encoded within Data
type PubSubContainer struct {
	Data []byte `json:"data"`
}

// PubSubMessage is the decoded pub/sub message sent to the application
type PubSubMessage struct {
	Target string `json:"target"`
}

// FetchRealms is a cloud function to fetch all wow realms
func FetchRealms(ctx context.Context, m PubSubContainer) error {
	log.Println("in function")
	if len(m.Data) == 0 {
		log.Println("got empty message, skipping")
		return nil
	}

	msg, err := getMessage(m)
	if err != nil {
		log.Printf("failed to get message from pub/sub message: %v", err)
		return err
	}

	if msg.Target != _targetName {
		log.Printf("trigger intended for a different target %q", msg.Target)
		return nil
	}

	secrets := map[string]string{
		_clientIDSecretName:     "",
		_clientSecretSecretName: "",
	}

	err = fetchSecrets(ctx, secrets)
	if err != nil {
		log.Fatalf("failed to fetch secrets: %v", err)
	}

	httpClient, err := wowapiclient.GetHTTPClient(ctx, wowapiclient.OAuth2Secrets{
		ClientID:     secrets[_clientIDSecretName],
		ClientSecret: secrets[_clientSecretSecretName],
	}, _region)
	if err != nil {
		log.Printf("failed to get oauth2 http client %v", err)
		return err
	}

	apiClient := wowapiclient.NewWOWAPIClient(httpClient, _region)

	realms, err := apiClient.GetConnectedRealms()
	log.Printf("Got realms %v", realms)

	return nil
}

func getMessage(m PubSubContainer) (PubSubMessage, error) {
	out := make([]byte, len(m.Data))
	_, err := base64.StdEncoding.Strict().Decode(out, m.Data)
	if err != nil {
		return PubSubMessage{}, errors.Wrapf(err, "failed to decode base64 as string %q, raw %v", string(m.Data), m.Data)
	}

	msg := PubSubMessage{}

	decoder := json.NewDecoder(bytes.NewReader(out))
	err = decoder.Decode(&msg)
	if err != nil {
		return PubSubMessage{}, errors.Wrapf(err, "failed to decode json %q", string(out))
	}
	return msg, nil
}

// fetchSecrets fetches secrets from the secretmanager API using the keys of toFetch as the
// secret names. It mutates the given toFetch map.
func fetchSecrets(ctx context.Context, toFetch map[string]string) error {
	setupCtx := context.Background()
	secretClient, err := secretmanager.NewClient(setupCtx)

	if err != nil {
		return err
	}

	for name := range toFetch {
		secret, err := fetchSecret(ctx, secretClient, name)
		if err != nil {
			return errors.Wrapf(err, "failed to fetch secret for %q", name)
		}

		toFetch[name] = secret
	}

	return nil
}

func fetchSecret(ctx context.Context, secretClient *secretmanager.Client, name string) (string, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, _secretFetchTimeout)
	defer cancel()

	secret, err := secretClient.AccessSecretVersion(fetchCtx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to fetch %q", name)
	}

	payload := secret.GetPayload()
	if payload == nil {
		return "", fmt.Errorf("sercet %q returned an empty payload", name)
	}

	data := payload.GetData()
	if len(data) == 0 {
		return "", fmt.Errorf("secret %q returned empty data", name)
	}

	return string(data), nil
}
