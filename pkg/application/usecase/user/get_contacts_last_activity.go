package user

import (
	"context"
	"strings"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetContactsLastActivityUseCase devolve o timestamp da última mensagem por
// chat, derivado do backfill local de message_history (HistorySync
// pós-pareamento + mensagens correntes). Não exige sessão whatsmeow ativa
// pro READ em si — é leitura de banco local — mas a NORMALIZAÇÃO de
// identidade (ver normalizeToLID) precisa da sessão pra consultar o mapa
// LID↔PN do whatsmeow; sem sessão ativa, devolve o dado cru sem normalizar
// (degrada, não falha).
type GetContactsLastActivityUseCase struct {
	activity appport.ChatActivityReader
	contacts appport.ContactDirectory
	logger   appport.Logger
}

// NewGetContactsLastActivityUseCase creates a new instance.
func NewGetContactsLastActivityUseCase(
	ar appport.ChatActivityReader,
	cd appport.ContactDirectory,
	logger appport.Logger,
) *GetContactsLastActivityUseCase {
	return &GetContactsLastActivityUseCase{activity: ar, contacts: cd, logger: logger}
}

// Execute retrieves last-activity-per-chat, normalizado pro espaço de
// identidade LID.
func (uc *GetContactsLastActivityUseCase) Execute(ctx context.Context, userID string) (map[string]time.Time, error) {
	raw, err := uc.activity.GetLastActivityByUser(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get contacts last activity", "error", err, "user_id", userID)
		return nil, err
	}
	result := uc.normalizeToLID(ctx, userID, raw)
	uc.logger.Info(ctx, "Retrieved contacts last activity", "user_id", userID, "count", len(result))
	return result, nil
}

// normalizeToLID reescreve as chaves `@s.whatsapp.net` de `raw` pro LID
// equivalente, quando o whatsmeow já conhece o mapeamento (populado pelo
// próprio app-state sync, sem custo de round-trip extra por contato — 1
// chamada em lote pro store local).
//
// Por quê: GetAllContacts (whatsmeow_contacts) devolve majoritariamente
// `@lid` no WhatsApp Multi-Device atual, mas message_history mistura `@lid`
// e `@s.whatsapp.net` (HistorySync preserva o JID original de cada
// conversa). Sem normalizar, o caller (wa-worker) tenta casar last-activity
// com o roster e a maioria dos contatos fica sem sinal — descoberto ao medir
// contra dados reais: 0/1141 contatos casaram por telefone, e só 5/284 por
// JID bruto, porque os dois lados vivem em espaços de identidade diferentes.
//
// PN sem LID conhecido mantém a chave original (`@s.whatsapp.net`) no
// resultado — não é descartado, só não normalizado; o caller ainda pode
// casar por telefone nesse caso residual.
func (uc *GetContactsLastActivityUseCase) normalizeToLID(
	ctx context.Context,
	userID string,
	raw map[string]time.Time,
) map[string]time.Time {
	pnJIDs := make([]domain.JID, 0, len(raw))
	for jid := range raw {
		if strings.HasSuffix(jid, "@s.whatsapp.net") {
			pnJIDs = append(pnJIDs, domain.JID(jid))
		}
	}
	if len(pnJIDs) == 0 {
		return raw
	}

	resolved, err := uc.contacts.GetManyLIDsForPNs(ctx, userID, pnJIDs)
	if err != nil {
		// Best-effort: sem sessão ativa (standby) ou erro do store, devolve
		// o dado cru — degrada pro comportamento pré-normalização, não falha
		// a chamada inteira por causa de um passo de enriquecimento.
		uc.logger.Error(ctx, "Failed to resolve LID mapping for last activity, returning unnormalized", "error", err, "user_id", userID)
		return raw
	}

	// Duas passadas deliberadas (não uma só): a ordem de iteração de um map
	// em Go é aleatória, então mesclar tudo numa passada faria o resultado
	// depender de qual entrada (a @lid direta ou a @s.whatsapp.net
	// resolvida) é vista primeiro — o `out[lidKey] = ts` da 1ª passada
	// nunca pode ser sobrescrito incondicionalmente pela 2ª.
	out := make(map[string]time.Time, len(raw))
	for jid, ts := range raw {
		if !strings.HasSuffix(jid, "@s.whatsapp.net") {
			out[jid] = ts // @lid, @g.us, formatos desconhecidos: passthrough.
		}
	}
	for jid, ts := range raw {
		if !strings.HasSuffix(jid, "@s.whatsapp.net") {
			continue
		}
		lid, ok := resolved[domain.JID(jid)]
		if !ok || lid == "" {
			out[jid] = ts // sem LID conhecido: mantém o PN original.
			continue
		}
		// GREATEST: mesmo contato pode ter entrada tanto por @lid direto
		// (chat que o HistorySync já trouxe em espaço LID) quanto por PN
		// resolvido pra esse LID — fica com o timestamp mais recente.
		lidKey := string(lid)
		if existing, has := out[lidKey]; !has || ts.After(existing) {
			out[lidKey] = ts
		}
	}
	return out
}
