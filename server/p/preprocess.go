// Package p contains an HTTP Cloud Function.
package p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/url"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
)

func processGif(macroURL string, fileSize int64) (string, int64, string, int64, error) {
	response, err := sendHTTPGetRequest(macroURL)
	if err != nil {
		return "", 0, "", 0, fmt.Errorf("unable to read GIF '%s' : %v", macroURL, err)
	}

	client, err := newStorageClient()
	if err != nil {
		return "", 0, "", 0, fmt.Errorf("failed to create storage client: %v", err)
	}

	defer func() {
		if closeErr := closeStorageClient(client); closeErr != nil {
			log.Printf("failed to close storage client: %v", closeErr)
		}
	}()

	firstFrameURL, firstFrameSize, err := getGifFirstFrameURL(client, bytes.NewReader(response))
	if err != nil {
		return "", 0, "", 0, fmt.Errorf("failed to save GIF first frame: %v", err)
	}

	if fileSize < cGifThumbnailSize {
		return firstFrameURL, firstFrameSize, macroURL, fileSize, nil
	}

	resizedGifURL, resizedGifSize, err := getResizedGifURL(client, bytes.NewReader(response))
	if err != nil {
		log.Printf("failed to get resizes GIF: %v", err)
	}

	if resizedGifSize > fileSize {
		log.Printf("resized gif is bigger than the original gif: %s", macroURL)
		resizedGifURL = ""
		resizedGifSize = 0
	}

	return firstFrameURL, firstFrameSize, resizedGifURL, resizedGifSize, nil
}

func getResizedGifURL(client *storage.Client, reader io.Reader) (string, int64, error) {
	fileName := uuid.NewString() + ".gif"

	fileWriter := getNewPublicFileWriter(client, fileName)

	if err := resizeGif(reader, fileWriter); err != nil {
		return "", 0, err
	}

	if err := fileWriter.Close(); err != nil {
		return "", 0, err
	}

	return getPublicFileURL(fileName), fileWriter.GetTotalBytes(), nil
}

// Decode reads and analyzes the given reader as a GIF image
func getGifFirstFrameURL(client *storage.Client, reader io.Reader) (firstFrameURL string, firstFrameSize int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error while decoding: %s", r)
		}
	}()

	firstFrame, err := getGifFirstFrame(reader)
	if err != nil {
		return "", 0, nil
	}

	resizedImage := resizeImage(firstFrame)

	filePath := uuid.NewString() + ".jpeg"

	fileSize, err := writeNewPublicFile(
		client,
		filePath,
		func(w io.Writer) error {
			return jpeg.Encode(w, resizedImage, &jpeg.Options{Quality: 80})
		},
	)

	if err != nil {
		return "", 0, err
	}

	return getPublicFileURL(filePath), fileSize, nil
}

func getCompressedImageURL(macroURL string) (string, int64, error) {
	u, _ := url.Parse("https://api.resmush.it/ws.php")
	q := u.Query()
	q.Set("img", macroURL)
	q.Set("qlty", "50")
	u.RawQuery = q.Encode()

	response, err := sendHTTPGetRequest(u.String())

	if err != nil {
		return "", 0, err
	}

	compressionResult := map[string]interface{}{}

	if err := json.Unmarshal(response, &compressionResult); err != nil {
		return "", 0, fmt.Errorf("failed to parse image compression API response: %v", err)
	}

	compressedImage, ok := compressionResult["dest"].(string)

	if !ok {
		return "", 0, fmt.Errorf("unable to find compressed image")
	}

	compressedImageSize, ok := compressionResult["dest_size"].(float64)

	return compressedImage, int64(compressedImageSize), nil
}

func getImageThumbnailURL(macroURL string) (string, int64, error) {
	client, err := newStorageClient()
	if err != nil {
		return "", 0, fmt.Errorf("failed to create storage client: %v", err)
	}

	defer closeStorageClient(client)

	imgBuf, err := sendHTTPGetRequest(macroURL)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read image: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(imgBuf))
	if err != nil {
		return "", 0, fmt.Errorf("unable to decode image: %v", err)
	}

	resizedImg := resizeImage(img)

	filePath := uuid.NewString() + ".jpeg"

	fileSize, err := writeNewPublicFile(
		client,
		filePath,
		func(w io.Writer) error {
			return jpeg.Encode(w, resizedImg, &jpeg.Options{Quality: 80})
		},
	)

	if err != nil {
		return "", 0, err
	}

	return getPublicFileURL(filePath), fileSize, nil
}

func processImage(macroURL string, fileSize int64) (string, int64, error) {
	if fileSize <= cThumbnailMaxSize {
		return macroURL, fileSize, nil
	}

	var (
		compressedImage     string
		compressedImageSize int64
	)

	compressedImage, compressedImageSize, err := getCompressedImageURL(macroURL)

	if err != nil {
		compressedImage, compressedImageSize, err = getCompressedImageURL(macroURL)
	}

	if err == nil {
		return compressedImage, compressedImageSize, nil
	}

	log.Printf("unable to compress image using API: %v", err)

	thumbnailURL, thumbnailSize, err := getImageThumbnailURL(macroURL)

	if err != nil {
		return "", 0, fmt.Errorf("unable to create thumbnail: %v", err)
	}

	if thumbnailSize > fileSize {
		log.Printf("image thumbnail is bigger than the original image: %s", macroURL)
		return "", 0, nil
	}

	return thumbnailURL, thumbnailSize, nil
}
