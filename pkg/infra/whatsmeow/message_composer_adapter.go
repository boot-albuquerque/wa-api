package whatsmeow

import (
	"context"

	appport "wa-api/pkg/application/contracts"

	wa "go.mau.fi/whatsmeow"
)

// MessageComposerAdapter implementa appport.MessageComposer sobre o
// clientManager.
type MessageComposerAdapter struct {
	*SessionGuardAdapter
}

// NewMessageComposerAdapter cria o adapter com a função de lookup.
// O parâmetro getClient é tipicamente clientManager.GetWhatsmeowClient.
func NewMessageComposerAdapter(getClient func(txtID string) *wa.Client) *MessageComposerAdapter {
	return &MessageComposerAdapter{SessionGuardAdapter: NewSessionGuardAdapter(getClient)}
}

// NewMessageID delega ao GenerateMessageID do cliente da sessão txtID.
func (a *MessageComposerAdapter) NewMessageID(_ context.Context, txtID string) (string, error) {
	client := a.getClient(txtID)
	if client == nil {
		return "", ErrNoSession(txtID, nil)
	}
	return client.GenerateMessageID(), nil
}

// Verificação em tempo de compilação de que o adapter implementa a porta.
var _ appport.MessageComposer = (*MessageComposerAdapter)(nil)
