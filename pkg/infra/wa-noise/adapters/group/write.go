package group

import (
	"context"
	"time"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"

	"wa-api/pkg/domain"

	wa "wa-api/internal/wa-noise"
)

// CreateGroup creates a group, community, or group-inside-community.
func (a *GroupAdapter) CreateGroup(ctx context.Context, txtID, name string, participants []domain.JID, opts domain.CreateGroupOpts) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jids, err := wajid.ToJIDs(participants)
	if err != nil {
		return nil, err
	}
	req := wa.ReqCreateGroup{Name: name, Participants: jids}
	if opts.IsParent {
		req.IsParent = true
	}
	if opts.LinkedParentJID != "" {
		parentJID, err := wajid.ToJID(opts.LinkedParentJID)
		if err != nil {
			return nil, err
		}
		req.LinkedParentJID = parentJID
	}
	return client.CreateGroup(ctx, req)
}

// JoinGroup entra num grupo por código de convite.
func (a *GroupAdapter) JoinGroup(ctx context.Context, txtID, code string) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	return client.JoinGroupWithLink(bgCtx, code)
}

// LeaveGroup sai de um grupo.
func (a *GroupAdapter) LeaveGroup(ctx context.Context, txtID string, group domain.JID) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	return client.LeaveGroup(bgCtx, jid)
}

// SetGroupName renomeia o grupo.
func (a *GroupAdapter) SetGroupName(ctx context.Context, txtID string, group domain.JID, name string) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupName(bgCtx, jid, name)
}

// SetGroupTopic define a descrição do grupo.
func (a *GroupAdapter) SetGroupTopic(ctx context.Context, txtID string, group domain.JID, topic string) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupTopic(bgCtx, jid, "", "", topic)
}

// SetGroupPhoto define a foto do grupo; photo nil remove a foto.
func (a *GroupAdapter) SetGroupPhoto(ctx context.Context, txtID string, group domain.JID, photo []byte) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	_, err = client.SetGroupPhoto(bgCtx, jid, photo)
	return err
}

// SetGroupAnnounce liga/desliga o modo somente-administradores.
func (a *GroupAdapter) SetGroupAnnounce(ctx context.Context, txtID string, group domain.JID, announce bool) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupAnnounce(bgCtx, jid, announce)
}

// SetGroupLocked tranca/destranca as configurações do grupo.
func (a *GroupAdapter) SetGroupLocked(ctx context.Context, txtID string, group domain.JID, locked bool) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupLocked(bgCtx, jid, locked)
}

// SetDisappearingTimer define o tempo de expiração das mensagens.
func (a *GroupAdapter) SetDisappearingTimer(ctx context.Context, txtID string, group domain.JID, d time.Duration, at time.Time) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	return client.SetDisappearingTimer(bgCtx, jid, d, at)
}

// SetJoinApprovalMode liga/desliga a exigência de aprovação para entrar.
func (a *GroupAdapter) SetJoinApprovalMode(ctx context.Context, txtID string, group domain.JID, mode bool) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupJoinApprovalMode(ctx, jid, mode)
}
