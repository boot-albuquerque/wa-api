package wanoise

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/group"
	"wa-api/internal/wa-noise/protocol/types"
)

// Fachada; ver o cabecalho de group.go.

// SetGroupPhoto updates the group picture/icon of the given group on WhatsApp.
// The avatar should be a JPEG photo, other formats may be rejected with ErrInvalidImageFormat.
// The bytes can be nil to remove the photo. Returns the new picture ID.
func (cli *Client) SetGroupPhoto(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
	if cli == nil {
		return "", ErrClientIsNil
	}
	return group.SetPhoto(ctx, cli.groupT(), jid, avatar)
}

// SetGroupName updates the name (subject) of the given group on WhatsApp.
func (cli *Client) SetGroupName(ctx context.Context, jid types.JID, name string) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.SetName(ctx, cli.groupT(), jid, name)
}

// SetGroupTopic updates the topic (description) of the given group on WhatsApp.
//
// The previousID and newID fields are optional. If the previous ID is not specified, this will
// automatically fetch the current group info to find the previous topic ID. If the new ID is not
// specified, one will be generated with Client.GenerateMessageID().
func (cli *Client) SetGroupTopic(ctx context.Context, jid types.JID, previousID, newID, topic string) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.SetTopic(ctx, cli.groupT(), jid, previousID, newID, topic)
}

// SetGroupLocked changes whether the group is locked (i.e. whether only admins can modify group info).
func (cli *Client) SetGroupLocked(ctx context.Context, jid types.JID, locked bool) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.SetLocked(ctx, cli.groupT(), jid, locked)
}

// SetGroupAnnounce changes whether the group is in announce mode (i.e. whether only admins can send messages).
func (cli *Client) SetGroupAnnounce(ctx context.Context, jid types.JID, announce bool) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.SetAnnounce(ctx, cli.groupT(), jid, announce)
}

// SetGroupJoinApprovalMode sets the group join approval mode to 'on' or 'off'.
func (cli *Client) SetGroupJoinApprovalMode(ctx context.Context, jid types.JID, mode bool) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.SetJoinApprovalMode(ctx, cli.groupT(), jid, mode)
}

// SetGroupMemberAddMode sets the group member add mode to 'admin_add' or 'all_member_add'.
func (cli *Client) SetGroupMemberAddMode(ctx context.Context, jid types.JID, mode types.GroupMemberAddMode) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.SetMemberAddMode(ctx, cli.groupT(), jid, mode)
}

// SetGroupDescription updates the group description.
func (cli *Client) SetGroupDescription(ctx context.Context, jid types.JID, description string) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return group.SetDescription(ctx, cli.groupT(), jid, description)
}
