package core

import (
	"context"

	"wa-api/internal/noise/capabilities/group"
	"wa-api/internal/noise/protocol/types"
)

// Fachada; ver o cabecalho de group.go.

// InviteLinkPrefix e' o MESMO valor que group.InviteLinkPrefix — constante, nao
// copia, para que comparacoes e concatenacoes nos dois lados nunca divirjam.
const InviteLinkPrefix = group.InviteLinkPrefix

// GetGroupInviteLink requests the invite link to the group from the WhatsApp servers.
//
// If reset is true, then the old invite link will be revoked and a new one generated.
func (cli *Client) GetGroupInviteLink(ctx context.Context, jid types.JID, reset bool) (string, error) {
	if cli == nil {
		return "", ErrClientIsNil
	}
	return group.GetInviteLink(ctx, cli.groupT(), jid, reset)
}

// GetGroupInfoFromInvite gets the group info from an invite message.
//
// Note that this is specifically for invite messages, not invite links. Use GetGroupInfoFromLink for resolving chat.whatsapp.com links.
func (cli *Client) GetGroupInfoFromInvite(ctx context.Context, jid, inviter types.JID, code string, expiration int64) (*types.GroupInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.GetInfoFromInvite(ctx, cli.groupT(), jid, inviter, code, expiration)
}

// JoinGroupWithInvite joins a group using an invite message.
//
// Note that this is specifically for invite messages, not invite links. Use JoinGroupWithLink for joining with chat.whatsapp.com links.
func (cli *Client) JoinGroupWithInvite(ctx context.Context, jid, inviter types.JID, code string, expiration int64) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.JoinWithInvite(ctx, cli.groupT(), jid, inviter, code, expiration)
}

// GetGroupInfoFromLink resolves the given invite link and asks the WhatsApp servers for info about the group.
// This will not cause the user to join the group.
func (cli *Client) GetGroupInfoFromLink(ctx context.Context, code string) (*types.GroupInfo, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.GetInfoFromLink(ctx, cli.groupT(), code)
}

// JoinGroupWithLink joins the group using the given invite link.
func (cli *Client) JoinGroupWithLink(ctx context.Context, code string) (types.JID, error) {
	if cli == nil {
		return types.EmptyJID, ErrClientIsNil
	}
	return group.JoinWithLink(ctx, cli.groupT(), code)
}
