package p

import (
	"context"
	"io"

	"cloud.google.com/go/storage"
)

const gBucket = "github-macros-public-tmp-storage"

func newStorageClient() (*storage.Client, error) {
	ctx := context.Background()
	return storage.NewClient(ctx)
}

func closeStorageClient(client *storage.Client) error {
	return client.Close()
}

func getNewPublicFileWriter(client *storage.Client, filePath string) io.WriteCloser {
	ctx := context.Background()
	object := client.Bucket(gBucket).Object(filePath)

	return object.NewWriter(ctx)
}

func writeNewPublicFile(client *storage.Client, filePath string, writeFile func(io.Writer) error) error {
	fileWriter := getNewPublicFileWriter(client, filePath)

	if err := writeFile(fileWriter); err != nil {
		return err
	}

	if err := fileWriter.Close(); err != nil {
		return err
	}

	return nil
}

func getPublicFileURL(filePath string) string {
	return "https://storage.googleapis.com/" + gBucket + "/" + filePath
}
