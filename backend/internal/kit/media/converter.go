package media

import (
	"bytes"
	"errors"
	"golang.org/x/image/webp"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
)

func ConvertWebPToJPEG(webpData []byte, quality int) ([]byte, error) {

	img, err := webp.Decode(bytes.NewReader(webpData))
	if err != nil {
		return nil, errors.New("failed to decode webp: " + err.Error())
	}

	bounds := img.Bounds()
	jpgImg := image.NewRGBA(bounds)

	draw.Draw(jpgImg, bounds, &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	draw.Draw(jpgImg, bounds, img, bounds.Min, draw.Over)

	if quality <= 0 {
		quality = 100
	}

	out := new(bytes.Buffer)
	err = jpeg.Encode(out, jpgImg, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, errors.New("failed to encode jpeg: " + err.Error())
	}

	return out.Bytes(), nil
}
