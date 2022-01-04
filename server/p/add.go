// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"fmt"
	"log"
	"net/http"

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

func getMacroWithTheSameURL(client *bigquery.Client, macroURL string) (*MacroRow, error) {
	query := client.Query(
		"SELECT 1 FROM `github-macros.macros.macros` WHERE orig_url=@orig_url",
	)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "orig_url",
			Value: macroURL,
		},
	}

	ctx := context.Background()

	iter, err := runQuery(ctx, query)

	if err != nil {
		return nil, fmt.Errorf("error executing runQuery: %v", err)
	}

	var r MacroRow
	err = iter.Next(&r)

	if err != nil && err != iterator.Done {
		return nil, err
	}

	if err == iterator.Done {
		return nil, nil
	}

	return &r, nil
}

func createNewUsagesEntry(client *bigquery.Client, macroName string) error {
	query := client.Query(`
		INSERT INTO github-macros.macros.usages (macro_name, usages)
		SELECT '@macro_name', 0 FROM (SELECT 1) 
		LEFT JOIN github-macros.macros.usages
		ON macro_name = @macro_name
		WHERE macro_name IS NULL
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "macro_name",
			Value: macroName,
		},
	}

	_, err := runQuery(context.Background(), query)

	return err
}

func createNewReportsEntry(client *bigquery.Client, macroName string) error {
	query := client.Query(`
		INSERT INTO github-macros.macros.reports (macro_name, reports)
		SELECT '@macro_name', 0 FROM (SELECT 1) 
		LEFT JOIN github-macros.macros.reports
		ON macro_name = @macro_name
		WHERE macro_name IS NULL
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "macro_name",
			Value: macroName,
		},
	}

	_, err := runQuery(context.Background(), query)

	return err
}

func insertNewMacro(
	client *bigquery.Client,
	macroName, macroURL, thumbnailURL string,
	isGif bool,
	gifThumbnailURL, origURL string,
	macroSize,
	thumbnailSize,
	gifThumbnailSize int64,
) error {
	if err := createNewReportsEntry(client, macroName); err != nil {
		return err
	}

	if err := createNewUsagesEntry(client, macroName); err != nil {
		return err
	}

	query := client.Query(
		"INSERT INTO `github-macros.macros.macros` (name, orig_url, url, url_size, thumbnail, thumbnail_size, is_gif, gif_thumbnail, gif_thumbnail_size, width, height) VALUES (@name, @orig_url, @url, @url_size, @thumbnail, @thumbnail_size, @is_gif, @gif_thumbnail, @gif_thumbnail_size, 0, 0)",
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
		{
			Name:  "orig_url",
			Value: origURL,
		},
		{
			Name:  "url_size",
			Value: macroSize,
		},
		{
			Name:  "thumbnail",
			Value: thumbnailURL,
		},
		{
			Name:  "thumbnail_size",
			Value: thumbnailSize,
		},
		{
			Name:  "is_gif",
			Value: isGif,
		},
		{
			Name:  "gif_thumbnail",
			Value: gifThumbnailURL,
		},
		{
			Name:  "gif_thumbnail_size",
			Value: gifThumbnailSize,
		},
	}

	if _, err := runQuery(context.Background(), query); err != nil {
		return fmt.Errorf("failed to insert new macro: %v", err)
	}

	return nil
}

func duplicateExistingMacro(client *bigquery.Client, macroName string, macroToDuplicate *MacroRow) (*MacroRow, error) {
	if err := insertNewMacro(
		client,
		macroName,
		macroToDuplicate.URL,
		macroToDuplicate.Thumbnail,
		macroToDuplicate.IsGif,
		macroToDuplicate.GifThumbnail,
		macroToDuplicate.OrigURL,
		macroToDuplicate.URLSize,
		macroToDuplicate.ThumbnailSize,
		macroToDuplicate.GifThumbnailSize,
	); err != nil {
		return nil, err
	}

	var newMacro = *macroToDuplicate
	newMacro.Name = macroName

	return &newMacro, nil
}

func executaAdd(r *http.Request) (*MacroRow, ErrorCode, error) {
	ctx := context.Background()

	client, err := bigquery.NewClient(ctx, "github-macros")
	if err != nil {
		return nil, InfraFailure, err
	}
	defer client.Close()

	macroName := r.Form.Get("name")
	macroURL := r.Form.Get("url")

	errCode, err := validateNameAndURL(client, macroName, macroURL)

	if errCode != Success || err != nil {
		return nil, errCode, err
	}

	fileSize, fileType, err := getFileSizeAndType(macroURL)
	if err != nil {
		return nil, InvalidURL, err
	}

	if errCode := validateFileSizeAndType(fileSize, fileType, cFileMaxSize); errCode != Success {
		return nil, errCode, nil
	}

	macroRow, err := getMacroWithTheSameURL(client, macroURL)
	if err != nil {
		log.Printf("error while fetching macro with the same URL: %v", err)
	}

	if macroRow != nil {
		newMacro, dupMacroErr := duplicateExistingMacro(client, macroName, macroRow)
		if dupMacroErr != nil {
			return nil, InfraFailure, dupMacroErr
		}

		return newMacro, Success, nil
	}

	var (
		thumbnail        string
		gifThumbnail     string
		thumbnailSize    int64
		gifThumbnailSize int64
	)

	isGif := isGif(fileType)
	if isGif {
		thumbnail, thumbnailSize, gifThumbnail, gifThumbnailSize, err = processGif(macroURL, fileSize)
	} else {
		thumbnail, thumbnailSize, err = processImage(macroURL, fileSize)
	}

	if err != nil {
		return nil, InfraFailure, err
	}

	urlsToConvert := []string{macroURL}

	if thumbnail != "" {
		urlsToConvert = append(urlsToConvert, thumbnail)
	}

	if gifThumbnail != "" {
		urlsToConvert = append(urlsToConvert, gifThumbnail)
	}

	githubURLs, err := GetGithubImages(client, urlsToConvert)
	if err != nil {
		return nil, InfraFailure, err
	}

	finalMacroURL, ok := githubURLs[macroURL]
	if !ok {
		return nil, TransientError, fmt.Errorf("failed to locate github macro image")
	}

	finalThumbnailURL := githubURLs[thumbnail]
	finalGifThumbnailURL := githubURLs[gifThumbnail]

	if err := insertNewMacro(
		client,
		macroName,
		finalMacroURL,
		finalThumbnailURL,
		isGif,
		finalGifThumbnailURL,
		macroURL,
		fileSize,
		thumbnailSize,
		gifThumbnailSize,
	); err != nil {
		return nil, InfraFailure, err
	}

	newMacro := &MacroRow{
		Name:         macroName,
		URL:          finalMacroURL,
		Thumbnail:    finalThumbnailURL,
		IsGif:        isGif,
		GifThumbnail: finalGifThumbnailURL,
	}

	return newMacro, Success, nil
}

func Add(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	if err := r.ParseForm(); err != nil {
		log.Panicf("error while parsing form: %v", err)
	}

	newMacro, errCode, err := executaAdd(r)

	if errCode == InfraFailure {
		log.Panic(err)
	}

	var response string

	if errCode != Success {
		if err != nil {
			log.Println(err)
		}
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
