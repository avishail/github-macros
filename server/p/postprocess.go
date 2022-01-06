// Package p contains a Pub/Sub Cloud Function.
package p

import (
	"context"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/functions/metadata"
)

// PubSubMessage is the payload of a Pub/Sub event. Please refer to the docs for
// additional information regarding Pub/Sub events.
type PubSubMessage struct {
	Data []byte `json:"data"`
}

func deleteMacro(client *bigquery.Client, macroName string) error {
	query := client.Query("DELETE FROM github-macros.macros.macros WHERE name=@name")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "name",
			Value: macroName,
		},
	}

	_, err := runQuery(context.Background(), query)

	return err
}

func PostProcess(ctx context.Context, m PubSubMessage) error {
	macroName := string(m.Data)

	meta, err := metadata.FromContext(ctx)
	if err != nil {
		// Assume an error on the function invoker and try again.
		return fmt.Errorf("metadata.FromContext: %v", err)
	}

	client, clientErr := bigquery.NewClient(context.Background(), "github-macros")

	// Ignore events that are too old.
	expiration := meta.Timestamp.Add(1 * time.Minute)
	if time.Now().After(expiration) {
		log.Printf("timeout while trying to process %s", macroName)

		if clientErr != nil {
			if delErr := deleteMacro(client, macroName); delErr != nil {
				log.Printf("deleteMacro error: %v", delErr)
			}
		}

		return nil
	}

	if clientErr != nil {
		return fmt.Errorf("bigquery.NewClient: %v", err)
	}

	query := client.Query("SELECT * FROM `github-macros.macros.macros` WHERE name=@name")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "name",
			Value: macroName,
		},
	}

	rows := getQueryResults(query)
	if len(rows) == 0 {
		log.Printf("failed to locate the new macro %s", macroName)
		return nil
	}

	macro := rows[0]

	postprocessNewMacro(client, macroName, macro.URL, macro.URLSize, macro.IsGif, false)

	return nil
}
