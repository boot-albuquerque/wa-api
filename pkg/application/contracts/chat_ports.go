package port

import (
	"context"
	"time"

	"wa-api/pkg/domain"
)

// JIDResolver converte um telefone ou JID cru, como veio no payload, para a
// forma canônica do domínio.
//
// A resolução é uma porta e não uma função de domínio de propósito: a regra
// (prefixo "+", servidor padrão quando não há "@", validação de agente e
// dispositivo) é do SDK, e reimplementá-la em pkg/domain criaria duas
// gramáticas de JID que divergiriam em silêncio na primeira atualização do
// SDK.
type JIDResolver interface {
	// ResolveJID devolve o JID canônico, ou erro se raw não for um JID
	// válido.
	ResolveJID(ctx context.Context, raw string) (domain.JID, error)
}

// PresenceController expõe as operações de presença — o que a sessão sinaliza
// sobre estar online, digitando ou observando o estado de um contato.
type PresenceController interface {
	SessionGuard

	// SendPresence define a presença global da sessão.
	SendPresence(ctx context.Context, txtID string, presence domain.PresenceType) error

	// SendChatPresence sinaliza estado dentro de uma conversa (digitando,
	// gravando, pausado). state e media são repassados como vieram: o
	// upstream nunca os validou, e esta fase não muda comportamento.
	SendChatPresence(ctx context.Context, txtID string, chat domain.JID, state, media string) error

	// SubscribePresence assina as atualizações de presença de um contato.
	SubscribePresence(ctx context.Context, txtID string, target domain.JID) error
}

// ChatMessenger expõe as operações sobre mensagens já existentes numa
// conversa: confirmá-las como lidas e reagir a elas.
type ChatMessenger interface {
	SessionGuard

	// MarkRead confirma a leitura das mensagens ids. sender pode ser vazio
	// em conversa individual.
	MarkRead(ctx context.Context, txtID string, ids []string, at time.Time, chat, sender domain.JID) error

	// SendReaction envia uma reação. A montagem da mensagem no formato do
	// SDK é responsabilidade do adapter.
	SendReaction(ctx context.Context, txtID string, target domain.JID, reaction domain.Reaction) (domain.MessageSendResult, error)
}
