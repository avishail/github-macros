// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

var supportedTypes = map[string]bool{
	"image/gif":  true,
	"image/png":  true,
	"image/bmp":  true,
	"image/jpeg": true,
}

func isGif(fileType string) bool {
	return fileType == "image/gif"
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
	macroOrigURL := r.Form.Get("orig_url")

	errCode := validateNameAndURL(client, macroName, macroURL)

	if errCode != Success {
		return nil, errCode
	}

	fileSize, fileType, errCode := getFileSizeAndType(macroURL)
	if errCode != Success {
		return nil, errCode
	}

	if errCode := validateFileSizeAndType(fileSize, fileType, cFileMaxSize); errCode != Success {
		return nil, errCode
	}

	isGif := isGif(fileType)

	if isGithubMedia(macroURL) {
		insertNewMacro(client, macroName, macroURL, "", isGif, "", macroOrigURL, fileSize, 0, 0)

		newMacro := &MacroRow{
			Name: macroName,
			URL:  macroURL,
		}

		err := publishNewMacroMessage(macroName)

		if err == nil {
			return newMacro, Success
		}

		log.Printf("publishNewMacroMessage failed. retrying: %v", err)

		err = publishNewMacroMessage(macroName)

		if err == nil {
			return newMacro, Success
		}

		log.Printf("publishNewMacroMessage failed again. falling back to full flow: %v", err)
	}

	newMacro, success := postprocessNewMacro(client, macroName, macroURL, fileSize, isGif, true)
	if !success {
		log.Panic("failed to run postprocessNewMacro")
	}

	return newMacro, Success
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

func validateNameAndURL(client *bigquery.Client, macroName, macroURL string) ErrorCode {
	if macroName == "" {
		return EmptyName
	}

	if macroURL == "" {
		return EmptyURL
	}

	if strings.Contains(macroName, " ") {
		return NameContainsSpaces
	}

	isExist := isMacroExist(macroName, client)

	if isExist {
		return NameAlreadyExist
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

func isMacroExist(name string, client *bigquery.Client) bool {
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
		log.Panicf("failed to check if name already exist: %v", err)
	}

	type Row struct{}

	var r Row
	err = iter.Next(&r)

	if err != nil && err != iterator.Done {
		log.Panicf("failed to read query results: %v", err)
	}

	return iter.TotalRows > 0
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
