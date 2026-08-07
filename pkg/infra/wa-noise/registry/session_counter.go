package registry

import (
	"context"

	wanoise "wa-api/internal/wa-noise"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ClientLookup is the subset of ClientManager methods needed by adapters
// that look up WhatsApp clients by user ID. Both the root-level (package
// main) ClientManager and the internal/wa-noise ClientManager satisfy it
// implicitly, which breaks the circular concrete-type dependency between
// root main and internal/.
type ClientLookup interface {
	GetWaNoiseClient(id string) *wanoise.Client
}

// ClientHealthProvider is the interface the root-level ClientManager
// satisfies for health-related queries. Extracted to break the concrete
// type dependency between internal/ and package main.
type ClientHealthProvider interface {
	GetWaNoiseClientsCount() int
	IterateWaNoiseClients(func(*wanoise.Client) bool)
}

// SessionCounterAdapter adapta o ClientManager para appport.SessionCounter.
//
// A iteração sobre *wa-noise.Client — que antes vivia dentro do use case de
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
	counts := domain.SessionCounts{Total: a.cm.GetWaNoiseClientsCount()}

	a.cm.IterateWaNoiseClients(func(client *wanoise.Client) bool {
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
