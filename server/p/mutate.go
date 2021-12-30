// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"cloud.google.com/go/bigquery"
)

const mutateTypeAdd = "add"
const mutateTypeUse = "use"
const mutateTypeReport = "report"

const reportsThreshold = 10

func validateGithubURL(urlToCheck string) ErrorCode {
	u, err := url.Parse(urlToCheck)
	if err != nil {
		return InvalidURL
	}

	if !strings.HasSuffix(u.Hostname(), "githubusercontent.com") {
		return URLHostnameNotSupported
	}

	return Success
}

func validateURL(macroURL string, maxSize int64) ErrorCode {
	if errCode := validateGithubURL(macroURL); errCode != Success {
		return errCode
	}

	fileSize, fileType, errCode := getFileSizeAndType(macroURL)

	if errCode != Success {
		return errCode
	}

	if fileSize > maxSize {
		return FileIsTooBig
	}

	if _, ok := supportedTypes[fileType]; !ok {
		return FileFormatNotSupported
	}

	return Success
}

func getMutationQuery(
	client *bigquery.Client,
	mutationType,
	macroName,
	macroURL,
	macroThumbnailURL string,
	isMacroGif bool,
	macroGifThumbnailURL string,
) (*bigquery.Query, error) {
	var query *bigquery.Query

	switch mutationType {
	case mutateTypeAdd:
		log.Printf(
			"add: name=%s, url: %s, thumbnail: %s, isGif: %t, gifThumbnail: %s",
			macroName,
			macroURL,
			macroThumbnailURL,
			isMacroGif,
			macroGifThumbnailURL,
		)

		query = client.Query(
			"INSERT  INTO `github-macros.macros.macros` (name, url, usages, reports, thumbnail, is_gif, gif_thumbnail) VALUES (@name, @url, 0, 0, @thumbnail, @is_gif, @gif_thumbnail)",
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
				Name:  "thumbnail",
				Value: macroThumbnailURL,
			},
			{
				Name:  "is_gif",
				Value: isMacroGif,
			},
			{
				Name:  "gif_thumbnail",
				Value: macroGifThumbnailURL,
			},
		}
	case mutateTypeUse:
		log.Printf("update usages of: %s", macroName)
		query = client.Query("UPDATE `github-macros.macros.macros` SET usages = usages + 1 WHERE name=@name")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: macroName,
			},
		}
	case mutateTypeReport:
		log.Printf("update reports of: %s", macroName)
		query = client.Query("UPDATE `github-macros.macros.macros` SET reports = reports + 1 WHERE name=@name")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: macroName,
			},
		}
	default:
		return nil, fmt.Errorf("error: unknown mutation type '%s'", mutationType)
	}

	return query, nil
}

func revalidateMacro(name string, client *bigquery.Client) error {
	ctx := context.Background()

	query := client.Query(
		"SELECT url, reports FROM `github-macros.macros.macros` WHERE name=@name",
	)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "name",
			Value: name,
		},
	}

	iter, err := runQuery(ctx, query)
	if err != nil {
		return err
	}

	type Row struct {
		URL     string
		Reports int
	}

	var row Row

	if err = iter.Next(&row); err != nil {
		return fmt.Errorf("error fetching reports: %v", err)
	}

	if row.Reports < reportsThreshold {
		return nil
	}

	if errCode := validateURL(row.URL, 1024*1024*10); errCode == Success {
		query = client.Query("UPDATE `github-macros.macros.macros` SET reports = 0 WHERE name=@name")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: name,
			},
		}

		_, err = runQuery(ctx, query)
	} else {
		query = client.Query("DELETE FROM `github-macros.macros.macros` WHERE name=@name")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: name,
			},
		}

		_, err = runQuery(ctx, query)
	}

	if err != nil {
		return fmt.Errorf("error while revalidating URL: %v", err)
	}

	return nil
}

func execMutate(r *http.Request) (ErrorCode, error) {
	if err := r.ParseForm(); err != nil {
		return TransientError, fmt.Errorf("error while parsing form: %v", err)
	}
	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "github-macros")

	if err != nil {
		return TransientError, fmt.Errorf("error while running bigquery.NewClient: %v", err)
	}
	defer client.Close()

	macroName := r.Form.Get("name")
	macroURL := r.Form.Get("url")
	macroThumbnailURL := r.Form.Get("thumbnail")
	isMacroGif := r.Form.Get("is_gif") == "true"
	macroGifThumbnailURL := r.Form.Get("gif_thumbnail")

	mutationType := r.Form.Get("type")

	if mutationType == mutateTypeAdd {
		if errCode := validateGithubURL(macroURL); errCode != Success {
			return errCode, nil
		}
		errCode, validationErr := validateNameAndURL(macroName, macroURL, client)
		if validationErr != nil {
			return TransientError, fmt.Errorf("error checking if input is valid: %v", validationErr)
		}

		if errCode != Success {
			return errCode, nil
		}

		if errCode := validateURL(macroThumbnailURL, cThumbnailMaxSize); errCode != Success {
			macroThumbnailURL = ""
		}

		if errCode := validateURL(macroGifThumbnailURL, cGifThumbnailMaxSize); errCode != Success {
			macroGifThumbnailURL = ""
		}
	}

	query, err := getMutationQuery(
		client,
		mutationType,
		macroName,
		macroURL,
		macroThumbnailURL,
		isMacroGif,
		macroGifThumbnailURL,
	)

	if err != nil {
		return TransientError, err
	}

	_, err = runQuery(ctx, query)

	if err != nil {
		return TransientError, fmt.Errorf("error while running query: %v", err)
	}

	if r.Form.Get("type") == mutateTypeReport {
		err = revalidateMacro(r.Form.Get("name"), client)
		if err != nil {
			return TransientError, fmt.Errorf("error while revalidate macro: %v", err)
		}
	}

	return Success, nil
}

func Mutate(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	errCode, err := execMutate(r)

	if err != nil {
		log.Println(err)
	}

	response, err := getErrorCodeResponse(errCode)
	if err != nil {
		log.Println(err)
	}

	_, err = fmt.Fprint(w, response)

	if err != nil {
		log.Fatalf("error writing response: %v", err)
	}
}
