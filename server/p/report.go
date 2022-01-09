// Package p contains an HTTP Cloud Function.
package p

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"cloud.google.com/go/bigquery"
)

const cReportsThreshold = 50

func createNewReportsEntryIfNotExist(client *bigquery.Client, macroName string) {
	query := client.Query(`
		INSERT INTO github-macros.macros.reports (macro_name, reports)
		SELECT @macro_name, 1 FROM (SELECT 1) 
		LEFT JOIN github-macros.macros.reports
		ON macro_name = @macro_name
		WHERE macro_name IS NULL
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "macro_name",
			Value: macroName,
		},
	}

	if _, err := runQuery(context.Background(), query); err != nil {
		log.Panicf("failed to create reports entry: %v", err)
	}
}

func incrementNumberOfReports(client *bigquery.Client, macroName string) {
	query := client.Query("UPDATE `github-macros.macros.reports` SET reports = reports + 1 WHERE macro_name=@macro_name")
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "macro_name",
			Value: macroName,
		},
	}

	if _, err := runQuery(context.Background(), query); err != nil {
		log.Panicf("incrementNumberOfReports: %v", err)
	}
}

func revalidateMacro(client *bigquery.Client, macroName, macroURL string) {
	_, err := getImageConfig(macroURL)

	var query *bigquery.Query

	if err != nil {
		query = client.Query(`
			DELETE FROM github-macros.macros.macros WHERE name=@macro_name;
			DELETE FROM github-macros.macros.reports WHERE macro_name=@macro_name;
			DELETE FROM github-macros.macros.usages WHERE macro_name=@macro_name;
		`)
	} else {
		query = client.Query("UPDATE `github-macros.macros.reports` SET reports = 0 WHERE macro_name=@macro_name")
	}

	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "macro_name",
			Value: macroName,
		},
	}

	if _, err = runQuery(context.Background(), query); err != nil {
		log.Panicf("revalidateMacro: %v", err)
	}
}

func getURLAndReports(client *bigquery.Client, macroName string) (string, int64, bool) {
	query := client.Query(`
		SELECT M.url, R.reports FROM github-macros.macros.macros M
		LEFT JOIN github-macros.macros.reports R 
		ON M.name = R.macro_name
		WHERE M.name=@name
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "name",
			Value: macroName,
		},
	}

	iter, err := runQuery(context.Background(), query)
	if err != nil {
		log.Panicf("failed to increment number of reports: %v", err)
	}

	var row struct {
		URL     string
		Reports bigquery.NullInt64
	}

	if err = iter.Next(&row); err != nil {
		log.Panicf("failed to read reports: %v", err)
	}

	return row.URL, row.Reports.Int64, row.Reports.Valid
}

func Report(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	if err := r.ParseForm(); err != nil {
		log.Panicf("failed to parse form: %v", err)
	}

	macroName := r.Form.Get("name")
	if macroName == "" {
		log.Panicf("missing marco name")
	}

	ctx := context.Background()
	client, err := bigquery.NewClient(ctx, "github-macros")

	if err != nil {
		log.Panicf("failed to create bigquery client: %v", err)
	}
	defer client.Close()

	macroURL, reports, exist := getURLAndReports(client, macroName)
	if !exist {
		createNewReportsEntryIfNotExist(client, macroName)
	} else if reports < cReportsThreshold {
		incrementNumberOfReports(client, macroName)
	} else {
		revalidateMacro(client, macroName, macroURL)
	}

	_, err = fmt.Fprint(w, "OK")

	if err != nil {
		log.Panicf("failed to write response: %v", err)
	}
}
