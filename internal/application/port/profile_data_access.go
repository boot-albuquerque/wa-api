// Package port define as interfaces (ports) que os usecases consomem.
// Implementações concretas (adapters) vivem em internal/infrastructure/.
package port

import (
	"context"

	"disparazap/internal/domain"
)

// ProfileDataAccess abstrai a leitura de dados do perfil WhatsApp.
// Implementação concreta em internal/infrastructure/whatsmeow.
type ProfileDataAccess interface {
	PushName() string
	OwnJID() (domain.JID, bool)
	ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error)
	ContactInfo(ctx context.Context, jid domain.JID) (string, string, error)
}
