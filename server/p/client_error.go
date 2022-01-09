// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"cloud.google.com/go/bigquery"
)

const (
	cErrorTypeJS  = "js"
	cErrorTypeNet = "net"
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
	if version == "" {
		log.Panic("missing version parameter")
	}

	stacktrace := r.Form.Get("stacktrace")
	if stacktrace == "" {
		log.Panic("missing stacktrace parameter")
	}

	errType := r.Form.Get("type")
	if errType != cErrorTypeJS && errType != cErrorTypeNet {
		log.Panic("missing or wrong type parameter")
	}

	query := client.Query(`
		INSERT INTO github-macros.macros.client_errors (id, client_version, type, stacktrace, timestamp) 
		VALUES (GENERATE_UUID(), @client_version, @error_type, @stacktrace, CURRENT_TIMESTAMP())
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "client_version",
			Value: version,
		},
		{
			Name:  "error_type",
			Value: errType,
		},
		{
			Name:  "stacktrace",
			Value: stacktrace,
		},
		{
			Name:  "type",
			Value: errType,
		},
	}

	_, err = runQuery(ctx, query)

	if err != nil {
		log.Panicf("error while running query: %v", err)
	}
}

func ClientError(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	saveLog(r)

	_, err := fmt.Fprint(w, "OK")

	if err != nil {
		log.Panicf("error writing response: %v", err)
	}
}
