// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

const mutateTypeAdd = "add"
const mutateTypeUse = "use"
const mutateTypeReport = "report"

const reportsThreshold = 10

var supportedTypes = map[string]bool{
	"image/gif":  true,
	"image/png":  true,
	"image/bmp":  true,
	"image/jpeg": true,
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

func validateURL(url string) (isValid bool, errorMessage string) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, http.NoBody)

	if err != nil {
		isValid = false
		errorMessage = "Invalid URL."

		return
	}

	req.Header.Add("Range", "bytes=0-512")

	var client http.Client

	resp, err := client.Do(req)

	if err != nil {
		isValid = false
		errorMessage = "Unable to process image from URL"

		return
	}

	if getFileSize(resp) > 1024*1024*1.5 {
		isValid = false
		errorMessage = `Image exceeds 1.5Mb. Please reduce its size and try again. You can use <a target="_blank" href="https://ezgif.com/optimize">this</a> website to do it`

		return
	}

	body := make([]byte, 512)
	_, err = resp.Body.Read(body)

	if err != nil {
		isValid = false
		errorMessage = "Unable to process image from URL"

		return
	}

	str := http.DetectContentType(body)

	if _, ok := supportedTypes[str]; !ok {
		isValid = false
		errorMessage = "Unknown image format. Only jpeg, gif, png and bmp are supported"

		return
	}

	return true, ""
}

func validateInput(r *http.Request, client *bigquery.Client) (isValid bool, errorMessage string, err error) {
	if r.Form.Get("type") != mutateTypeAdd {
		isValid = true
		return
	}

	name := r.Form.Get("name")
	macroURL := r.Form.Get("url")

	if strings.Contains(name, " ") {
		isValid = false
		errorMessage = "Name can't contain spaces"

		return
	}

	isValid, errorMessage = validateURL(macroURL)

	if !isValid {
		return
	}

	isExist, err := isMacroExist(name, client)

	if err != nil {
		err = fmt.Errorf("error checking if macro exist: %v", err)

		return
	}

	if isExist {
		errorMessage = fmt.Sprintf("The name '%s' already exist", name)
	}

	isValid = !isExist

	return isValid, errorMessage, nil
}

func getMutationQuery(r *http.Request, client *bigquery.Client) (*bigquery.Query, error) {
	var query *bigquery.Query

	name := r.Form.Get("name")

	switch r.Form.Get("type") {
	case mutateTypeAdd:
		macroURL := r.Form.Get("url")

		log.Printf("add: name=%s, URL: %v", name, macroURL)

		query = client.Query(
			"INSERT  INTO `github-macros.macros.macros` (name, url, usages, reports) VALUES (@name, @url, 0, 0)",
		)
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: name,
			},
			{
				Name:  "url",
				Value: macroURL,
			},
		}
	case mutateTypeUse:
		log.Printf("update usages of: %s", name)
		query = client.Query("UPDATE `github-macros.macros.macros` SET usages = usages + 1 WHERE name=@name")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: name,
			},
		}
	case mutateTypeReport:
		log.Printf("update reports of: %s", name)
		query = client.Query("UPDATE `github-macros.macros.macros` SET reports = reports + 1 WHERE name=@name")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: name,
			},
		}
	default:
		return nil, fmt.Errorf("error: unknown mutation type '%s'", r.URL.Query().Get("type"))
	}

	return query, nil
}

func getResponse(errorMessage string) (string, error) {
	responseMap := map[string]interface{}{
		"success": errorMessage == "",
	}

	if errorMessage != "" {
		responseMap["error_message"] = errorMessage
	}

	response, err := json.Marshal(responseMap)

	if err != nil {
		return "", fmt.Errorf("error while marshaling response: %v", err)
	}

	return string(response), nil
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

	isValid, _ := validateURL(row.URL)

	if isValid {
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

func execMutate(r *http.Request) (string, error) {
	if err := r.ParseForm(); err != nil {
		return "", fmt.Errorf("error while parsing form: %v", err)
	}
	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "github-macros")

	if err != nil {
		return "", fmt.Errorf("error while running bigquery.NewClient: %v", err)
	}
	defer client.Close()

	isValid, errorMessage, err := validateInput(r, client)

	if err != nil {
		return "", fmt.Errorf("error checking if input is valid: %v", err)
	}

	if !isValid {
		return getResponse(errorMessage)
	}

	query, err := getMutationQuery(r, client)

	if err != nil {
		return "", err
	}

	_, err = runQuery(ctx, query)

	if err != nil {
		return "", fmt.Errorf("error while running query: %v", err)
	}

	if r.Form.Get("type") == mutateTypeReport {
		err = revalidateMacro(r.Form.Get("name"), client)
		if err != nil {
			return "", fmt.Errorf("error while revalidate macro: %v", err)
		}
	}

	return getResponse("")
}

func Mutate(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	response, err := execMutate(r)

	if err != nil {
		log.Fatal(err)
	}

	_, err = fmt.Fprint(w, response)

	if err != nil {
		log.Fatalf("error writing response: %v", err)
	}
}
