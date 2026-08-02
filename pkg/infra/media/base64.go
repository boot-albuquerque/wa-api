package media

import (
	"encoding/base64"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
)

// FileToBase64 converts a file to base64 encoding and returns the encoding and MIME type
func FileToBase64(filepath string) (string, string, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		log.Warn().Err(err).Str("path", filepath).Msg("Failed to read file for base64 encoding")
		return "", "", err
	}
	mimeType := http.DetectContentType(data)
	return base64.StdEncoding.EncodeToString(data), mimeType, nil
}
