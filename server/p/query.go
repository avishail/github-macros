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

type row struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

const queryTypeSearch = "search"
const queryTypeGet = "get"
const queryTypeSuggestion = "suggestion"
const resultsPerPage = 20

func getQuery(r *http.Request, client *bigquery.Client) (*bigquery.Query, error) {
	queryText := r.URL.Query().Get("text")
	page, err := strconv.Atoi(r.URL.Query().Get("page"))

	if err != nil {
		page = 0
	}

	offset := page * resultsPerPage

	var query *bigquery.Query

	switch r.URL.Query().Get("type") {
	case queryTypeSearch:
		log.Printf("search: %s, offset: %v", queryText, offset)
		query = client.Query(
			"SELECT name, url FROM `github-macros.macros.macros` WHERE name LIKE @name ORDER BY usages, name DESC LIMIT @limit OFFSET @offset",
		)
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: "%" + queryText + "%",
			},
			{
				Name:  "limit",
				Value: resultsPerPage + 1,
			},
			{
				Name:  "offset",
				Value: offset,
			},
		}
	case queryTypeGet:
		log.Printf("get: %s", queryText)
		query = client.Query("SELECT * FROM `github-macros.macros.macros` WHERE name IN UNNEST(@list)")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "list",
				Value: strings.Split(queryText, ","),
			},
		}
	case "", queryTypeSuggestion:
		log.Printf("suggestion: offset: %v", offset)
		query = client.Query("SELECT * FROM `github-macros.macros.macros` ORDER BY usages, name DESC LIMIT @limit OFFSET @offset")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "limit",
				Value: resultsPerPage + 1,
			},
			{
				Name:  "offset",
				Value: offset,
			},
		}
	default:
		return nil, fmt.Errorf("unknown query type: %s", r.URL.Query().Get("type"))
	}

	return query, nil
}

func execQuery(r *http.Request) (string, error) {
	ctx := context.Background()

	client, err := bigquery.NewClient(ctx, "github-macros")
	if err != nil {
		return "", fmt.Errorf("bigquery.NewClient: %v", err)
	}
	defer client.Close()

	query, err := getQuery(r, client)
	if err != nil {
		return "", fmt.Errorf("getQuery: %v", err)
	}

	iter, err := runQuery(ctx, query)
	if err != nil {
		return "", fmt.Errorf("runQuery: %v", err)
	}

	rows := []row{}

	isQueryGet := r.URL.Query().Get("type") == queryTypeGet

	for {
		var curRow row
		err = iter.Next(&curRow)

		if err == iterator.Done {
			break
		}

		if err != nil {
			return "", fmt.Errorf("error iterating through results: %v", err)
		}

		rows = append(rows, curRow)

		if len(rows) == resultsPerPage && !isQueryGet {
			break
		}
	}

	responseMap := map[string]interface{}{
		"data": rows,
	}

	if !isQueryGet {
		responseMap["has_more"] = iter.TotalRows > resultsPerPage
	}

	response, err := json.Marshal(responseMap)
	if err != nil {
		return "", fmt.Errorf("error marshaling results: %v", err)
	}

	return string(response), nil
}

func Query(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	response, err := execQuery(r)

	if err != nil {
		log.Fatalf("error executing query: %v", err)
	}

	_, err = fmt.Fprint(w, response)

	if err != nil {
		log.Fatalf("error writing response: %v", err)
	}
}
