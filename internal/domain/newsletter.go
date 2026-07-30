package domain

import "go.mau.fi/whatsmeow/types"

// NewsletterCollection represents a collection of newsletters
type NewsletterCollection struct {
	Newsletter []types.NewsletterMetadata `json:"newsletter"`
}
