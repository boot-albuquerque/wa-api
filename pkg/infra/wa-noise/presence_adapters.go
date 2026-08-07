package whatsmeow

import (
	"context"
	"fmt"
	wajid "wa-api/pkg/infra/wa-noise/jid"
	wasession "wa-api/pkg/infra/wa-noise/session"
	"wa-api/pkg/infra/wa-noise/waclient"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/internal/wa-noise/types"
)

// PresenceControllerAdapter implementa appport.PresenceController.
type PresenceControllerAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewPresenceControllerAdapter cria o adapter com a função de lookup.
func NewPresenceControllerAdapter(getClient waclient.Getter) *PresenceControllerAdapter {
	return &PresenceControllerAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// SendPresence define a presença global da sessão.
func (a *PresenceControllerAdapter) SendPresence(ctx context.Context, txtID string, presence domain.PresenceType) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}

	var p types.Presence
	switch presence {
	case domain.PresenceAvailable:
		p = types.PresenceAvailable
	case domain.PresenceUnavailable:
		p = types.PresenceUnavailable
	default:
		return fmt.Errorf("whatsmeow: unknown presence type %q", string(presence))
	}

	return client.SendPresence(ctx, p)
}

// SendChatPresence sinaliza estado dentro de uma conversa.
func (a *PresenceControllerAdapter) SendChatPresence(ctx context.Context, txtID string, chat domain.JID, state, media string) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(chat)
	if err != nil {
		return err
	}
	return client.SendChatPresence(ctx, jid, types.ChatPresence(state), types.ChatPresenceMedia(media))
}

// SubscribePresence assina as atualizações de presença de um contato.
func (a *PresenceControllerAdapter) SubscribePresence(ctx context.Context, txtID string, target domain.JID) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(target)
	if err != nil {
		return err
	}
	return client.SubscribePresence(ctx, jid)
}

// Verificação em tempo de compilação de que o adapter implementa a porta.
var _ appport.PresenceController = (*PresenceControllerAdapter)(nil)
