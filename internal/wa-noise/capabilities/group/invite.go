package group

import (
	"context"
	"errors"
	"fmt"
	"strings"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// GetInviteLink requests the invite link to the group from the WhatsApp servers.
//
// If reset is true, then the old invite link will be revoked and a new one generated.
func GetInviteLink(ctx context.Context, t Transport, jid types.JID, reset bool) (string, error) {
	iqErrs := t.IQErrors()
	iqType := IQGet
	if reset {
		iqType = IQSet
	}
	resp, err := sendIQ(ctx, t, iqType, jid, waBinary.Node{Tag: inviteTag})
	if errors.Is(err, iqErrs.NotAuthorized) {
		return "", t.WrapIQError(ErrInviteLinkUnauthorized, err)
	} else if errors.Is(err, iqErrs.NotFound) {
		return "", t.WrapIQError(ErrNotFound, err)
	} else if errors.Is(err, iqErrs.Forbidden) {
		return "", t.WrapIQError(ErrNotInGroup, err)
	} else if err != nil {
		return "", err
	}
	code, ok := resp.GetChildByTag(inviteTag).Attrs["code"].(string)
	if !ok {
		return "", fmt.Errorf("didn't find invite code in response")
	}
	return InviteLinkPrefix + code, nil
}

// GetInfoFromInvite gets the group info from an invite message.
//
// Note that this is specifically for invite messages, not invite links. Use GetInfoFromLink for resolving chat.whatsapp.com links.
func GetInfoFromInvite(ctx context.Context, t Transport, jid, inviter types.JID, code string, expiration int64) (*types.GroupInfo, error) {
	resp, err := sendIQ(ctx, t, IQGet, jid, waBinary.Node{
		Tag: "query",
		Content: []waBinary.Node{{
			Tag: addRequestTag,
			Attrs: waBinary.Attrs{
				"code":       code,
				"expiration": expiration,
				"admin":      inviter,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	groupNode, ok := resp.GetOptionalChildByTag(nodeTag)
	if !ok {
		return nil, t.ElementMissing(nodeTag, "response to invite group info query")
	}
	return ParseNode(t, &groupNode)
}

// JoinWithInvite joins a group using an invite message.
//
// Note that this is specifically for invite messages, not invite links. Use JoinWithLink for joining with chat.whatsapp.com links.
func JoinWithInvite(ctx context.Context, t Transport, jid, inviter types.JID, code string, expiration int64) error {
	_, err := sendIQ(ctx, t, IQSet, jid, waBinary.Node{
		Tag: "accept",
		Attrs: waBinary.Attrs{
			"code":       code,
			"expiration": expiration,
			"admin":      inviter,
		},
	})
	return err
}

// GetInfoFromLink resolves the given invite link and asks the WhatsApp servers for info about the group.
// This will not cause the user to join the group.
func GetInfoFromLink(ctx context.Context, t Transport, code string) (*types.GroupInfo, error) {
	iqErrs := t.IQErrors()
	code = strings.TrimPrefix(code, InviteLinkPrefix)
	resp, err := sendIQ(ctx, t, IQGet, types.GroupServerJID, waBinary.Node{
		Tag:   inviteTag,
		Attrs: waBinary.Attrs{"code": code},
	})
	if errors.Is(err, iqErrs.Gone) {
		return nil, t.WrapIQError(ErrInviteLinkRevoked, err)
	} else if errors.Is(err, iqErrs.NotAcceptable) {
		return nil, t.WrapIQError(ErrInviteLinkInvalid, err)
	} else if err != nil {
		return nil, err
	}
	groupNode, ok := resp.GetOptionalChildByTag(nodeTag)
	if !ok {
		return nil, t.ElementMissing(nodeTag, "response to group link info query")
	}
	return ParseNode(t, &groupNode)
}

// JoinWithLink joins the group using the given invite link.
func JoinWithLink(ctx context.Context, t Transport, code string) (types.JID, error) {
	iqErrs := t.IQErrors()
	code = strings.TrimPrefix(code, InviteLinkPrefix)
	resp, err := sendIQ(ctx, t, IQSet, types.GroupServerJID, waBinary.Node{
		Tag:   inviteTag,
		Attrs: waBinary.Attrs{"code": code},
	})
	if errors.Is(err, iqErrs.Gone) {
		return types.EmptyJID, t.WrapIQError(ErrInviteLinkRevoked, err)
	} else if errors.Is(err, iqErrs.NotAcceptable) {
		return types.EmptyJID, t.WrapIQError(ErrInviteLinkInvalid, err)
	} else if err != nil {
		return types.EmptyJID, err
	}
	membershipApprovalModeNode, ok := resp.GetOptionalChildByTag("membership_approval_request")
	if ok {
		return membershipApprovalModeNode.AttrGetter().JID("jid"), nil
	}
	groupNode, ok := resp.GetOptionalChildByTag(nodeTag)
	if !ok {
		return types.EmptyJID, t.ElementMissing(nodeTag, "response to group link join query")
	}
	return groupNode.AttrGetter().JID("jid"), nil
}
