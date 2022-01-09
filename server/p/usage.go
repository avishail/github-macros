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
	cClickTrigger  = "click"
	cDirectTrigger = "direct"
)

func createNewUsagesEntryIfNotExist(client *bigquery.Client, macroName string) error {
	query := client.Query(`
		INSERT INTO github-macros.macros.usages (macro_name, clicks, directs)
		SELECT @macro_name, 0, 0 FROM (SELECT 1)
		LEFT JOIN github-macros.macros.macros M
		ON M.name = @macro_name
		LEFT JOIN github-macros.macros.usages U
		ON U.macro_name = @macro_name
		WHERE M.name IS NOT NULL AND U.macro_name IS NULL
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

func Usage(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	if err := r.ParseForm(); err != nil {
		log.Panicf("failed to parse form: %v", err)
	}

	macroName := r.Form.Get("name")
	if macroName == "" {
		log.Panicf("macro name is missing")
	}

	trigger := r.Form.Get("trigger")
	if trigger != cClickTrigger && trigger != cDirectTrigger {
		log.Panicf("macro name is missing")
	}

	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "github-macros")

	if err != nil {
		log.Panicf("failed to create bigquery client: %v", err)
	}

	defer func() {
		if err = client.Close(); err != nil {
			log.Printf("client.Close(): %v", err)
		}
	}()

	if err = createNewUsagesEntryIfNotExist(client, macroName); err != nil {
		log.Panicf("createNewUsagesEntryIfNotExist: %v", err)
	}

	var query *bigquery.Query

	if trigger == cClickTrigger {
		query = client.Query("UPDATE `github-macros.macros.usages` SET clicks = clicks + 1 WHERE macro_name=@macro_name")
	} else {
		query = client.Query("UPDATE `github-macros.macros.usages` SET directs = directs + 1 WHERE macro_name=@macro_name")
	}

	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "macro_name",
			Value: macroName,
		},
	}

	if _, err = runQuery(ctx, query); err != nil {
		log.Panicf("failed to increment number of usages: %v", err)
	}

	_, err = fmt.Fprint(w, "OK")

	if err != nil {
		log.Panicf("failed to write response: %v", err)
	}
}
