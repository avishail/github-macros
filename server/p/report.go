// Package p contains an HTTP Cloud Function.
package p //nolint

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"cloud.google.com/go/bigquery"
)

func Report(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	if err := r.ParseForm(); err != nil {
		log.Panicf("failed to parse form: %v", err)
	}

	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "github-macros")

	if err != nil {
		log.Panicf("failed to create bigquery client: %v", err)
	}
	defer client.Close()

	macroName := r.Form.Get("name")

	query := client.Query("UPDATE `github-macros.macros.reports` SET reports = reports + 1 WHERE macro_name=@macro_name")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "macro_name",
			Value: macroName,
		},
	}

	if _, err = runQuery(ctx, query); err != nil {
		log.Panicf("failed to increment number of reports: %v", err)
	}

	_, err = fmt.Fprint(w, "OK")

	if err != nil {
		log.Panicf("failed to write response: %v", err)
	}
}
