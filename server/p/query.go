// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"cloud.google.com/go/bigquery"
)

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
		query = client.Query(`
			SELECT 
				name,
				github_url AS url,
				width,
				height,
				(CASE WHEN Usages.clicks is NULL THEN 0 ELSE Usages.clicks END) + (CASE WHEN Usages.directs is NULL THEN 0 ELSE Usages.directs END) as usages,
			FROM github-macros.macros.macros Macros
			LEFT JOIN github-macros.macros.usages Usages
			ON Macros.name = Usages.macro_name
			WHERE name LIKE @name
			ORDER BY usages, Macros.name DESC 
			LIMIT @limit
			OFFSET @offset
		`)
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
		query = client.Query("SELECT name, github_url AS url, width, height FROM `github-macros.macros.macros` WHERE name=@name")
		query.Parameters = []bigquery.QueryParameter{
			{
				Name:  "name",
				Value: queryText,
			},
		}
	case "", queryTypeSuggestion:
		log.Printf("suggestion: offset: %v", offset)
		query = client.Query(`
			SELECT
				name,
				github_url AS url,
				width,
				height,
				(CASE WHEN Usages.clicks is NULL THEN 0 ELSE Usages.clicks END) + (CASE WHEN Usages.directs is NULL THEN 0 ELSE Usages.directs END) as usages
			FROM github-macros.macros.macros Macros
			LEFT JOIN github-macros.macros.usages Usages
			ON Macros.name = Usages.macro_name
			ORDER BY usages, Macros.name DESC
			LIMIT @limit
			OFFSET @offset
		`)
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

	rows := getQueryResults(query)

	hasMore := len(rows) > resultsPerPage

	// remove the extra item we fetched just to verify if we have more
	if hasMore {
		rows = rows[:resultsPerPage]
	}

	isQueryGet := r.URL.Query().Get("type") == queryTypeGet

	responseMap := map[string]interface{}{
		"data": rows,
	}

	if !isQueryGet {
		responseMap["has_more"] = len(rows) > resultsPerPage
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
