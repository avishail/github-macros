package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

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
			RawQuery: "?type=suggestion&page=0",
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
	t := time.Now().Unix()
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"name=example&url=https://www.researchgate.net/profile/Tama-Leaver/publication/301644273/figure/fig2/AS:362433639141377@1463422321730/Example-of-the-One-does-not-simply-meme-created-with-MemeGenerator.png&github_url=https://camo.githubusercontent.com/bdafd73963fbf563a5beba36eda5644daf04987f30350cfa710a2fba3b5b120f/68747470733a2f2f7777772e7265736561726368676174652e6e65742f70726f66696c652f54616d612d4c65617665722f7075626c69636174696f6e2f3330313634343237332f6669677572652f666967322f41533a33363234333336333931343133373740313436333432323332313733302f4578616d706c652d6f662d7468652d4f6e652d646f65732d6e6f742d73696d706c792d6d656d652d637265617465642d776974682d4d656d6547656e657261746f722e706e67",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Add(&ResponseWriter{}, r)
	fmt.Println("Total time: ", time.Now().Unix()-t)
}

func testUsage() {
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"name=magic",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Usage(&ResponseWriter{}, r)
}

func testReportMutation() {
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"name=magic",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Report(&ResponseWriter{}, r)
}

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
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// f, err := os.Open("/workspaces/github-macros/server/test.jpeg")
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// defer f.Close()
	// co, er := jpeg.DecodeConfig(f)
	// fmt.Println(co, er)

	// image, tt, err := image.DecodeConfig(f)
	// fmt.Println(err)
	// fmt.Println(tt)
	// fmt.Println(image)

	// testSuggestionQuery()

	// p.PublishNewMacroMessage("macroName123")

	//testPreprocessing()

	//testSearchQuery()
	//testGetQuery()
	testSuggestionQuery()
	// testUsage()
	//testAddMutation()
	//testUsage()
	//testReportMutation()

	// ctx := context.Background()

	// client, err := bigquery.NewClient(ctx, "github-macros")
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// defer client.Close()

	// ghi, err := p.GetGithubImages(client, "https://i.imgur.com/lqKlotB.png")
	// fmt.Println(ghi, err)

	// query := client.Query(`
	// 	INSERT INTO github-macros.macros.reports (macro_name, reports) VALUES ("abc", 0);
	// 	INSERT INTO github-macros.macros.reports (macro_name, reports) VALUES ("def", 0);
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
