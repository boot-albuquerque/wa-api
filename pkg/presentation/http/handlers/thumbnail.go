package handlers

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"

	"github.com/nfnt/resize"
)

// JPEGThumbnail creates a JPEG thumbnail of the given image at the specified
// width and height using Lanczos resampling.
func JPEGThumbnail(img image.Image, width, height uint) ([]byte, error) {
	if img == nil {
		return nil, errors.New("cannot create thumbnail from a nil image")
	}
	thumb := resize.Thumbnail(width, height, img, resize.Lanczos3)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
