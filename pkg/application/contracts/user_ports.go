package port

import (
	"context"
	"time"

	"wa-api/pkg/domain"
)

// ContactDirectory expõe as consultas sobre contatos e usuários WhatsApp.
type ContactDirectory interface {
	SessionGuard

	// IsOnWhatsApp verifica quais dos telefones informados têm conta.
	IsOnWhatsApp(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error)

	// GetUserInfo devolve os metadados dos JIDs informados.
	GetUserInfo(ctx context.Context, txtID string, jids []domain.JID) (any, error)

	// GetAllContacts devolve a agenda da sessão e a contagem, que o use
	// case usa para logar.
	GetAllContacts(ctx context.Context, txtID string) (any, int, error)

	// GetProfilePicture devolve o avatar de um contato, ou nil se não há.
	GetProfilePicture(ctx context.Context, txtID string, target domain.JID, preview bool) (*domain.AvatarInfo, error)

	// GetLIDForPN resolve o LID correspondente a um número de telefone.
	GetLIDForPN(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error)
}

// ChatActivityReader expõe o histórico de atividade por chat já persistido
// localmente (message_history) — em particular o backfill automático do
// HistorySync pós-pareamento. Não estende SessionGuard: é leitura de banco
// local, não chamada ao whatsmeow, então não exige sessão whatsmeow ativa
// (funciona mesmo com a sessão em standby).
type ChatActivityReader interface {
	// GetLastActivityByUser devolve, por JID de chat (string bruta — pode
	// incluir grupos/broadcasts, filtragem é do caller), o timestamp da
	// mensagem mais recente já persistida.
	GetLastActivityByUser(ctx context.Context, userID string) (map[string]time.Time, error)
}

// BlocklistManager expõe a lista de bloqueados.
type BlocklistManager interface {
	SessionGuard

	// GetBlocklist devolve a lista atual de bloqueados.
	GetBlocklist(ctx context.Context, txtID string) (domain.Blocklist, error)

	// UpdateBlocklist bloqueia (block=true) ou desbloqueia um alvo.
	//
	// A resolução do JID pedido para o JID que a lista de bloqueio de fato
	// usa — normalização de servidor legado, e tradução de LID para número
	// de telefone — é do adapter: é regra do SDK, não do domínio. Por isso o
	// resultado devolve os dois JIDs, e não só um.
	UpdateBlocklist(ctx context.Context, txtID string, target domain.JID, block bool) (domain.BlocklistUpdate, error)
}

// PrivacyManager expõe as configurações de privacidade da sessão.
type PrivacyManager interface {
	SessionGuard

	// GetPrivacySettings devolve as configurações atuais.
	GetPrivacySettings(ctx context.Context, txtID string) (any, error)

	// SetPrivacySetting altera uma configuração. A validação de name e
	// value é do domínio (domain.ValidatePrivacySetting) e acontece antes.
	SetPrivacySetting(ctx context.Context, txtID, name, value string) (any, error)
}
