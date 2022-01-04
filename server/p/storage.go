package p

import (
	"context"
	"io"

	"cloud.google.com/go/storage"
)

const gBucket = "github-macros-public-tmp-storage"

type WriteCloserWithSize struct {
	writeCloser io.WriteCloser
	totalBytes  int64
}

func (w *WriteCloserWithSize) Write(p []byte) (n int, err error) {
	n, err = w.writeCloser.Write(p)
	w.totalBytes += int64(n)

	return n, err
}

func (w *WriteCloserWithSize) Close() error {
	return w.writeCloser.Close()
}

func (w *WriteCloserWithSize) GetTotalBytes() int64 {
	return w.totalBytes
}

func newStorageClient() (*storage.Client, error) {
	ctx := context.Background()
	return storage.NewClient(ctx)
}

func closeStorageClient(client *storage.Client) error {
	return client.Close()
}

func getNewPublicFileWriter(client *storage.Client, filePath string) *WriteCloserWithSize {
	ctx := context.Background()
	object := client.Bucket(gBucket).Object(filePath)

	return &WriteCloserWithSize{writeCloser: object.NewWriter(ctx)}
}

func writeNewPublicFile(client *storage.Client, filePath string, writeFile func(io.Writer) error) (int64, error) {
	fileWriter := getNewPublicFileWriter(client, filePath)

	if err := writeFile(fileWriter); err != nil {
		return 0, err
	}

	if err := fileWriter.Close(); err != nil {
		return 0, err
	}

	return fileWriter.totalBytes, nil
}

func getPublicFileURL(filePath string) string {
	return "https://storage.googleapis.com/" + gBucket + "/" + filePath
}
