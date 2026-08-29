package group

import (
	"context"

	"wa-api/pkg/domain"
	"wa-api/pkg/infra/noise/errmap"
	wajid "wa-api/pkg/infra/noise/mapping/jid"
)

// GetGroupInfo devolve os metadados de um grupo.
func (a *GroupAdapter) GetGroupInfo(ctx context.Context, txtID string, group domain.JID) (*domain.GroupInfo, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return nil, err
	}
	res, err := client.GetGroupInfo(ctx, jid)
	if err := errmap.ClassifyIQ(err); err != nil {
		return nil, err
	}
	return toDomainGroupInfo(res), nil
}

// GetGroupInfoFromLink devolve os metadados a partir de um código de convite.
func (a *GroupAdapter) GetGroupInfoFromLink(ctx context.Context, txtID, code string) (*domain.GroupInfo, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	res, err := client.GetGroupInfoFromLink(ctx, code)
	if err != nil {
		return nil, err
	}
	return toDomainGroupInfo(res), nil
}

// GetGroupInviteLink devolve o link de convite de um grupo.
func (a *GroupAdapter) GetGroupInviteLink(ctx context.Context, txtID string, group domain.JID) (string, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return "", err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return "", err
	}
	link, err := client.GetGroupInviteLink(ctx, jid, false)
	return link, errmap.ClassifyIQ(err)
}

// ListJoinedGroups devolve os grupos de que a sessão participa e a contagem.
func (a *GroupAdapter) ListJoinedGroups(ctx context.Context, txtID string) ([]*domain.GroupInfo, int, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, 0, err
	}
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, 0, err
	}
	// Não-nula mesmo vazia: `[]` e `null` são valores diferentes para todo
	// cliente, e só um dos dois se percorre sem verificação.
	out := make([]*domain.GroupInfo, 0, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		out = append(out, toDomainGroupInfo(g))
	}
	return out, len(out), nil
}

// GroupNames devolve o nome de cada grupo da sessão, por JID.
//
// Uma única chamada ao SDK, e não uma por grupo: a lista de conversas precisa
// de todos os nomes de uma vez, e GetGroupInfo por grupo custaria um
// round-trip por item.
//
// Grupo sem nome definido entra no mapa com string vazia, e não fica de fora:
// o chamador precisa distinguir "grupo que não conheço" de "grupo sem nome".
func (a *GroupAdapter) GroupNames(ctx context.Context, txtID string) (map[domain.JID]string, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.JID]string, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		out[domain.JID(g.JID.String())] = g.Name
	}
	return out, nil
}
