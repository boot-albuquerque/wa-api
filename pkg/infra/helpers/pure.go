// Package helpers provides pure utility functions extracted from
// root wa-api/helpers.go (Phase 12a).
package helpers

import (
	"net/url"
	"regexp"
)

var urlRegex = regexp.MustCompile(`https?://[^\s"']*[^\"'\s\.,!?()[\]{}]`)

// Find returns true if val is in slice.
func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// IsHTTPURL checks if the input is a valid HTTP/HTTPS URL.
func IsHTTPURL(input string) bool {
	parsed, err := url.ParseRequestURI(input)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// ExtractFirstURL extracts the first URL from a text string.
func ExtractFirstURL(text string) string {
	match := urlRegex.FindString(text)
	if match == "" {
		return ""
	}
	return match
}
