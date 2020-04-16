package storagetobigtable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
)

const _projectID = "wow-auction-data-274214"

// PubSubContainer is a container for the inbound pubsub message which is provided in
// Data.
type PubSubContainer struct {
	Data []byte `json:"data"`
}

type pubSubMessage struct {
	// expected to be of the form gs://...
	GCSReference string `json:"sourceBucket"`
	DatasetID    string `json:"datasetID"`
	TableID      string `json:"tableID"`
}

// LoadFromStorageToBigTable loads a csv from storage and imports it to big table.
func LoadFromStorageToBigTable(ctx context.Context, m PubSubContainer) error {
	params := pubSubMessage{}
	decoder := json.NewDecoder(bytes.NewReader(m.Data))

	if err := decoder.Decode(&params); err != nil {
		return errors.Wrapf("failed to decode expected parameters from %q: %v", string(m.Data), err)
	}

	client, err := bigquery.NewClient(ctx, _projectID)
	if err != nil {
		return errors.Wrap(err, "failed to create big query client")
	}
	defer client.Close()

	gscRef := bigquery.NewGCPReference(params.GCSReference)

	loader := client.Dataset(params.DatasetID).Table(params.TableID).LoaderFrom(gcsRef)
	loader.WriteDisposition = bigquery.WriteEmpty

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
