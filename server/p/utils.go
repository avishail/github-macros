package p

import (
	"context"

	"cloud.google.com/go/bigquery"
)

type MacroRow struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	Thumbnail        string `json:"thumbnail"`
	IsGif            bool   `json:"is_gif" bigquery:"is_gif"`
	GifThumbnail     string `json:"gif_thumbnail" bigquery:"gif_thumbnail"`
	OrigURL          string `json:"orig_url" bigquery:"orig_url"`
	URLSize          int64  `json:"url_size" bigquery:"url_size"`
	ThumbnailSize    int64  `json:"thumbnail_size" bigquery:"thumbnail_size"`
	GifThumbnailSize int64  `json:"gif_thumbnail_size" bigquery:"gif_thumbnail_size"`
}

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
