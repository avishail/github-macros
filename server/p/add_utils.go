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
	"math"
	"net/http"
	"net/url"
	"strings"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"github.com/nfnt/resize"
	"google.golang.org/api/iterator"
)

const (
	Success                 = 0
	EmptyName               = 1
	NameContainsSpaces      = 2
	NameAlreadyExist        = 3
	EmptyURL                = 4
	InvalidURL              = 5
	URLHostnameNotSupported = 6
	FileIsTooBig            = 7
	FileFormatNotSupported  = 8
	TransientError          = 9
	MissingMandatoryFields  = 10
	InfraFailure            = 11
	PermanentError          = 12
)

const (
	cFileMaxSize              = 1024 * 1024 * 10
	cThumbnailMaxSize         = 1024 * 400
	cThumbnailFallbackMaxSize = int64(1024 * 1024 * 1.5)
	cGifThumbnailSize         = 1024 * 700
)

type ErrorCode = int

type permanentError struct {
	errMessage string
}

func NewPerError(errMsg string) *permanentError {
	return &permanentError{errMessage: errMsg}
}

func (pe *permanentError) Error() string {
	return pe.errMessage
}

func panicIfNotPermanent(err error, format string, v ...interface{}) {
	if _, ok := err.(*permanentError); ok {
		log.Printf(format, v...)
		return
	}

	log.Panicf(format, v...)
}

func isGithubMedia(macroURL string) bool {
	u, err := url.Parse(macroURL)
	if err != nil {
		return false
	}

	return strings.HasSuffix(u.Host, "githubusercontent.com")
}

func getErrorCodeResponse(errCode ErrorCode) (string, error) {
	return getRespons(
		map[string]interface{}{
			"code": errCode,
		},
	)
}

func getRespons(responseMap map[string]interface{}) (string, error) {
	response, err := json.Marshal(responseMap)

	if err != nil {
		return "", fmt.Errorf("error while marshaling response: %v", err)
	}

	return string(response), nil
}

type readerWithMaxSize struct {
	Reader  io.Reader
	MaxSize int64
	curSize int64
}

func (r *readerWithMaxSize) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)

	if err == nil || err == io.EOF {
		r.curSize += int64(n)
		if r.curSize > r.MaxSize {
			return n, NewPerError("network reader exceed read limit")
		}
	}

	return n, err
}

func sendHTTPGetRequest(requestURL string) ([]byte, error) {
	resp, err := http.Get(requestURL)

	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %v", err)
	}

	defer resp.Body.Close()

	return io.ReadAll(
		&readerWithMaxSize{Reader: resp.Body, MaxSize: cFileMaxSize},
	)
}

func resizeImage(m image.Image) image.Image {
	p := m.Bounds().Max
	width := uint(p.X)
	height := uint(p.Y)

	if width >= height {
		width = 150
		height = 0
	} else {
		height = 150
		width = 0
	}

	return resize.Resize(width, height, m, resize.NearestNeighbor)
}

func postprocessNewMacro(
	bigqueryClient *bigquery.Client, macroName, macroURL string, fileSize int64, isGif, shouldAdd bool,
) (*MacroRow, bool) {
	macroRow, err := getMacroWithTheSameURL(bigqueryClient, macroURL)
	if err != nil {
		log.Printf("error while fetching macro with the same URL: %v", err)
	}

	if macroRow != nil {
		newMacro := duplicateExistingMacro(bigqueryClient, macroName, macroRow)

		return newMacro, true
	}

	var (
		thumbnail        string
		gifThumbnail     string
		thumbnailSize    int64
		gifThumbnailSize int64
	)

	var success bool

	storageClient := newStorageClient()
	defer func() {
		if closeErr := closeStorageClient(storageClient); closeErr != nil {
			log.Printf("closeStorageClient: %v", closeErr)
		}
	}()

	if isGif {
		fileSize, thumbnail, thumbnailSize, gifThumbnail, gifThumbnailSize, success = processGif(storageClient, macroURL, fileSize)
	} else {
		fileSize, thumbnail, thumbnailSize, success = processImage(storageClient, macroURL, fileSize)
	}

	if !success {
		return nil, false
	}

	if err != nil {
		log.Panicf("failed to process new macro: %v", err)
	}

	urlsToConvert := []string{macroURL}

	if thumbnail != "" {
		urlsToConvert = append(urlsToConvert, thumbnail)
	}

	if gifThumbnail != "" {
		urlsToConvert = append(urlsToConvert, gifThumbnail)
	}

	githubURLs, err := GetGithubImages(bigqueryClient, urlsToConvert)
	if err != nil {
		log.Panicf("failed to get github images: %v", err)
	}

	finalMacroURL, ok := githubURLs[macroURL]
	if !ok {
		log.Panic("failed to locate github macro image")
	}

	finalThumbnailURL := githubURLs[thumbnail]
	finalGifThumbnailURL := githubURLs[gifThumbnail]

	if err := createNewReportsEntry(bigqueryClient, macroName); err != nil {
		log.Panicf("failed to create reports entry: %v", err)
	}

	if err := createNewUsagesEntry(bigqueryClient, macroName); err != nil {
		log.Panicf("failed to create usages entry: %v", err)
	}

	if shouldAdd {
		insertNewMacro(
			bigqueryClient,
			macroName,
			finalMacroURL,
			finalThumbnailURL,
			isGif,
			finalGifThumbnailURL,
			macroURL,
			fileSize,
			thumbnailSize,
			gifThumbnailSize,
		)
	} else {
		updateMacro(bigqueryClient, macroName, finalMacroURL, finalThumbnailURL, finalGifThumbnailURL, thumbnailSize, gifThumbnailSize)
	}

	newMacro := &MacroRow{
		Name:         macroName,
		URL:          finalMacroURL,
		Thumbnail:    finalThumbnailURL,
		IsGif:        isGif,
		GifThumbnail: finalGifThumbnailURL,
	}

	return newMacro, true
}

func insertNewMacro(
	client *bigquery.Client,
	macroName, macroURL, thumbnailURL string,
	isGif bool,
	gifThumbnailURL, origURL string,
	macroSize,
	thumbnailSize,
	gifThumbnailSize int64,
) {
	query := client.Query(`
		INSERT INTO github-macros.macros.macros 
		(name, orig_url, url, url_size, thumbnail, thumbnail_size, is_gif, gif_thumbnail, gif_thumbnail_size, width, height, add_retries)
		VALUES (@name, @orig_url, @url, @url_size, @thumbnail, @thumbnail_size, @is_gif, @gif_thumbnail, @gif_thumbnail_size, 0, 0, 0)
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "name",
			Value: macroName,
		},
		{
			Name:  "url",
			Value: macroURL,
		},
		{
			Name:  "orig_url",
			Value: origURL,
		},
		{
			Name:  "url_size",
			Value: macroSize,
		},
		{
			Name:  "thumbnail",
			Value: thumbnailURL,
		},
		{
			Name:  "thumbnail_size",
			Value: thumbnailSize,
		},
		{
			Name:  "is_gif",
			Value: isGif,
		},
		{
			Name:  "gif_thumbnail",
			Value: gifThumbnailURL,
		},
		{
			Name:  "gif_thumbnail_size",
			Value: gifThumbnailSize,
		},
	}

	if _, err := runQuery(context.Background(), query); err != nil {
		log.Panicf("failed to insert new macro: %v", err)
	}
}

func updateMacro(
	client *bigquery.Client,
	macroName, macroURL, thumbnailURL, gifThumbnailURL string,
	thumbnailSize, gifThumbnailSize int64,
) {
	query := client.Query(`
		UPDATE github-macros.macros.macros 
		SET	url=@url
			thumbnail=@thumbnail
			thumbnail_size=@thumbnail_size
			gif_thumbnail=@gif_thumbnail
			gif_thumbnail_size=@gif_thumbnail_size
		WHERE name=@name	
	`)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "url",
			Value: macroURL,
		},
		{
			Name:  "thumbnail",
			Value: thumbnailURL,
		},
		{
			Name:  "thumbnail_size",
			Value: thumbnailSize,
		},
		{
			Name:  "gif_thumbnail",
			Value: gifThumbnailURL,
		},
		{
			Name:  "gif_thumbnail_size",
			Value: gifThumbnailSize,
		},
		{
			Name:  "name",
			Value: macroName,
		},
	}

	if _, err := runQuery(context.Background(), query); err != nil {
		log.Panicf("failed to update macro '%s': %v", macroName, err)
	}
}

func duplicateExistingMacro(client *bigquery.Client, macroName string, macroToDuplicate *MacroRow) *MacroRow {
	insertNewMacro(
		client,
		macroName,
		macroToDuplicate.URL,
		macroToDuplicate.Thumbnail,
		macroToDuplicate.IsGif,
		macroToDuplicate.GifThumbnail,
		macroToDuplicate.OrigURL,
		macroToDuplicate.URLSize,
		macroToDuplicate.ThumbnailSize,
		macroToDuplicate.GifThumbnailSize,
	)

	var newMacro = *macroToDuplicate
	newMacro.Name = macroName

	return &newMacro
}

func createNewUsagesEntry(client *bigquery.Client, macroName string) error {
	query := client.Query(`
		INSERT INTO github-macros.macros.usages (macro_name, usages)
		SELECT @macro_name, 0 FROM (SELECT 1) 
		LEFT JOIN github-macros.macros.usages
		ON macro_name = @macro_name
		WHERE macro_name IS NULL
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

func createNewReportsEntry(client *bigquery.Client, macroName string) error {
	query := client.Query(`
		INSERT INTO github-macros.macros.reports (macro_name, reports)
		SELECT @macro_name, 0 FROM (SELECT 1) 
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

	_, err := runQuery(context.Background(), query)

	return err
}

func getMacroWithTheSameURL(client *bigquery.Client, macroURL string) (*MacroRow, error) {
	query := client.Query(
		"SELECT 1 FROM `github-macros.macros.macros` WHERE orig_url=@orig_url",
	)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "orig_url",
			Value: macroURL,
		},
	}

	ctx := context.Background()

	iter, err := runQuery(ctx, query)

	if err != nil {
		return nil, fmt.Errorf("error executing runQuery: %v", err)
	}

	var r MacroRow
	err = iter.Next(&r)

	if err != nil && err != iterator.Done {
		return nil, err
	}

	if err == iterator.Done {
		return nil, nil
	}

	return &r, nil
}

func processGif(
	storageClient *storage.Client,
	macroURL string,
	macroSize int64) (
	actualMacroSize int64, thumbnailURL string, thunmbailSize int64, gifThumbnailURL string, gifThumbnailSize int64, success bool,
) {
	success = false
	gifBytes, err := sendHTTPGetRequest(macroURL)

	if err != nil {
		panicIfNotPermanent(err, "unable to read GIF '%s' : %v", macroURL, err)
		return
	}

	actualMacroSize = int64(len(gifBytes))

	thumbnailURL, thunmbailSize, err = getGifFirstFrameURL(storageClient, bytes.NewReader(gifBytes))
	if err != nil {
		panicIfNotPermanent(err, "getGifFirstFrameURL: %v", err)
		return
	}

	if macroSize < cGifThumbnailSize {
		success = true
		return
	}

	// we can live without gif thumbnail since by default we don't show the animated gif
	// but the thumbnail, so the experience will be good from user perspective
	gifThumbnailURL, gifThumbnailSize, err = getResizedGifURL(storageClient, bytes.NewReader(gifBytes))
	if err != nil {
		log.Printf("failed to get resizes GIF: %v", err)
	}

	if gifThumbnailSize > macroSize {
		log.Printf("resized gif is bigger than the original gif: %s", macroURL)
		gifThumbnailURL = ""
		gifThumbnailSize = 0
	}

	success = true

	return
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
			err = NewPerError(fmt.Sprintf("error while decoding: %s", r))
		}
	}()

	firstFrame, err := getGifFirstFrame(reader)
	if err != nil {
		panicIfNotPermanent(err, "getGifFirstFrame: %v", err)
		return "", 0, err
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
		log.Panicf("failed to store gif first frame: %v", err)
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
		return "", math.MaxInt64, err
	}

	compressionResult := map[string]interface{}{}

	if err := json.Unmarshal(response, &compressionResult); err != nil {
		return "", math.MaxInt64, fmt.Errorf("failed to parse image compression API response: %v", err)
	}

	compressedImage, ok := compressionResult["dest"].(string)

	if !ok {
		return "", math.MaxInt64, fmt.Errorf("unable to find compressed image")
	}

	compressedImageSize, ok := compressionResult["dest_size"].(float64)

	if !ok {
		return "", math.MaxInt64, fmt.Errorf("unable to find compressed image size")
	}

	return compressedImage, int64(compressedImageSize), nil
}

func getImageThumbnailURL(storageClient *storage.Client, macroURL string) (int64, string, int64, error) {
	imgBuf, err := sendHTTPGetRequest(macroURL)
	if err != nil {
		return 0, "", math.MaxInt64, fmt.Errorf("failed to read image: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(imgBuf))
	if err != nil {
		return int64(len(imgBuf)), "", math.MaxInt64, fmt.Errorf("unable to decode image: %v", err)
	}

	resizedImg := resizeImage(img)

	filePath := uuid.NewString() + ".jpeg"

	fileSize, err := writeNewPublicFile(
		storageClient,
		filePath,
		func(w io.Writer) error {
			return jpeg.Encode(w, resizedImg, &jpeg.Options{Quality: 80})
		},
	)

	if err != nil {
		return int64(len(imgBuf)), "", math.MaxInt64, err
	}

	return int64(len(imgBuf)), getPublicFileURL(filePath), fileSize, nil
}

func processImage(storageClient *storage.Client, macroURL string, macroSize int64) (int64, string, int64, bool) {
	if macroSize <= cThumbnailMaxSize {
		return macroSize, macroURL, macroSize, true
	}

	var (
		compressedImage     string
		compressedImageSize int64
	)

	compressedImage, compressedImageSize, compressionErr := getCompressedImageURL(macroURL)

	if compressionErr != nil {
		compressedImage, compressedImageSize, compressionErr = getCompressedImageURL(macroURL)
	}

	if compressionErr != nil {
		log.Printf("unable to compress '%s' using API: %v", macroURL, compressionErr)
	} else if compressedImageSize < cThumbnailMaxSize {
		return macroSize, compressedImage, compressedImageSize, true
	}

	log.Printf("compressed image is too big (%d), trying to resize", compressedImageSize)

	macroActualSize, thumbnailURL, thumbnailSize, thumbnailErr := getImageThumbnailURL(storageClient, macroURL)

	// let's use the actual file size of the image we read
	if macroActualSize != 0 {
		macroSize = macroActualSize
	}

	if thumbnailErr != nil {
		log.Printf("failed to resize '%s': %v", macroURL, thumbnailErr)
	}

	if thumbnailSize <= compressedImageSize && thumbnailSize <= macroSize && thumbnailSize <= cThumbnailFallbackMaxSize {
		return macroSize, thumbnailURL, thumbnailSize, true
	}

	if compressedImageSize <= thumbnailSize && compressedImageSize < macroSize && compressedImageSize <= cThumbnailFallbackMaxSize {
		return macroSize, thumbnailURL, thumbnailSize, true
	}

	if macroSize <= cThumbnailFallbackMaxSize {
		return macroSize, macroURL, macroSize, true
	}

	panicIfNotPermanent(thumbnailErr, "failed to create thumbnail and size is unknown or too big: %s", macroURL)

	return macroSize, "", 0, false
}
