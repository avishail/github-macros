package p

import (
	"context"

	"cloud.google.com/go/bigquery"
)

func runQuery(ctx context.Context, query *bigquery.Query) (*bigquery.RowIterator, error) {
	job, err := query.Run(ctx)

	if err != nil {
		return nil, err
	}

	status, err := job.Wait(ctx)
	if err != nil {
		return nil, err
	}

	if status.Err() != nil {
		return nil, status.Err()
	}

	iter, err := job.Read(ctx)
	if err != nil {
		return nil, err
	}

	return iter, nil
}
