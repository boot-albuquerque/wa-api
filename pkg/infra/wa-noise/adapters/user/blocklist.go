package user

import (
	"context"
	"fmt"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/errmap"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"

	"wa-api/pkg/domain"

	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// GetBlocklist devolve a lista atual de bloqueados.
func (a *UserAdapter) GetBlocklist(ctx context.Context, txtID string) (domain.Blocklist, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.Blocklist{}, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	blocklist, err := client.GetBlocklist(ctxWithTimeout)
	if err != nil {
		// F204: a recusa do servidor do WhatsApp chegava ao cliente como 500.
		return domain.Blocklist{}, errmap.ClassifyIQ(err)
	}
	return toDomainBlocklist(blocklist), nil
}

// UpdateBlocklist bloqueia ou desbloqueia um alvo.
func (a *UserAdapter) UpdateBlocklist(ctx context.Context, txtID string, target domain.JID, block bool) (domain.BlocklistUpdate, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}
	parsed, err := wajid.ToJID(target)
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}
	requested := normalizeBlocklistJID(parsed)

	action := events.BlocklistChangeActionUnblock
	if block {
		action = events.BlocklistChangeActionBlock
	}

	resolved, err := resolveBlocklistPNJID(ctx, client, requested)
	if err != nil {
		// A resolução também faz info query: era por aqui que o 400 do
		// /user/block medido subia (F204).
		return domain.BlocklistUpdate{}, errmap.ClassifyIQ(err)
	}

	blocklist, err := client.UpdateBlocklist(ctx, resolved, action)
	if err != nil {
		// F204: medido em POST /user/block com número sem conta — o servidor
		// respondia 400 bad-request e nós devolvíamos 500.
		return domain.BlocklistUpdate{}, errmap.ClassifyIQ(err)
	}

	list := toDomainBlocklist(blocklist)
	return domain.BlocklistUpdate{
		ResolvedJID:  domain.JID(resolved.String()),
		RequestedJID: domain.JID(requested.String()),
		Entries:      list.JIDs,
		DHash:        list.DHash,
	}, nil
}

// toDomainBlocklist converte a lista do SDK, tolerando nil como o use case
// tolerava.
func toDomainBlocklist(blocklist *types.Blocklist) domain.Blocklist {
	out := domain.Blocklist{JIDs: []string{}}
	if blocklist == nil {
		return out
	}
	out.JIDs = make([]string, len(blocklist.JIDs))
	for i, jid := range blocklist.JIDs {
		out.JIDs[i] = jid.String()
	}
	out.DHash = blocklist.DHash
	return out
}

// normalizeBlocklistJID descarta agente/dispositivo e troca o servidor legado
// pelo padrão. Migrado literalmente de block_user.go.
func normalizeBlocklistJID(jid types.JID) types.JID {
	jid = jid.ToNonAD()
	if jid.Server == types.LegacyUserServer {
		jid.Server = types.DefaultUserServer
	}
	return jid
}

// resolveBlocklistPNJID normaliza o JID que vai para `<item jid=…>`.
//
// F278/LIB-02: o WhatsApp migrou a escrita da blocklist para endereçamento
// por LID — o `<item>` correto já usa `@lid`, não `@s.whatsapp.net`. Até
// 2026-08-27 esta função ia no sentido ERRADO num dos dois casos: um LID já
// recebido era traduzido de volta para PN antes de enviar, produzindo a
// forma pré-migração que o servidor recusa com `400 bad-request` em
// `client.UpdateBlocklist`.
//
// Um LID que chega aqui já é a forma que o protocolo exige — não há PN
// nenhum a resolver, e a chamada correspondente foi REMOVIDA (não só
// contornada): manter a chamada e ignorar o resultado esconderia a mesma
// causa atrás de um código que parece cauteloso.
//
// Um PN (o caso mais comum — a maioria dos chamadores manda telefone, não
// LID) é a metade que só ficou completa nesta revisão: tenta-se resolver o
// LID correspondente no store local (`getCachedLIDForPN`, o par de
// `getCachedPNForLID`); se o mapeamento já estiver em cache — normalmente
// está, para qualquer contacto que já trocou mensagem ou já apareceu numa
// sincronização —, o `<item>` sai em `@lid`. Sem mapeamento em cache, cai
// para o PN tal como veio: pior que a forma correta, mas nunca pior que o
// comportamento anterior a esta correção.
//
// Isto corrige `unblock` sempre que o LID (do parâmetro, ou resolvido a
// partir do PN) estiver disponível: o stanza de unblock não leva `pn_jid`,
// só `jid=…@lid`. NÃO corrige `block`: o WhatsApp exige um `pn_jid`
// adicional nesse caso, que `internal/wa-noise` ainda não emite (LIB-02,
// correção "completa" pendente — porta de whatsmeow `8d023aa973`).
func resolveBlocklistPNJID(ctx context.Context, client waclient.Client, jid types.JID) (types.JID, error) {
	jid = normalizeBlocklistJID(jid)
	switch jid.Server {
	case types.HiddenUserServer:
		return jid, nil
	case types.DefaultUserServer:
		if lid, err := getCachedLIDForPN(ctx, client, jid); err == nil {
			return lid, nil
		}
		return jid, nil
	default:
		return types.JID{}, fmt.Errorf("unsupported blocklist JID server %q", jid.Server)
	}
}

// getCachedLIDForPN é o par, no sentido PN→LID, de getCachedPNForLID
// (abaixo). Erro devolvido, nunca pânico: o chamador trata "sem mapeamento
// em cache" como esperado, não excepcional — é o caso de um contacto que
// nunca trocou mensagem nem apareceu numa sincronização.
func getCachedLIDForPN(ctx context.Context, client waclient.Client, jid types.JID) (types.JID, error) {
	store := client.Store()
	if store == nil || store.LIDs == nil {
		return types.JID{}, fmt.Errorf("PN-to-LID mapping store is not available")
	}
	lid, err := store.LIDs.GetLIDForPN(ctx, jid)
	if err != nil {
		return types.JID{}, fmt.Errorf("could not resolve LID JID for phone number %s: %w", jid, err)
	}
	if lid.IsEmpty() {
		return types.JID{}, fmt.Errorf("could not resolve LID JID for phone number %s", jid)
	}
	return lid, nil
}

func getCachedPNForLID(ctx context.Context, client waclient.Client, jid types.JID) (types.JID, error) {
	store := client.Store()
	if store == nil || store.LIDs == nil {
		return types.JID{}, fmt.Errorf("LID-to-PN mapping store is not available")
	}
	pn, err := store.LIDs.GetPNForLID(ctx, jid)
	if err != nil {
		return types.JID{}, fmt.Errorf("could not resolve phone-number JID for LID %s: %w", jid, err)
	}
	if pn.IsEmpty() {
		return types.JID{}, fmt.Errorf("could not resolve phone-number JID for LID %s", jid)
	}
	return pn, nil
}
