// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"cloud.google.com/go/bigquery"
)

var supportedTypes = map[string]bool{
	"image/gif":  true,
	"image/png":  true,
	"image/bmp":  true,
	"image/jpeg": true,
}

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
	MissingMandatoryFields  = 10
	InfraFailure            = 11
	PermanentError          = 12
)

type ErrorCode = int

func isGithubMedia(macroURL string) bool {
	u, err := url.Parse(macroURL)
	if err != nil {
		return false
	}

	return strings.HasSuffix(u.Host, "githubusercontent.com")
}

func validateFileSizeAndType(fileSize int64, fileType string, maxSize int64) ErrorCode {
	if fileSize > maxSize {
		return FileIsTooBig
	}

	if _, ok := supportedTypes[fileType]; !ok {
		return FileFormatNotSupported
	}

	return Success
}

func executaAdd(r *http.Request) (*MacroRow, ErrorCode) {
	ctx := context.Background()

	client, err := bigquery.NewClient(ctx, "github-macros")
	if err != nil {
		log.Panicf("failed to get bigquery client: %v", err)
	}

	defer client.Close()

	macroName := r.Form.Get("name")
	macroURL := r.Form.Get("url")

	if errCode := staticNameAndURLValidation(macroName, macroURL); errCode != Success {
		return nil, errCode
	}

	isExist, sameURLMacro := queryExistingMacroMetadata(macroName, macroURL, client)

	if isExist {
		return nil, NameAlreadyExist
	}

	if sameURLMacro != nil {
		newMacro := duplicateExistingMacro(client, macroName, sameURLMacro)
		return newMacro, Success
	}

	fileSize, fileType, errCode := getFileSizeAndType(macroURL)
	if errCode != Success {
		return nil, errCode
	}

	if errCode := validateFileSizeAndType(fileSize, fileType, cFileMaxSize); errCode != Success {
		return nil, errCode
	}

	width, height := getMacroDimensions(macroURL)

	insertNewMacro(client, macroName, macroURL, fileSize, width, height)

	return &MacroRow{
		Name:   macroName,
		URL:    macroURL,
		Width:  width,
		Height: height,
	}, Success
}

func getFileSizeAndType(macroURL string) (fileSize int64, fileType string, errCode ErrorCode) {
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

	fileSize = getFileSize(resp)
	if fileSize > cFileMaxSize {
		return 0, "", FileIsTooBig
	}

	body := make([]byte, 512)
	_, err = resp.Body.Read(body)

	if err != nil {
		return 0, "", InvalidURL
	}

	fileType = http.DetectContentType(body)

	return fileSize, fileType, Success
}

func staticNameAndURLValidation(macroName, macroURL string) ErrorCode {
	if macroName == "" {
		return EmptyName
	}

	if macroURL == "" {
		return EmptyURL
	}

	if strings.Contains(macroName, " ") {
		return NameContainsSpaces
	}

	return Success
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

func queryExistingMacroMetadata(macroName, macroURL string, client *bigquery.Client) (bool, *MacroRow) {
	query := client.Query(
		"SELECT * FROM github-macros.macros.macros WHERE name=@name OR url=@url",
	)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "name",
			Value: macroName,
		},
		{
			Name:  "url",
			Value: macroURL,
		},
	}

	results := getQueryResults(query)

	var sameURL *MacroRow

	for _, res := range results {
		if res.Name == macroName {
			return true, nil
		}

		if res.URL == macroURL {
			sameURL = res
		}
	}

	return false, sameURL
}

func Add(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	if err := r.ParseForm(); err != nil {
		log.Panicf("error while parsing form: %v", err)
	}

	newMacro, errCode := executaAdd(r)

	var (
		response string
		err      error
	)

	if errCode != Success {
		response, err = getErrorCodeResponse(errCode)
	} else {
		response, err = getRespons(
			map[string]interface{}{
				"code": Success,
				"data": *newMacro,
			},
		)
	}

	if err != nil {
		log.Panicf("failed to create response %v", err)
	}

	_, err = fmt.Fprint(w, response)

	if err != nil {
		log.Panicf("error writing response: %v", err)
	}
}

func getMacroDimensions(macroURL string) (width, height int64) {
	config, err := getImageConfig(macroURL)
	if err != nil {
		log.Panicf("getMacroDimensions: %v", err)
	}

	return int64(config.Width), int64(config.Height)
}

func getErrorCodeResponse(errCode ErrorCode) (string, error) {
	return getRespons(
		map[string]interface{}{
			"code": errCode,
		},
	)
}

func getRespons(responseMap map[string]interface{}) (string, error) {
	response, err := json.Marshal(responseMap)

	if err != nil {
		return "", fmt.Errorf("error while marshaling response: %v", err)
	}

	return string(response), nil
}

func insertNewMacro(
	client *bigquery.Client,
	macroName, macroURL string,
	macroSize, width, height int64,
) {
	query := client.Query(`
		INSERT INTO github-macros.macros.macros 
		(name, url, url_size, width, height)
		VALUES (@name, @url, @url_size, @width, @height)
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "name",
			Value: macroName,
		},
		{
			Name:  "url",
			Value: macroURL,
		},
		{
			Name:  "url_size",
			Value: macroSize,
		},
		{
			Name:  "width",
			Value: width,
		},
		{
			Name:  "height",
			Value: height,
		},
	}

	if _, err := runQuery(context.Background(), query); err != nil {
		log.Panicf("failed to insert new macro: %v", err)
	}
}

func duplicateExistingMacro(client *bigquery.Client, macroName string, macroToDuplicate *MacroRow) *MacroRow {
	insertNewMacro(
		client,
		macroName,
		macroToDuplicate.URL,
		macroToDuplicate.URLSize,
		macroToDuplicate.Width,
		macroToDuplicate.Height,
	)

	var newMacro = *macroToDuplicate
	newMacro.Name = macroName

	return &newMacro
}
