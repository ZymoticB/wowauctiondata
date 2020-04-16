package fetchrealms

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/pubsub"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/storage"
	"github.com/ZymoticB/wowauctiondata/wowapiclient"
	"github.com/pkg/errors"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

const (
	_targetName         = "fetch-realms"
	_secretFetchTimeout = 5 * time.Second

	_clientIDSecretName     = "projects/13595582905/secrets/blizzard-oauth-client-id/versions/latest"
	_clientSecretSecretName = "projects/13595582905/secrets/blizzard-oauth-client-secret/versions/latest"

	_destBucketName = "wow-realm-data"

	_region = "us"

	_datasetID     = "wow_data"
	_tableID       = "connected_realms"
	_writeTruncate = "truncate"
)

var _projectID = os.Getenv("GCP_PROJECT")

// PubSubContainer is a container for the inbound pubsub message which is provided in
// Data.
type PubSubContainer struct {
	Data []byte `json:"data"`
}

// PubSubMessage is the decoded pub/sub message sent to the application
type PubSubMessage struct {
	Target string `json:"target"`
}

// pubsubClient is a global Pub/Sub client, initialized once per instance.
var pubsubClient *pubsub.Client

func init() {
	// err is pre-declared to avoid shadowing client.
	var err error

	// client is initialized with context.Background() because it should
	// persist between function invocations.
	pubsubClient, err = pubsub.NewClient(context.Background(), _projectID)
	if err != nil {
		log.Fatalf("pubsub.NewClient: %v", err)
	}
}

// FetchRealms is a cloud function to fetch all wow realms
func FetchRealms(ctx context.Context, m PubSubContainer) error {
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
		log.Printf("failed to fetch secrets: %v", err)
		return err
	}

	realms, err := fetchRealms(ctx, secrets)
	if err != nil {
		log.Printf("failed to fetch realms: %v", err)
		return err
	}
	log.Printf("Got realms %v", realms)

	gcsRef, err := writeRealmsToStorage(ctx, realms)
	if err != nil {
		log.Printf("failed to write to storage: %v", err)
		return err
	}

	if err := notifyStorageToBigQuery(ctx, gcsRef); err != nil {
		return errors.Wrap(err, "failed to notify storagetobigquery")
	}

	log.Printf("successfully wrote realms to storage")
	return nil
}

func writeRealmsToStorage(ctx context.Context, realms wowapiclient.ConnectedRealms) (string, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to create gcp client")
	}

	bkt := client.Bucket(_destBucketName)
	obj := bkt.Object("realms")
	writer := obj.NewWriter(ctx)
	csvWriter := csv.NewWriter(writer)
	for name, cr := range realms {
		err := csvWriter.Write([]string{name, strconv.Itoa(cr.ID)})
		if err != nil {
			return "", errors.Wrap(err, "failed to write to storage")
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return "", errors.Wrap(err, "failed to write to storage")
	}

	if err := writer.Close(); err != nil {
		return "", errors.Wrap(err, "failed to write to storage")
	}

	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to read back attrs")
	}
	return attrs.MediaLink, nil
}

// TODO move this to a shared module
type pubSubMessage struct {
	// expected to be of the form gs://...
	GCSReference string `json:"gcsReference"`
	DatasetID    string `json:"datasetID"`
	TableID      string `json:"tableID"`
	WriteMode    string `json:"writeMode"`
}

func notifyStorageToBigQuery(ctx context.Context, gcsRef string) error {
	msg := pubSubMessage{
		GCSReference: gcsRef,
		DatasetID:    _datasetID,
		TableID:      _tableID,
		WriteMode:    _writeTruncate,
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return errors.Wrap(err, "failed to marshal pubsub message")
	}

	t := pubsubClient.Topic("storagetobigtable")
	_, err = t.Publish(ctx, &pubsub.Message{
		Data: b,
	}).Get(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to publish message to pubsub")
	}

	log.Printf("wrote message to storagetobigtable")
	return nil
}

func fetchRealms(ctx context.Context, secrets map[string]string) (wowapiclient.ConnectedRealms, error) {
	httpClient, err := wowapiclient.GetHTTPClient(ctx, wowapiclient.OAuth2Secrets{
		ClientID:     secrets[_clientIDSecretName],
		ClientSecret: secrets[_clientSecretSecretName],
	}, _region)
	if err != nil {
		log.Printf("failed to get oauth2 http client %v", err)
		return nil, err
	}

	apiClient := wowapiclient.NewWOWAPIClient(httpClient, _region)

	return apiClient.GetConnectedRealms()
}

func getMessage(m PubSubContainer) (PubSubMessage, error) {
	msg := PubSubMessage{}

	decoder := json.NewDecoder(bytes.NewReader(m.Data))
	err := decoder.Decode(&msg)
	if err != nil {
		return PubSubMessage{}, errors.Wrapf(err, "failed to decode json %q", string(m.Data))
	}
	return msg, nil
}

// fetchSecrets fetches secrets from the secretmanager API using the keys of toFetch as the
// secret names. It mutates the given toFetch map.
func fetchSecrets(ctx context.Context, toFetch map[string]string) error {
	secretClient, err := secretmanager.NewClient(ctx)
	defer secretClient.Close()

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
