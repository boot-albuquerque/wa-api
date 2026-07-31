// Package port define as interfaces (ports) que os usecases consomem.
// Implementações concretas (adapters) vivem em internal/infrastructure/.
package port

import (
	"context"

	"go.mau.fi/whatsmeow"
)

// ClientProvider abstrai o acesso ao cliente whatsmeow conectado.
// A implementação concreta usa o clientManager global do upstream.
type ClientProvider interface {
	// GetWhatsmeowClient retorna o cliente whatsmeow associado ao txtID
	// informado, ou erro se não houver sessão conectada.
	GetWhatsmeowClient(ctx context.Context, txtID string) (*whatsmeow.Client, error)
}
