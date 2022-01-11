package p

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"

	_ "golang.org/x/image/bmp"
)

const (
	cFileMaxSize = 1024 * 1024 * 10
)

type readerWithMaxSize struct {
	Reader  io.Reader
	MaxSize int64
	curSize int64
}

func (r *readerWithMaxSize) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)

	if err == nil || err == io.EOF {
		r.curSize += int64(n)
		if r.curSize > r.MaxSize {
			return n, errors.New("network reader exceed read limit")
		}
	}

	return n, err
}

func sendHTTPGetRequest(requestURL string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, requestURL, http.NoBody)

	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %v", err)
	}

	defer resp.Body.Close()

	return io.ReadAll(
		&readerWithMaxSize{Reader: resp.Body, MaxSize: cFileMaxSize},
	)
}

func getImageConfig(macroURL string) (*image.Config, error) {
	macroBuf, err := sendHTTPGetRequest(macroURL)
	if err != nil {
		return nil, fmt.Errorf("failed to read image '%s': %v", macroURL, err)
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(macroBuf))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image '%s': %v", macroURL, err)
	}

	return &config, nil
}
