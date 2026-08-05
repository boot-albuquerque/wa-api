package port

import (
	"context"

	"wa-api/pkg/domain"
)

// ChatOperations agrupa as operações avulsas sobre uma conversa que não
// pertencem nem ao envio de mensagens nem à administração de grupos.
type ChatOperations interface {
	SessionGuard

	// ArchiveChat arquiva ou desarquiva uma conversa.
	ArchiveChat(ctx context.Context, txtID string, chat domain.JID, archive bool) error

	// RejectCall rejeita uma chamada recebida.
	RejectCall(ctx context.Context, txtID string, from domain.JID, callID string) error

	// RequestUnavailableMessage pede ao par o reenvio de uma mensagem que
	// não pôde ser decifrada.
	RequestUnavailableMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string) (domain.UnavailableMessageAck, error)
}

// ProfileAccessProvider entrega o ProfileDataAccess da sessão.
//
// Substitui a fábrica func(*whatsmeow.Client) ProfileDataAccess que
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
