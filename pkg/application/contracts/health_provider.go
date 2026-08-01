package port

import (
	"context"

	"wa-api/pkg/domain"
)

// SessionCounter expõe a contagem agregada de sessões WhatsApp em termos de
// domínio.
//
// Substitui a antiga HealthClientProvider, que devolvia o tipo concreto do
// SDK (IterateWhatsmeowClients(func(*whatsmeow.Client) bool)) e portanto
// obrigava o use case de health a importar go.mau.fi/whatsmeow só para
// perguntar "quantas sessões estão conectadas?" — uma pergunta que é
// inteiramente de domínio. Ver ADR-001.
type SessionCounter interface {
	// CountSessions devolve as contagens agregadas de sessões. O ctx existe
	// para que implementações que consultem estado remoto possam respeitar
	// cancelamento; a implementação em memória o ignora.
	CountSessions(ctx context.Context) (domain.SessionCounts, error)
}
