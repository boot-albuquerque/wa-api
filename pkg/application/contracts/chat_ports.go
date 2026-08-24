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
	// válido. Aceita telefone sem servidor, aplicando o servidor padrão.
	ResolveJID(ctx context.Context, raw string) (domain.JID, error)

	// ResolveQualifiedJID exige que raw já traga o servidor explícito. É a
	// regra estrita que group_request.go aplicava com seu próprio helper
	// parseJID — mais restrita que ResolveJID, e mantida separada porque
	// unificá-las passaria a aceitar entradas que aquelas rotas hoje
	// rejeitam.
	ResolveQualifiedJID(ctx context.Context, raw string) (domain.JID, error)
}

// A presença tem DUAS capacidades, e elas se separam por quem depende de quem
// (decisão 80, aplicada aqui).
//
// Anunciar é uma ação desta sessão: ela diz o que está a fazer. Assinar é pedir
// que o servidor mande as atualizações de OUTRA pessoa, e no transporte de
// página isso está bloqueado por dependência HUMANA medida — a H144 mediu
// `isMyContact:false isAddressBookContact:false` e a assinatura nunca chega,
// porque o vínculo de agenda cria-se no TELEFONE. Não é código por fazer.
//
// Com uma interface única, o adaptador de página teria de implementar as três
// para compilar. Separadas, ele satisfaz o que faz e recusa o que depende de um
// humano com o aparelho na mão.

// PresenceAnnouncer anuncia o que ESTA sessão está a fazer.
type PresenceAnnouncer interface {
	SessionGuard

	// SendPresence define a presença global da sessão.
	SendPresence(ctx context.Context, txtID string, presence domain.PresenceType) error

	// SendChatPresence sinaliza estado dentro de uma conversa (digitando,
	// gravando, pausado). state e media são repassados como vieram: o
	// upstream nunca os validou, e esta fase não muda comportamento.
	SendChatPresence(ctx context.Context, txtID string, chat domain.JID, state, media string) error
}

// PresenceSubscriber pede as atualizações de presença de OUTRA pessoa.
type PresenceSubscriber interface {
	SessionGuard

	// SubscribePresence assina as atualizações de presença de um contato.
	SubscribePresence(ctx context.Context, txtID string, target domain.JID) error
}

// PresenceController é a composição das duas, para o adaptador que satisfaz
// ambas declarar isso numa linha. Um caso de uso deve pedir a metade que usa.
type PresenceController interface {
	PresenceAnnouncer
	PresenceSubscriber
}

// ChatMessenger expõe as operações sobre mensagens já existentes numa
// conversa: confirmá-las como lidas, reagir a elas, revogá-las e editá-las.
//
// RevokeMessage e EditMessage entram AQUI, e não numa porta nova (CAP-10):
// a fronteira que separou TextMessenger de ChatMessenger no CAP-01 foi
// "cria uma mensagem" contra "opera sobre uma mensagem QUE JÁ EXISTE".
// Revogar e editar são o segundo caso — como MarkRead e SendReaction, são
// identificadas por {conversa, ID da mensagem alvo} e não têm sentido sem
// uma mensagem anterior. Uma porta própria dividiria esse mesmo conceito em
// duas sem nenhuma diferença de forma para justificá-la, ao contrário do
// que aconteceu com SimpleMessenger (que não tem etapa de upload nem de
// fetch e por isso não cabia em MediaMessenger).
type ChatMessenger interface {
	SessionGuard

	// MarkRead confirma a leitura das mensagens ids. sender pode ser vazio
	// em conversa individual.
	MarkRead(ctx context.Context, txtID string, ids []string, at time.Time, chat, sender domain.JID) error

	// SendReaction envia uma reação. A montagem da mensagem no formato do
	// SDK é responsabilidade do adapter.
	SendReaction(ctx context.Context, txtID string, target domain.JID, reaction domain.Reaction) (domain.MessageSendResult, error)

	// RevokeMessage revoga ("apaga para todos") a mensagem messageID na
	// conversa target. Só revoga mensagem PRÓPRIA: o remetente é sempre o
	// JID vazio, como no histórico (`git show 41bc8e2^:handlers.go`, linha
	// 2825). Revogar mensagem de terceiro como admin de grupo existe no
	// SDK mas NUNCA esteve exposto nesta API, e acrescentá-lo seria
	// mudança de contrato público.
	//
	// O resultado devolvido traz o Timestamp que a sessão REALMENTE usou;
	// o ID que vem nele é o da mensagem de revogação, não o da mensagem
	// revogada — quem chama decide qual dos dois publicar.
	RevokeMessage(ctx context.Context, txtID string, target domain.JID, messageID string) (domain.MessageSendResult, error)

	// EditMessage substitui o texto da mensagem messageID na conversa
	// target por newText. ctxInfo, quando não nil, monta ContextInfo no
	// ExtendedTextMessage (citação e menções — F134). A montagem
	// (FutureProofMessage/ProtocolMessage MESSAGE_EDIT) é responsabilidade
	// do adapter.
	EditMessage(ctx context.Context, txtID string, target domain.JID, messageID, newText string, ctxInfo *domain.EditContextInfo) (domain.MessageSendResult, error)

	// SendPollVote votes on an existing poll. The vote is encrypted with a
	// secret derived from the original poll message, so the caller must
	// provide the full poll identity (chat, sender, id, timestamp) via
	// payload. CAP-48 adds this to the same port as MarkRead, SendReaction,
	// RevokeMessage and EditMessage: voting is an OPERATION on an existing
	// message, not creation of a new one.
	SendPollVote(ctx context.Context, txtID string, target domain.JID, payload domain.PollVotePayload, id string) (domain.MessageSendResult, error)
}
