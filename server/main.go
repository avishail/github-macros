package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"cloud.google.com/go/bigquery"
	"github.com/avishail/github-macros/server/p"
	"github.com/joho/godotenv"
)

type ResponseWriter struct {
}

func (rw *ResponseWriter) Header() http.Header {
	return http.Header{}
}

func (rw *ResponseWriter) Write(buffer []byte) (int, error) {
	log.Println(string(buffer))
	return len(buffer), nil
}

func (rw *ResponseWriter) WriteHeader(statusCode int) {}

func testSearchQuery() {
	r := &http.Request{
		URL: &url.URL{
			RawQuery: "type=search&text=dog",
		},
	}

	p.Query(&ResponseWriter{}, r)
}

func testGetQuery() {
	r := &http.Request{
		URL: &url.URL{
			RawQuery: "type=get&text=wowdogreview",
		},
	}

	p.Query(&ResponseWriter{}, r)
}

func testSuggestionQuery() {
	r := &http.Request{
		URL: &url.URL{
			RawQuery: "type=suggestion",
		},
	}

	p.Query(&ResponseWriter{}, r)
}

// func testPreprocessing() {
// 	r := &http.Request{
// 		URL: &url.URL{
// 			RawQuery: "name=nonono2&url=https://camo.githubusercontent.com/0fabfd20960f454a010c9761f4ed4edf340c0c87a4302d95270a26471daf6812/68747470733a2f2f6d65646961342e67697068792e636f6d2f6d656469612f764d4e6f4b4b7a6e4f72554a692f67697068792e6769663f6369643d373930623736313164366236336563356633643738613633616434633032343139303064393834353538313766366233267269643d67697068792e6769662663743d67",
// 			//RawQuery: "name=blablabla&url=https://upload.wikimedia.org/wikipedia/commons/thumb/e/ea/BB61_USS_Iowa_BB61_broadside_USN.jpg/1280px-BB61_USS_Iowa_BB61_broadside_USN.jpg",
// 		},
// 	}
// 	p.Preprocess(&ResponseWriter{}, r)
// }

func testAddMutation() {
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"name=magic&url=https://c.tenor.com/wn2_Qq6flogAAAAM/magical-magic.gif",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Add(&ResponseWriter{}, r)
}

func testUsage() {
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"name=nonono",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Usage(&ResponseWriter{}, r)
}

/*
func testReportMutation() {
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"type=report&name=blabla",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Mutate(&ResponseWriter{}, r)
}
*/

func runQuery(ctx context.Context, query *bigquery.Query) (*bigquery.RowIterator, error) {
	job, err := query.Run(ctx)

	if err != nil {
		return nil, err
	}

	status, err := job.Wait(ctx)
	if err != nil {
		return nil, err
	}

	if status.Err() != nil {
		return nil, status.Err()
	}

	iter, err := job.Read(ctx)
	if err != nil {
		return nil, err
	}

	return iter, nil
}

func main() {
	// m := map[string]interface{}{}

	// a, _ := m["a"].(float64)
	// fmt.Println(a)

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// p.PublishNewMacroMessage("macroName123")

	//testPreprocessing()

	//testSearchQuery()
	//testGetQuery()
	//testSuggestionQuery()

	testAddMutation()
	//testUsage()
	//testReportMutation()

	// ctx := context.Background()

	// client, err := bigquery.NewClient(ctx, "github-macros")
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// defer client.Close()

	// query := client.Query(`
	// 	INSERT INTO github-macros.macros.reports (macro_name, reports)
	// 	SELECT @macro_name, 0 FROM (SELECT 1)
	// 	LEFT JOIN github-macros.macros.reports
	// 	ON macro_name = @macro_name
	// 	WHERE macro_name IS NULL
	// `)
	// query.Parameters = []bigquery.QueryParameter{
	// 	{
	// 		Name:  "macro_name",
	// 		Value: "cool_macro",
	// 	},
	// }

	// fmt.Println(runQuery(ctx, query))

	// githubImages, err := p.GetGithubImages(client, []string{"https://upload.wikimedia.org/wikipedia/commons/e/ea/BB61_USS_Iowa_BB61_broadside_USN.jpg", "https://www.researchgate.net/profile/Kirsi-Kauppinen/publication/305882572/figure/fig2/AS:391865942724609@1470439529117/A-doge-meme-about-linguistics.png"})

	// fmt.Println(githubImages, err)
}
