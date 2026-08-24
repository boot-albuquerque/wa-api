package status

import (
	"net/http"
	"net/url"
	"strings"
)

func resolveMimeType(reqMimeType string, data []byte) string {
	if reqMimeType != "" {
		return reqMimeType
	}
	return http.DetectContentType(data)
}

func decodedBase64Len(encoded string) (int64, bool) {
	if len(encoded)%4 != 0 {
		return 0, false
	}
	return int64(len(encoded))/4*3 - int64(len(encoded)-len(strings.TrimRight(encoded, "="))), true
}

func isHTTPURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}
