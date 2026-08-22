package port

import (
	"context"

	"wa-api/pkg/domain"
)

// As operações avulsas sobre uma conversa vivem em portas SEPARADAS, uma por
// capacidade (decisão 80).
//
// Antes eram uma interface só. O problema apareceu quando um SEGUNDO transporte
// passou a existir: o adaptador headless dirige a SPA e sabe arquivar, mas
// `RequestUnavailableMessage` não faz sentido nenhum ali — é pedir ao par o
// reenvio de uma mensagem que não pôde ser DECIFRADA, e quem dirige a página
// não decifra nada, a página já entrega texto. Com uma interface única, esse
// adaptador teria de devolver "não suportado" para satisfazer o compilador:
// satisfazer o TIPO enquanto mente sobre a CAPACIDADE.
//
// Portas por capacidade removem a mentira. Quem precisa de arquivar pede
// ChatArchiver, e um transporte que não rejeita chamadas simplesmente não
// implementa CallRejecter — a composição escolhe apenas contratos realmente
// implementados, e a assimetria fica visível no tipo em vez de escondida num
// erro em tempo de execução.

// ChatArchiver arquiva e desarquiva conversas.
type ChatArchiver interface {
	SessionGuard

	// ArchiveChat arquiva ou desarquiva uma conversa.
	ArchiveChat(ctx context.Context, txtID string, chat domain.JID, archive bool) error
}

// CallRejecter rejeita chamadas recebidas.
type CallRejecter interface {
	SessionGuard

	// RejectCall rejeita uma chamada recebida.
	RejectCall(ctx context.Context, txtID string, from domain.JID, callID string) error
}

// UnavailableMessageRequester pede o reenvio de mensagem indecifrável.
//
// É a mais específica das três por natureza: só faz sentido num transporte que
// FAÇA a decifração, e portanto não é esperado que todo transporte a satisfaça.
type UnavailableMessageRequester interface {
	SessionGuard

	// RequestUnavailableMessage pede ao par o reenvio de uma mensagem que
	// não pôde ser decifrada.
	RequestUnavailableMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string) (domain.UnavailableMessageAck, error)
}

// ChatOperations é a composição das três, mantida para o adaptador que as
// satisfaz TODAS declarar isso numa linha só. Não é o que um caso de uso deve
// pedir: pedir a composição é voltar a exigir capacidades que não se usa.
type ChatOperations interface {
	ChatArchiver
	CallRejecter
	UnavailableMessageRequester
}

// ProfileAccessProvider entrega o ProfileDataAccess da sessão.
//
// Substitui a fábrica func(*wanoise.Client) ProfileDataAccess que
// GetProfileUseCase recebia no construtor: a porta existia, mas o use case
// precisava do cliente concreto do SDK para poder construí-la, o que anulava
// o isolamento que ela deveria dar.
type ProfileAccessProvider interface {
	SessionGuard

	// ProfileAccess devolve o acesso ao perfil da sessão txtID.
	ProfileAccess(ctx context.Context, txtID string) (ProfileDataAccess, error)
}

// NewsletterReader lê as newsletters assinadas pela sessão.
type NewsletterReader interface {
	SessionGuard

	// ListSubscribed devolve as newsletters assinadas. O resultado é any
	// pelo mesmo motivo das portas de grupo: o valor atravessa o use case
	// opaco até a serialização.
	ListSubscribed(ctx context.Context, txtID string) (any, error)
}

// AppStateSyncer força o pull de um patch de app-state do servidor WhatsApp.
type AppStateSyncer interface {
	SessionGuard

	// SyncContactRoster puxa o patch critical_unblock_low (agenda de
	// contatos). mode: "if_unsynced" (no-op se já sincronizado) |
	// "incremental" (fetch barato, não apaga versão) | "full" (re-snapshot
	// completo, caro).
	SyncContactRoster(ctx context.Context, txtID string, mode string) error
}
