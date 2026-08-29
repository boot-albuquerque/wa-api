package sessioncount

import (
	"context"

	"wa-api/internal/noise"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ClientHealthProvider is the interface the root-level ClientManager
// satisfies for health-related queries. Extracted to break the concrete
// type dependency between internal/ and package main.
type ClientHealthProvider interface {
	GetNoiseClientsCount() int
	IterateNoiseClients(func(*noise.Client) bool)
}

// SessionCounterAdapter adapta o ClientManager para appport.SessionCounter.
//
// A iteração sobre *noise.Client — que antes vivia dentro do use case de
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
	counts := domain.SessionCounts{Total: a.cm.GetNoiseClientsCount()}

	a.cm.IterateNoiseClients(func(client *noise.Client) bool {
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
