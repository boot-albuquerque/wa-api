package chat

import (
	"context"
	waclient "wa-api/pkg/infra/wa-noise/client"
	wasession "wa-api/pkg/infra/wa-noise/runtime/session"

	appport "wa-api/pkg/application/contracts"
)

// MessageComposerAdapter implementa appport.MessageComposer sobre o
// clientManager.
type MessageComposerAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewMessageComposerAdapter cria o adapter com a função de lookup.
// O parâmetro getClient é tipicamente clientManager.GetWaNoiseClient
// (convertido via clientForGetter).
func NewMessageComposerAdapter(getClient waclient.Getter) *MessageComposerAdapter {
	return &MessageComposerAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// NewMessageID delega ao GenerateMessageID do cliente da sessão txtID.
func (a *MessageComposerAdapter) NewMessageID(_ context.Context, txtID string) (string, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return "", err
	}
	return client.GenerateMessageID(), nil
}

// Verificação em tempo de compilação de que o adapter implementa a porta.
var _ appport.MessageComposer = (*MessageComposerAdapter)(nil)
