package storagetobigtable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
)

var _projectID = os.Getenv("GCP_PROJECT")

// PubSubContainer is a container for the inbound pubsub message which is provided in
// Data.
type PubSubContainer struct {
	Data []byte `json:"data"`
}

type pubSubMessage struct {
	// expected to be of the form gs://...
	GCSReference string    `json:"gcsReference"`
	DatasetID    string    `json:"datasetID"`
	TableID      string    `json:"tableID"`
	WriteMode    writeMode `json:"writeMode"`
}

// ifempty, truncate, append
type writeMode bigquery.TableWriteDisposition

func (w *writeMode) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "ifempty":
		*w = writeMode(bigquery.WriteEmpty)
	case "truncate":
		*w = writeMode(bigquery.WriteTruncate)
	case "append":
		*w = writeMode(bigquery.WriteAppend)
	default:
		return fmt.Errorf("cannot unmarshal %q as writeMode", string(b))
	}
	return nil
}

// LoadFromStorageToBigTable loads a csv from storage and imports it to big table.
func LoadFromStorageToBigTable(ctx context.Context, m PubSubContainer) error {
	params := pubSubMessage{}
	decoder := json.NewDecoder(bytes.NewReader(m.Data))

	if err := decoder.Decode(&params); err != nil {
		return errors.Wrapf(err, "failed to decode expected parameters from %q", string(m.Data))
	}

	client, err := bigquery.NewClient(ctx, _projectID)
	if err != nil {
		return errors.Wrap(err, "failed to create big query client")
	}
	defer client.Close()

	gcsRef := bigquery.NewGCSReference(params.GCSReference)

	loader := client.Dataset(params.DatasetID).Table(params.TableID).LoaderFrom(gcsRef)
	loader.WriteDisposition = bigquery.TableWriteDisposition(params.WriteMode)

	job, err := loader.Run(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to run loader job")
	}
	status, err := job.Wait(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to wait for loader job")
	}

	if status.Err() != nil {
		return fmt.Errorf("job completed with error: %v", status.Err())
	}
	return nil
}
