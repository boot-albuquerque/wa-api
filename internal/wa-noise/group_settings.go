// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"fmt"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// SetGroupPhoto updates the group picture/icon of the given group on WhatsApp.
// The avatar should be a JPEG photo, other formats may be rejected with ErrInvalidImageFormat.
// The bytes can be nil to remove the photo. Returns the new picture ID.
func (cli *Client) SetGroupPhoto(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
	var content interface{}
	if avatar != nil {
		content = []waBinary.Node{{
			Tag:     "picture",
			Attrs:   waBinary.Attrs{"type": "image"},
			Content: avatar,
		}}
	}
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "w:profile:picture",
		Type:      iqSet,
		To:        types.ServerJID,
		Target:    jid,
		Content:   content,
	})
	if errors.Is(err, ErrIQNotAcceptable) {
		return "", wrapIQError(ErrInvalidImageFormat, err)
	} else if err != nil {
		return "", err
	}
	if avatar == nil {
		return groupPhotoRemovedID, nil
	}
	pictureID, ok := resp.GetChildByTag("picture").Attrs["id"].(string)
	if !ok {
		return "", fmt.Errorf("didn't find picture ID in response")
	}
	return pictureID, nil
}

// SetGroupName updates the name (subject) of the given group on WhatsApp.
func (cli *Client) SetGroupName(ctx context.Context, jid types.JID, name string) error {
	_, err := cli.sendGroupIQ(ctx, iqSet, jid, waBinary.Node{
		Tag:     groupSubjectTag,
		Content: []byte(name),
	})
	return err
}

// SetGroupTopic updates the topic (description) of the given group on WhatsApp.
//
// The previousID and newID fields are optional. If the previous ID is not specified, this will
// automatically fetch the current group info to find the previous topic ID. If the new ID is not
// specified, one will be generated with Client.GenerateMessageID().
func (cli *Client) SetGroupTopic(ctx context.Context, jid types.JID, previousID, newID, topic string) error {
	if previousID == "" {
		oldInfo, err := cli.GetGroupInfo(ctx, jid)
		if err != nil {
			return fmt.Errorf("failed to get old group info to update topic: %v", err)
		}
		previousID = oldInfo.TopicID
	}
	if newID == "" {
		newID = cli.GenerateMessageID()
	}
	attrs := waBinary.Attrs{
		"id": newID,
	}
	if previousID != "" {
		attrs["prev"] = previousID
	}
	content := []waBinary.Node{{
		Tag:     groupDescriptionBodyTag,
		Content: []byte(topic),
	}}
	if len(topic) == 0 {
		attrs["delete"] = "true"
		content = nil
	}
	_, err := cli.sendGroupIQ(ctx, iqSet, jid, waBinary.Node{
		Tag:     groupDescriptionTag,
		Attrs:   attrs,
		Content: content,
	})
	return err
}

// SetGroupLocked changes whether the group is locked (i.e. whether only admins can modify group info).
func (cli *Client) SetGroupLocked(ctx context.Context, jid types.JID, locked bool) error {
	tag := groupLockedTag
	if !locked {
		tag = groupUnlockedTag
	}
	_, err := cli.sendGroupIQ(ctx, iqSet, jid, waBinary.Node{Tag: tag})
	return err
}

// SetGroupAnnounce changes whether the group is in announce mode (i.e. whether only admins can send messages).
func (cli *Client) SetGroupAnnounce(ctx context.Context, jid types.JID, announce bool) error {
	tag := groupAnnouncementTag
	if !announce {
		tag = groupNotAnnouncementTag
	}
	_, err := cli.sendGroupIQ(ctx, iqSet, jid, waBinary.Node{Tag: tag})
	return err
}

// SetGroupJoinApprovalMode sets the group join approval mode to 'on' or 'off'.
func (cli *Client) SetGroupJoinApprovalMode(ctx context.Context, jid types.JID, mode bool) error {
	modeStr := groupJoinStateOff
	if mode {
		modeStr = groupJoinStateOn
	}

	content := waBinary.Node{
		Tag: groupMembershipApprovalModeTag,
		Content: []waBinary.Node{
			{
				Tag:   groupJoinTag,
				Attrs: waBinary.Attrs{"state": modeStr},
			},
		},
	}

	_, err := cli.sendGroupIQ(ctx, iqSet, jid, content)
	return err
}

// SetGroupMemberAddMode sets the group member add mode to 'admin_add' or 'all_member_add'.
func (cli *Client) SetGroupMemberAddMode(ctx context.Context, jid types.JID, mode types.GroupMemberAddMode) error {
	if mode != types.GroupMemberAddModeAdmin && mode != types.GroupMemberAddModeAllMember {
		return fmt.Errorf("invalid mode, must be %q or %q", types.GroupMemberAddModeAdmin, types.GroupMemberAddModeAllMember)
	}

	content := waBinary.Node{
		Tag:     groupMemberAddModeTag,
		Content: []byte(mode),
	}

	_, err := cli.sendGroupIQ(ctx, iqSet, jid, content)
	return err
}

// SetGroupDescription updates the group description.
func (cli *Client) SetGroupDescription(ctx context.Context, jid types.JID, description string) error {
	content := waBinary.Node{
		Tag: groupDescriptionTag,
		Content: []waBinary.Node{
			{
				Tag:     groupDescriptionBodyTag,
				Content: []byte(description),
			},
		},
	}

	_, err := cli.sendGroupIQ(ctx, iqSet, jid, content)
	return err
}
