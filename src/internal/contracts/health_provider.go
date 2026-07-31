package port

import "go.mau.fi/whatsmeow"

// HealthClientProvider abstracts client manager operations for health checks
type HealthClientProvider interface {
	// GetWhatsmeowClientsCount returns the count of active whatsmeow clients
	GetWhatsmeowClientsCount() int
	// IterateWhatsmeowClients iterates over all whatsmeow clients
	IterateWhatsmeowClients(fn func(*whatsmeow.Client) bool)
}
