package p

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/PuerkitoBio/goquery"
	"google.golang.org/api/iterator"
)

const (
	gMaxComments = 95
)

type GistRow struct {
	ID       string `bigquery:"id"`
	Comments int    `bigquery:"comments"`
}

func sendAPIRequest(apiURL, payload string) (map[string]interface{}, error) {
	body := strings.NewReader(payload)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, apiURL, body)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth("githubmacros", os.Getenv("GITHUB_TOKEN"))
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response map[string]interface{}

	if err := json.Unmarshal(respBytes, &response); err != nil {
		return nil, err
	}

	return response, nil
}

func queryAvailableGistID(client *bigquery.Client) (string, error) {
	query := client.Query("SELECT * FROM `github-macros.macros.gists` ORDER BY creation_time DESC LIMIT 1")
	it, err := runQuery(context.Background(), query)

	if err != nil {
		return "", err
	}

	var row GistRow

	if err := it.Next(&row); err != nil {
		if err == iterator.Done {
			return "", nil
		}

		return "", err
	}

	if row.Comments <= gMaxComments {
		return row.ID, nil
	}

	return "", nil
}

func addNewGist(client *bigquery.Client, gistID string) {
	query := client.Query(
		"INSERT INTO `github-macros.macros.gists` (id, comments, creation_time) VALUES (@id, 0, CURRENT_TIMESTAMP())",
	)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "id",
			Value: gistID,
		},
	}

	if _, err := runQuery(context.Background(), query); err != nil {
		log.Printf("failed to add new gist: %v", err)
	}
}

func updateComments(client *bigquery.Client, gistID string) {
	query := client.Query(
		"UPDATE `github-macros.macros.gists` SET comments = comments + 1 WHERE id=@id",
	)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "id",
			Value: gistID,
		},
	}

	if _, err := runQuery(context.Background(), query); err != nil {
		log.Printf("failed to update comments: %v", err)
	}
}

func getGistID(client *bigquery.Client) (string, error) {
	gistID, err := queryAvailableGistID(client)
	if err != nil {
		return "", err
	}

	if gistID != "" {
		return gistID, nil
	}

	res, err := sendAPIRequest(
		"https://api.github.com/gists",
		fmt.Sprintf(
			`{"public": true, "files":{%q: {"content": "created on %s"}}}`,
			strconv.FormatInt(time.Now().UnixNano(), 10), //nolint: gomnd
			time.Now().String(),
		),
	)

	if err != nil {
		return "", err
	}

	gistID, ok := res["id"].(string)

	if !ok {
		return "", errors.New("missing gist ID")
	}

	addNewGist(client, gistID)

	return gistID, nil
}

func commentOnGist(client *bigquery.Client, gistID, comment string) (int64, error) {
	res, err := sendAPIRequest(
		fmt.Sprintf("https://api.github.com/gists/%s/comments", gistID),
		fmt.Sprintf(`{"body":%q}`, comment),
	)

	if err != nil {
		return 0, err
	}

	if _, ok := res["id"]; !ok {
		return 0, fmt.Errorf("unable to find the id of the new comment")
	}

	updateComments(client, gistID)

	return int64(res["id"].(float64)), nil
}

func getGist(gistID string) ([]byte, error) {
	resp, err := sendHTTPGetRequest(
		fmt.Sprintf("https://gist.github.com/githubmacros/%s", gistID),
	)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func GetGithubImage(client *bigquery.Client, imageURL string) (string, error) {
	gistID, err := getGistID(client)
	if err != nil {
		return "", err
	}

	payload := fmt.Sprintf("![ghm](%s)", imageURL)

	commentID, err := commentOnGist(client, gistID, payload)
	if err != nil {
		return "", err
	}

	gist, err := getGist(gistID)
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(gist))
	if err != nil {
		return "", err
	}

	githubImage := ""

	doc.Find(fmt.Sprintf("#gistcomment-%d img", commentID)).Each(func(i int, s *goquery.Selection) {
		src, ok := s.Attr("src")
		if ok {
			githubImage = src
		}
	})

	if githubImage == "" {
		return "", errors.New("unable to locate github image")
	}

	return githubImage, nil
}
