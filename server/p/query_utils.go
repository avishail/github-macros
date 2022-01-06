package p

import (
	"context"
	"log"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

func getQueryResults(query *bigquery.Query) []*MacroRow {
	ctx := context.Background()
	iter, err := runQuery(ctx, query)

	if err != nil {
		log.Panicf("failed to run query: %v", err)
	}

	rows := []*MacroRow{}

	for {
		var curRow MacroRow
		err = iter.Next(&curRow)

		if err == iterator.Done {
			break
		}

		if err != nil {
			log.Panicf("failed to read query result: %v", err)
		}

		rows = append(rows, &curRow)
	}

	return rows
}
