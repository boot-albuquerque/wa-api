package whatsmeow

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	wa "go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// JIDResolverAdapter implementa appport.JIDResolver sobre ParseJID, a mesma
// função que os use cases chamavam diretamente antes da ADR-001.
type JIDResolverAdapter struct{}

// NewJIDResolverAdapter cria o resolvedor.
func NewJIDResolverAdapter() *JIDResolverAdapter { return &JIDResolverAdapter{} }

// ResolveJID devolve o JID canônico de raw.
func (JIDResolverAdapter) ResolveJID(_ context.Context, raw string) (domain.JID, error) {
	jid, ok := ParseJID(raw)
	if !ok {
		return "", fmt.Errorf("whatsmeow: could not parse JID %q", raw)
	}
	return domain.JID(jid.String()), nil
}

// toJID reconverte um domain.JID para o tipo do SDK. O domain.JID sempre vem
// de ResolveJID, portanto já está canônico; o vazio mapeia para o JID zero,
// que é como o upstream representava "sem remetente" em MarkRead.
func toJID(j domain.JID) (types.JID, error) {
	if j == "" {
		return types.JID{}, nil
	}
	parsed, ok := ParseJID(string(j))
	if !ok {
		return types.JID{}, fmt.Errorf("whatsmeow: could not parse JID %q", string(j))
	}
	return parsed, nil
}

// PresenceControllerAdapter implementa appport.PresenceController.
type PresenceControllerAdapter struct {
	*SessionGuardAdapter
}

// NewPresenceControllerAdapter cria o adapter com a função de lookup.
func NewPresenceControllerAdapter(getClient func(txtID string) *wa.Client) *PresenceControllerAdapter {
	return &PresenceControllerAdapter{SessionGuardAdapter: NewSessionGuardAdapter(getClient)}
}

// SendPresence define a presença global da sessão.
func (a *PresenceControllerAdapter) SendPresence(ctx context.Context, txtID string, presence domain.PresenceType) error {
	client := a.getClient(txtID)
	if client == nil {
		return ErrNoSession(txtID, nil)
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
	client := a.getClient(txtID)
	if client == nil {
		return ErrNoSession(txtID, nil)
	}
	jid, err := toJID(chat)
	if err != nil {
		return err
	}
	return client.SendChatPresence(ctx, jid, types.ChatPresence(state), types.ChatPresenceMedia(media))
}

// SubscribePresence assina as atualizações de presença de um contato.
func (a *PresenceControllerAdapter) SubscribePresence(ctx context.Context, txtID string, target domain.JID) error {
	client := a.getClient(txtID)
	if client == nil {
		return ErrNoSession(txtID, nil)
	}
	jid, err := toJID(target)
	if err != nil {
		return err
	}
	return client.SubscribePresence(ctx, jid)
}

// ChatMessengerAdapter implementa appport.ChatMessenger.
type ChatMessengerAdapter struct {
	*SessionGuardAdapter
}

// NewChatMessengerAdapter cria o adapter com a função de lookup.
func NewChatMessengerAdapter(getClient func(txtID string) *wa.Client) *ChatMessengerAdapter {
	return &ChatMessengerAdapter{SessionGuardAdapter: NewSessionGuardAdapter(getClient)}
}

// MarkRead confirma a leitura das mensagens ids.
func (a *ChatMessengerAdapter) MarkRead(ctx context.Context, txtID string, ids []string, at time.Time, chat, sender domain.JID) error {
	client := a.getClient(txtID)
	if client == nil {
		return ErrNoSession(txtID, nil)
	}
	jidChat, err := toJID(chat)
	if err != nil {
		return err
	}
	jidSender, err := toJID(sender)
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
	client := a.getClient(txtID)
	if client == nil {
		return domain.MessageSendResult{}, ErrNoSession(txtID, nil)
	}

	recipient, err := toJID(target)
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

// Verificações em tempo de compilação de que os adapters implementam as portas.
var (
	_ appport.JIDResolver        = (*JIDResolverAdapter)(nil)
	_ appport.PresenceController = (*PresenceControllerAdapter)(nil)
	_ appport.ChatMessenger      = (*ChatMessengerAdapter)(nil)
)
