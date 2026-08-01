package whatsmeow

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow"
)

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

// SessionCounterAdapter adapta o ClientManager para appport.SessionCounter.
//
// A iteração sobre *whatsmeow.Client — que antes vivia dentro do use case de
// health — passou para cá: é aqui que conhecer o tipo do SDK é legítimo.
type SessionCounterAdapter struct {
	cm ClientHealthProvider
}

// NewSessionCounterAdapter cria o adapter a partir do ClientManager global.
func NewSessionCounterAdapter(cm ClientHealthProvider) *SessionCounterAdapter {
	return &SessionCounterAdapter{cm: cm}
}

// CountSessions agrega total, conectadas e autenticadas numa única passada.
func (a *SessionCounterAdapter) CountSessions(_ context.Context) (domain.SessionCounts, error) {
	counts := domain.SessionCounts{Total: a.cm.GetWhatsmeowClientsCount()}

	a.cm.IterateWhatsmeowClients(func(client *whatsmeow.Client) bool {
		if client != nil {
			if client.IsConnected() {
				counts.Connected++
			}
			if client.IsLoggedIn() {
				counts.LoggedIn++
			}
		}
		return true
	})

	return counts, nil
}

// Verificação em tempo de compilação de que o adapter implementa a porta.
var _ appport.SessionCounter = (*SessionCounterAdapter)(nil)
