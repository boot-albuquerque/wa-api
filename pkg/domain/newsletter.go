package domain

// NewsletterCollection represents a collection of newsletters
type NewsletterCollection struct {
	// Newsletter carrega a lista devolvida pela porta NewsletterReader. É any
	// e não um tipo do SDK: o domínio não deve conhecer types.NewsletterMetadata,
	// e o valor só existe para ser serializado (ADR-001).
	Newsletter interface{} `json:"newsletter"`
}
