package whatsmeow

import (
	"context"
	"time"
	wajid "wa-api/pkg/infra/wa-noise/jid"
	wasession "wa-api/pkg/infra/wa-noise/session"
	"wa-api/pkg/infra/wa-noise/waclient"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/internal/wa-noise/proto/waCommon"
	"wa-api/internal/wa-noise/proto/waE2E"

	"google.golang.org/protobuf/proto"
)

// ChatMessengerAdapter implementa appport.ChatMessenger.
type ChatMessengerAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewChatMessengerAdapter cria o adapter com a função de lookup.
func NewChatMessengerAdapter(getClient waclient.Getter) *ChatMessengerAdapter {
	return &ChatMessengerAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// MarkRead confirma a leitura das mensagens ids.
func (a *ChatMessengerAdapter) MarkRead(ctx context.Context, txtID string, ids []string, at time.Time, chat, sender domain.JID) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jidChat, err := wajid.ToJID(chat)
	if err != nil {
		return err
	}
	jidSender, err := wajid.ToJID(sender)
	if err != nil {
		return err
	}
	return client.MarkRead(ctx, ids, at, jidChat, jidSender)
}

// SendReaction monta a mensagem de reação no formato do SDK e a envia.
//
// Esta montagem — MessageKey mais waE2E.ReactionMessage — vivia dentro de
// ReactUseCase. É exatamente o tipo de lógica que a ADR-001 previu migrar
// para o adapter, e o ponto em que o compilador deixa de cobrir a mudança.
func (a *ChatMessengerAdapter) SendReaction(ctx context.Context, txtID string, target domain.JID, reaction domain.Reaction) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	key := &waCommon.MessageKey{
		RemoteJID: proto.String(recipient.String()),
		FromMe:    proto.Bool(reaction.FromMe),
		ID:        proto.String(reaction.TargetMessageID),
	}
	if !reaction.FromMe && reaction.Participant != "" {
		key.Participant = proto.String(string(reaction.Participant))
	}

	msg := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key:               key,
			Text:              proto.String(reaction.Text),
			GroupingKey:       proto.String(reaction.Text),
			SenderTimestampMS: proto.Int64(reaction.SentAt.UnixMilli()),
		},
	}

	resp, err := client.SendMessage(ctx, recipient, msg)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp}, nil
}

// Verificação em tempo de compilação de que o adapter implementa a porta.
var _ appport.ChatMessenger = (*ChatMessengerAdapter)(nil)
