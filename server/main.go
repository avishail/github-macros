package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

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
			RawQuery: "type=search&text=ship",
		},
	}

	p.Query(&ResponseWriter{}, r)
}

func testGetQuery() {
	r := &http.Request{
		URL: &url.URL{
			RawQuery: "type=get&text=battleshipit,nonono",
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

func testAddMutation() {
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"type=add&name=nonono&url=https://media4.giphy.com/media/14ooolmDKfgrO8/giphy.gif?cid=ecf05e47nxyksedynzqvvxvf0ut55r38ak4ujw3agydaozjn&rid=giphy.gif&ct=g",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Mutate(&ResponseWriter{}, r)
}

func testUseMutation() {
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"type=use&name=nonono",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Mutate(&ResponseWriter{}, r)
}

func testReportMutation() {
	r := &http.Request{
		Method: http.MethodPost,
		Body: io.NopCloser(
			strings.NewReader(
				"type=report&name=nonono",
			),
		),
		Header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		},
	}

	p.Mutate(&ResponseWriter{}, r)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	testSearchQuery()
	testGetQuery()
	testSuggestionQuery()

	testAddMutation()
	testUseMutation()
	testReportMutation()
}
