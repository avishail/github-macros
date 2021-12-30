// Package p contains an HTTP Cloud Function.
package p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/google/uuid"
)

func processGif(responseMap map[string]interface{}, macroURL string, fileSize int64) {
	response, err := sendHTTPRequest(http.MethodGet, macroURL)
	if err != nil {
		log.Printf("unable to read GIF '%s' : %v", macroURL, err)
		return
	}

	client, err := newStorageClient()
	if err != nil {
		log.Printf("failed to create storage client: %v", err)
		return
	}

	defer closeStorageClient(client)

	firstFrameURL, err := getGifFirstFrameURL(client, bytes.NewReader(response))
	if err == nil {
		responseMap["thumbnail"] = firstFrameURL
	} else {
		log.Printf("failed to save GIF first frame: %v", err)
	}

	if fileSize < cGifThumbnailMaxSize {
		responseMap["resized_gif"] = macroURL
		return
	}

	resizedGifURL, err := getResizedGifURL(client, bytes.NewReader(response))
	if err == nil {
		responseMap["resized_gif"] = resizedGifURL
	} else {
		log.Printf("failed to get resizes GIF: %v", err)
	}
}

func getResizedGifURL(client *storage.Client, reader io.Reader) (string, error) {
	fileName := uuid.NewString() + ".gif"

	fileWriter := getNewPublicFileWriter(client, fileName)

	if err := resizeGif(reader, fileWriter); err != nil {
		return "", err
	}

	if err := fileWriter.Close(); err != nil {
		return "", err
	}

	return getPublicFileURL(fileName), nil
}

// Decode reads and analyzes the given reader as a GIF image
func getGifFirstFrameURL(client *storage.Client, reader io.Reader) (firstFrameURL string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error while decoding: %s", r)
		}
	}()

	firstFrame, err := getGifFirstFrame(reader)
	if err != nil {
		return "", nil
	}

	resizedImage := resizeImage(firstFrame)

	filePath := uuid.NewString() + ".jpeg"

	if err := writeNewPublicFile(
		client,
		filePath,
		func(w io.Writer) error {
			return jpeg.Encode(w, resizedImage, &jpeg.Options{Quality: 80})
		},
	); err != nil {
		return "", err
	}

	return getPublicFileURL(filePath), nil
}

func getCompressedImageURL(macroURL string) (string, error) {
	u, _ := url.Parse("https://api.resmush.it/ws.php")
	q := u.Query()
	q.Set("img", macroURL)
	q.Set("qlty", "50")
	u.RawQuery = q.Encode()

	response, err := sendHTTPRequest(http.MethodGet, u.String())

	if err != nil {
		return "", nil
	}

	compressionResult := map[string]interface{}{}

	if err := json.Unmarshal(response, &compressionResult); err != nil {
		return "", fmt.Errorf("failed to parse image compression API response: %v", err)
	}

	compressed, ok := compressionResult["dest"].(string)

	if !ok {
		return "", fmt.Errorf("unable to find compressed image")
	}

	return compressed, nil
}

func getImageThumbnailURL(macroURL string) (string, error) {
	client, err := newStorageClient()
	if err != nil {
		return "", fmt.Errorf("failed to create storage client: %v", err)
	}

	defer closeStorageClient(client)

	imgBuf, err := sendHTTPRequest(http.MethodGet, macroURL)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(imgBuf))
	if err != nil {
		return "", fmt.Errorf("unable to decode image: %v", err)
	}

	resizedImg := resizeImage(img)

	filePath := uuid.NewString() + ".jpeg"

	if err := writeNewPublicFile(
		client,
		filePath,
		func(w io.Writer) error {
			return jpeg.Encode(w, resizedImg, &jpeg.Options{Quality: 80})
		},
	); err != nil {
		return "", err
	}

	return getPublicFileURL(filePath), nil
}

func processImage(responseMap map[string]interface{}, macroURL string, fileSize int64) {
	if fileSize <= cThumbnailMaxSize {
		responseMap["thumbnail"] = macroURL
		return
	}

	var compressedImage string

	compressedImage, err := getCompressedImageURL(macroURL)

	if err != nil {
		compressedImage, err = getCompressedImageURL(macroURL)
	}

	if err == nil {
		responseMap["thumbnail"] = compressedImage
		return
	}

	log.Printf("unable to compress image using API: %v", err)

	thumbnailURL, err := getImageThumbnailURL(macroURL)

	if err != nil {
		log.Printf("unable to create thumbnail: %v", err)
	}

	responseMap["thumbnail"] = thumbnailURL
}

func Preprocess(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")

	macroName := r.URL.Query().Get("name")
	macroURL := r.URL.Query().Get("url")

	client, err := bigquery.NewClient(context.Background(), "github-macros")

	if err != nil {
		log.Fatalf("error while running bigquery.NewClient: %v", err)
	}
	defer client.Close()

	errCode, err := validateNameAndURL(macroName, macroURL, client)
	if err != nil {
		log.Fatalf("error checking validity of name and URL: %v", err)
	}

	if errCode != Success {
		writeErrorCodeResponse(w, errCode)
		return
	}

	fileSize, fileType, errCode := getFileSizeAndType(macroURL)

	if errCode != Success {
		writeErrorCodeResponse(w, errCode)
		return
	}

	responseMap := map[string]interface{}{
		"url": macroURL,
	}

	if isGif(fileType) {
		processGif(responseMap, macroURL, fileSize)
	} else {
		processImage(responseMap, macroURL, fileSize)
	}

	response, err := json.Marshal(responseMap)
	if err != nil {
		log.Fatalf("error marshaling results: %v", err)
	}

	writeResponse(w, string(response))
}
