package whatsmeow

import (
	"context"
	wajid "wa-api/pkg/infra/wa-noise/jid"

	"wa-api/pkg/domain"
)

// GetGroupInfo devolve os metadados de um grupo.
func (a *GroupAdapter) GetGroupInfo(ctx context.Context, txtID string, group domain.JID) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return nil, err
	}
	return client.GetGroupInfo(ctx, jid)
}

// GetGroupInfoFromLink devolve os metadados a partir de um código de convite.
func (a *GroupAdapter) GetGroupInfoFromLink(ctx context.Context, txtID, code string) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	return client.GetGroupInfoFromLink(ctx, code)
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
	return client.GetGroupInviteLink(ctx, jid, false)
}

// ListJoinedGroups devolve os grupos de que a sessão participa e a contagem.
func (a *GroupAdapter) ListJoinedGroups(ctx context.Context, txtID string) (any, int, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, 0, err
	}
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, 0, err
	}
	return groups, len(groups), nil
}
