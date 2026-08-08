package user

import (
	"context"
	"fmt"
	waclient "wa-api/pkg/infra/wa-noise/client"
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
		return domain.Blocklist{}, err
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
		return domain.BlocklistUpdate{}, err
	}

	blocklist, err := client.UpdateBlocklist(ctx, resolved, action)
	if err != nil {
		return domain.BlocklistUpdate{}, err
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

// resolveBlocklistPNJID traduz um LID para o número de telefone, que é a
// forma que a lista de bloqueio aceita. Migrado literalmente de block_user.go,
// menos a asserção de tipo `client.(*wanoise.Client)`, que existia só
// porque o helper recebia interface{} — aqui o tipo é a interface waclient.Client
// e o Store é acessado pelo método Store().
func resolveBlocklistPNJID(ctx context.Context, client waclient.Client, jid types.JID) (types.JID, error) {
	jid = normalizeBlocklistJID(jid)
	switch jid.Server {
	case types.DefaultUserServer:
		return jid, nil
	case types.HiddenUserServer:
		pn, err := getCachedPNForLID(ctx, client, jid)
		if err != nil {
			return types.JID{}, err
		}
		return normalizeBlocklistJID(pn), nil
	default:
		return types.JID{}, fmt.Errorf("unsupported blocklist JID server %q", jid.Server)
	}
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
