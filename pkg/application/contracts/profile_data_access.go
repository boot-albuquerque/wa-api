// Package port define as interfaces (ports) que os usecases consomem.
// Implementações concretas (adapters) vivem em pkg/infra/.
package port

import (
	"context"

	"wa-api/pkg/domain"
)

// ProfileDataAccess abstrai a leitura de dados do perfil WhatsApp.
// Implementação concreta em pkg/infra/wa-noise.
type ProfileDataAccess interface {
	PushName() string
	OwnJID() (domain.JID, bool)
	ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error)
	ContactInfo(ctx context.Context, jid domain.JID) (string, string, error)

	// DeviceInfo devolve identidade e estado do aparelho pareado a partir do
	// store local. Não faz chamada de rede e não devolve erro: campo ausente
	// vira zero-value, que é a resposta honesta para "o store ainda não tem
	// isso".
	DeviceInfo() domain.SessionDeviceInfo
}
