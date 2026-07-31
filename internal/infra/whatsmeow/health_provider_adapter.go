package whatsmeow

import "go.mau.fi/whatsmeow"

// ClientLookup is the subset of ClientManager methods needed by adapters
// that look up WhatsApp clients by user ID. Both the root-level (package
// main) ClientManager and the internal/whatsmeow ClientManager satisfy it
// implicitly, which breaks the circular concrete-type dependency between
// root main and internal/.
type ClientLookup interface {
	GetWhatsmeowClient(id string) *whatsmeow.Client
}

// ClientHealthProvider is the interface the root-level ClientManager
// satisfies for health-related queries. Extracted to break the concrete
// type dependency between internal/ and package main.
type ClientHealthProvider interface {
	GetWhatsmeowClientsCount() int
	IterateWhatsmeowClients(func(*whatsmeow.Client) bool)
}

// HealthClientProviderAdapter adapta o ClientManager para appport.HealthClientProvider.
type HealthClientProviderAdapter struct {
	cm ClientHealthProvider
}

// NewHealthClientProviderAdapter cria o adapter a partir do ClientManager global.
func NewHealthClientProviderAdapter(cm ClientHealthProvider) *HealthClientProviderAdapter {
	return &HealthClientProviderAdapter{cm: cm}
}

// GetWhatsmeowClientsCount retorna a contagem de clientes ativos.
func (a *HealthClientProviderAdapter) GetWhatsmeowClientsCount() int {
	return a.cm.GetWhatsmeowClientsCount()
}

// IterateWhatsmeowClients itera sobre todos os clientes whatsmeow.
func (a *HealthClientProviderAdapter) IterateWhatsmeowClients(fn func(*whatsmeow.Client) bool) {
	a.cm.IterateWhatsmeowClients(fn)
}
