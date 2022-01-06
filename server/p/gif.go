package p

import (
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"io"

	"willnorris.com/go/gifresize"
)

func getGifFirstFrame(reader io.Reader) (*image.RGBA, error) {
	decodedGif, err := gif.DecodeAll(reader)

	if err != nil {
		return nil, NewPerError(fmt.Sprintf("gif.DecodeAll: %v", err))
	}

	imgWidth, imgHeight := getGifDimensions(decodedGif)

	overpaintImage := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	if len(decodedGif.Image) == 0 {
		return nil, NewPerError("gif contains zero images")
	}

	srcImg := decodedGif.Image[0]
	draw.Draw(overpaintImage, overpaintImage.Bounds(), srcImg, image.Point{}, draw.Over)

	return overpaintImage, nil
}

func getGifDimensions(gifImage *gif.GIF) (x, y int) {
	var (
		lowestX  int
		lowestY  int
		highestX int
		highestY int
	)

	for _, img := range gifImage.Image {
		if img.Rect.Min.X < lowestX {
			lowestX = img.Rect.Min.X
		}

		if img.Rect.Min.Y < lowestY {
			lowestY = img.Rect.Min.Y
		}

		if img.Rect.Max.X > highestX {
			highestX = img.Rect.Max.X
		}

		if img.Rect.Max.Y > highestY {
			highestY = img.Rect.Max.Y
		}
	}

	return highestX - lowestX, highestY - lowestY
}

func resizeGif(reader io.Reader, writer io.Writer) error {
	tx := func(m image.Image) image.Image { //nolint
		return resizeImage(m)
	}

	if err := gifresize.Process(writer, reader, tx); err != nil {
		return err
	}

	return nil
}
