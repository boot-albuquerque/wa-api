// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package group

import (
	"context"
	"errors"
	"fmt"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// SetPhoto updates the group picture/icon of the given group on WhatsApp.
// The avatar should be a JPEG photo, other formats may be rejected with ErrInvalidImageFormat.
// The bytes can be nil to remove the photo. Returns the new picture ID.
func SetPhoto(ctx context.Context, t Transport, jid types.JID, avatar []byte) (string, error) {
	var content interface{}
	if avatar != nil {
		content = []waBinary.Node{{
			Tag:     "picture",
			Attrs:   waBinary.Attrs{"type": "image"},
			Content: avatar,
		}}
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: pictureIQNamespace,
		Type:      IQSet,
		To:        types.ServerJID,
		Target:    jid,
		Content:   content,
	})
	if errors.Is(err, t.IQErrors().NotAcceptable) {
		return "", t.WrapIQError(ErrInvalidImageFormat, err)
	} else if err != nil {
		return "", err
	}
	if avatar == nil {
		return photoRemovedID, nil
	}
	pictureID, ok := resp.GetChildByTag("picture").Attrs["id"].(string)
	if !ok {
		return "", fmt.Errorf("didn't find picture ID in response")
	}
	return pictureID, nil
}

// SetName updates the name (subject) of the given group on WhatsApp.
func SetName(ctx context.Context, t Transport, jid types.JID, name string) error {
	_, err := sendIQ(ctx, t, IQSet, jid, waBinary.Node{
		Tag:     subjectTag,
		Content: []byte(name),
	})
	return err
}

// SetTopic updates the topic (description) of the given group on WhatsApp.
//
// The previousID and newID fields are optional. If the previous ID is not specified, this will
// automatically fetch the current group info to find the previous topic ID. If the new ID is not
// specified, one will be generated with Client.GenerateMessageID().
func SetTopic(ctx context.Context, t Transport, jid types.JID, previousID, newID, topic string) error {
	if previousID == "" {
		// GetInfo com lock=true: e' o mesmo caminho do GetGroupInfo publico,
		// que era o que SetGroupTopic chamava antes da extracao.
		oldInfo, err := GetInfo(ctx, t, jid, true)
		if err != nil {
			return fmt.Errorf("failed to get old group info to update topic: %v", err)
		}
		previousID = oldInfo.TopicID
	}
	if newID == "" {
		newID = string(t.GenerateMessageID())
	}
	attrs := waBinary.Attrs{
		"id": newID,
	}
	if previousID != "" {
		attrs["prev"] = previousID
	}
	content := []waBinary.Node{{
		Tag:     descriptionBodyTag,
		Content: []byte(topic),
	}}
	if len(topic) == 0 {
		attrs["delete"] = "true"
		content = nil
	}
	_, err := sendIQ(ctx, t, IQSet, jid, waBinary.Node{
		Tag:     descriptionTag,
		Attrs:   attrs,
		Content: content,
	})
	return err
}

// SetLocked changes whether the group is locked (i.e. whether only admins can modify group info).
func SetLocked(ctx context.Context, t Transport, jid types.JID, locked bool) error {
	tag := lockedTag
	if !locked {
		tag = unlockedTag
	}
	_, err := sendIQ(ctx, t, IQSet, jid, waBinary.Node{Tag: tag})
	return err
}

// SetAnnounce changes whether the group is in announce mode (i.e. whether only admins can send messages).
func SetAnnounce(ctx context.Context, t Transport, jid types.JID, announce bool) error {
	tag := announcementTag
	if !announce {
		tag = notAnnouncementTag
	}
	_, err := sendIQ(ctx, t, IQSet, jid, waBinary.Node{Tag: tag})
	return err
}

// SetJoinApprovalMode sets the group join approval mode to 'on' or 'off'.
func SetJoinApprovalMode(ctx context.Context, t Transport, jid types.JID, mode bool) error {
	modeStr := joinStateOff
	if mode {
		modeStr = joinStateOn
	}

	content := waBinary.Node{
		Tag: membershipApprovalModeTag,
		Content: []waBinary.Node{
			{
				Tag:   joinTag,
				Attrs: waBinary.Attrs{"state": modeStr},
			},
		},
	}

	_, err := sendIQ(ctx, t, IQSet, jid, content)
	return err
}

// SetMemberAddMode sets the group member add mode to 'admin_add' or 'all_member_add'.
func SetMemberAddMode(ctx context.Context, t Transport, jid types.JID, mode types.GroupMemberAddMode) error {
	if mode != types.GroupMemberAddModeAdmin && mode != types.GroupMemberAddModeAllMember {
		return fmt.Errorf("invalid mode, must be %q or %q", types.GroupMemberAddModeAdmin, types.GroupMemberAddModeAllMember)
	}

	content := waBinary.Node{
		Tag:     memberAddModeTag,
		Content: []byte(mode),
	}

	_, err := sendIQ(ctx, t, IQSet, jid, content)
	return err
}

// SetDescription updates the group description.
func SetDescription(ctx context.Context, t Transport, jid types.JID, description string) error {
	content := waBinary.Node{
		Tag: descriptionTag,
		Content: []waBinary.Node{
			{
				Tag:     descriptionBodyTag,
				Content: []byte(description),
			},
		},
	}

	_, err := sendIQ(ctx, t, IQSet, jid, content)
	return err
}
