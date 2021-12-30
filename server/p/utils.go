package p

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/bigquery"
	"github.com/nfnt/resize"
	"google.golang.org/api/iterator"
)

const (
	Success                 = 0
	EmptyName               = 1
	NameContainsSpaces      = 2
	NameAlreadyExist        = 3
	EmptyURL                = 4
	InvalidURL              = 5
	URLHostnameNotSupported = 6
	FileIsTooBig            = 7
	FileFormatNotSupported  = 8
	TransientError          = 9
)

const (
	cFileMaxSize         = 1024 * 1024 * 10
	cThumbnailMaxSize    = 1024 * 400
	cGifThumbnailMaxSize = 1024 * 700
)

type ErrorCode = int

var supportedTypes = map[string]bool{
	"image/gif":  true,
	"image/png":  true,
	"image/bmp":  true,
	"image/jpeg": true,
}

func isGif(fileType string) bool {
	return fileType == "image/gif"
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

func isMacroExist(name string, client *bigquery.Client) (bool, error) {
	query := client.Query(
		"SELECT 1 FROM `github-macros.macros.macros` WHERE name=@name",
	)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "name",
			Value: name,
		},
	}

	ctx := context.Background()

	iter, err := runQuery(ctx, query)

	if err != nil {
		return false, fmt.Errorf("error executing runQuery: %v", err)
	}

	type Row struct{}

	var r Row
	err = iter.Next(&r)

	if err != nil && err != iterator.Done {
		return false, err
	}

	return iter.TotalRows > 0, nil
}

func getFileSize(r *http.Response) int64 {
	if r.Header.Get("Content-Range") != "" {
		parts := strings.Split(r.Header.Get("Content-Range"), "/")
		if len(parts) == 2 {
			size, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil {
				return size
			}
		}
	}

	if r.Header.Get("Content-Length") != "" {
		size, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
		if err == nil {
			if size > r.ContentLength {
				return size
			}

			return r.ContentLength
		}
	}

	return r.ContentLength
}

func getFileSizeAndType(macroURL string) (int64, string, ErrorCode) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", macroURL, http.NoBody)

	if err != nil {
		return 0, "", InvalidURL
	}

	req.Header.Add("Range", "bytes=0-512")

	var client http.Client

	resp, err := client.Do(req)

	if err != nil {
		return 0, "", InvalidURL
	}

	fileSize := getFileSize(resp)
	if fileSize > cFileMaxSize {
		return 0, "", FileIsTooBig
	}

	body := make([]byte, 512)
	_, err = resp.Body.Read(body)

	if err != nil {
		return 0, "", InvalidURL
	}

	fileType := http.DetectContentType(body)

	if _, ok := supportedTypes[fileType]; !ok {
		return 0, "", FileFormatNotSupported
	}

	return fileSize, fileType, Success
}

func getErrorCodeResponse(errCode ErrorCode) (string, error) {
	responseMap := map[string]interface{}{
		"code": errCode,
	}

	response, err := json.Marshal(responseMap)

	if err != nil {
		return "", fmt.Errorf("error while marshaling response: %v", err)
	}

	return string(response), nil
}

func writeResponse(w http.ResponseWriter, response string) {
	_, err := fmt.Fprint(w, response)

	if err != nil {
		log.Fatalf("error writing response: %v", err)
	}
}
func writeErrorCodeResponse(w http.ResponseWriter, errCode ErrorCode) {
	response, err := getErrorCodeResponse(errCode)
	if err != nil {
		log.Fatalf("error while marshaling response: %v", err)
		return
	}

	writeResponse(w, response)
}

func validateNameAndURL(macroName, macroURL string, client *bigquery.Client) (ErrorCode, error) {
	if macroName == "" {
		return EmptyName, nil
	}

	if macroURL == "" {
		return EmptyURL, nil
	}

	if strings.Contains(macroName, " ") {
		return NameContainsSpaces, nil
	}

	isExist, err := isMacroExist(macroName, client)

	if err != nil {
		return TransientError, fmt.Errorf("error checking if macro exist: %v", err)
	}

	if isExist {
		return NameAlreadyExist, nil
	}

	return Success, nil
}

func sendHTTPRequest(method, requestURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), method, requestURL, http.NoBody)
	defer req.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var client http.Client

	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %v", err)
	}

	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func resizeImage(m image.Image) image.Image {
	p := m.Bounds().Max
	width := uint(p.X)
	height := uint(p.Y)

	if width >= height {
		width = 150
		height = 0
	} else {
		height = 150
		width = 0
	}

	return resize.Resize(width, height, m, resize.NearestNeighbor)
}
