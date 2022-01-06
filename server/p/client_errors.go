// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"cloud.google.com/go/bigquery"
)

func saveLog(r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Panicf("error while parsing form: %v", err)
	}
	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "github-macros")

	if err != nil {
		log.Panicf("error while running bigquery.NewClient: %v", err)
	}
	defer client.Close()

	version := r.Form.Get("version")
	stacktrace := r.Form.Get("stacktrace")

	if version == "" {
		log.Panic("missing version parameter")
	}

	if stacktrace == "" {
		log.Panic("missing stacktrace parameter")
	}

	query := client.Query(`
		INSERT INTO github-macros.macros.client_errors (id, client_version, stacktrace, timestamp) 
		VALUES (GENERATE_UUID(), @client_version, @stacktrace, CURRENT_TIMESTAMP())
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "client_version",
			Value: version,
		},
		{
			Name:  "stacktrace",
			Value: stacktrace,
		},
	}

	_, err = runQuery(ctx, query)

	if err != nil {
		log.Panicf("error while running query: %v", err)
	}
}

func ClientErrors(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	saveLog(r)

	_, err := fmt.Fprint(w, "OK")

	if err != nil {
		log.Panicf("error writing response: %v", err)
	}
}
