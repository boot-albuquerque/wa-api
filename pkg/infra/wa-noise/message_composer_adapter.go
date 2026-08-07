package whatsmeow

import (
	"context"
	"wa-api/pkg/infra/wa-noise/waclient"

	appport "wa-api/pkg/application/contracts"
)

// MessageComposerAdapter implementa appport.MessageComposer sobre o
// clientManager.
type MessageComposerAdapter struct {
	*SessionGuardAdapter
}

// NewMessageComposerAdapter cria o adapter com a função de lookup.
// O parâmetro getClient é tipicamente clientManager.GetWhatsmeowClient
// (convertido via clientForGetter).
func NewMessageComposerAdapter(getClient waclient.Getter) *MessageComposerAdapter {
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
