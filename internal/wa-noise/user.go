// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"strings"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waHistorySync"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// SetStatusMessage updates the current user's status text, which is shown in the "About" section in the user profile.
//
// This is different from the ephemeral status broadcast messages. Use SendMessage to types.StatusBroadcastJID to send
// such messages.
func (cli *Client) SetStatusMessage(ctx context.Context, msg string) error {
	_, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "status",
		Type:      iqSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:     "status",
			Content: msg,
		}},
	})
	return err
}

// IsOnWhatsApp checks if the given phone numbers are registered on WhatsApp.
// The phone numbers should be in international format, including the `+` prefix.
func (cli *Client) IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	jids := make([]types.JID, len(phones))
	for i := range jids {
		jids[i] = types.NewJID(phones[i], types.LegacyUserServer)
	}
	list, err := cli.usync(ctx, jids, "query", "interactive", []waBinary.Node{
		{Tag: "business", Content: []waBinary.Node{{Tag: "verified_name"}}},
		{Tag: "contact"},
	})
	if err != nil {
		return nil, err
	}
	output := make([]types.IsOnWhatsAppResponse, 0, len(jids))
	querySuffix := "@" + types.LegacyUserServer
	for _, child := range list.GetChildren() {
		jid, jidOK := child.Attrs["jid"].(types.JID)
		if child.Tag != "user" || !jidOK {
			continue
		}
		var info types.IsOnWhatsAppResponse
		info.JID = jid
		info.VerifiedName, err = parseVerifiedName(child.GetChildByTag("business"))
		if err != nil {
			cli.Log.Warnf("Failed to parse %s's verified name details: %v", jid, err)
		}
		contactNode := child.GetChildByTag("contact")
		info.IsIn = contactNode.AttrGetter().String("type") == "in"
		contactQuery, _ := contactNode.Content.([]byte)
		info.Query = strings.TrimSuffix(string(contactQuery), querySuffix)
		output = append(output, info)
	}
	return output, nil
}

// GetUserInfo gets basic user info (avatar, status, verified business name, device list).
func (cli *Client) GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	list, err := cli.usync(ctx, jids, "full", "background", []waBinary.Node{
		{Tag: "business", Content: []waBinary.Node{{Tag: "verified_name"}}},
		{Tag: "status"},
		{Tag: "picture"},
		{Tag: "devices", Attrs: waBinary.Attrs{"version": "2"}},
		{Tag: "lid"},
	})
	if err != nil {
		return nil, err
	}
	respData := make(map[types.JID]types.UserInfo, len(jids))
	mappings := make([]store.LIDMapping, 0, len(jids))
	for _, child := range list.GetChildren() {
		jid, jidOK := child.Attrs["jid"].(types.JID)
		if child.Tag != "user" || !jidOK {
			continue
		}
		var info types.UserInfo
		verifiedName, err := parseVerifiedName(child.GetChildByTag("business"))
		if err != nil {
			cli.Log.Warnf("Failed to parse %s's verified name details: %v", jid, err)
		}
		status, _ := child.GetChildByTag("status").Content.([]byte)
		info.Status = string(status)
		info.PictureID, _ = child.GetChildByTag("picture").Attrs["id"].(string)
		info.Devices = parseDeviceList(jid, child.GetChildByTag("devices"))

		lidTag := child.GetChildByTag("lid")
		info.LID = lidTag.AttrGetter().OptionalJIDOrEmpty("val")

		if !info.LID.IsEmpty() {
			mappings = append(mappings, store.LIDMapping{PN: jid, LID: info.LID})
		}

		if verifiedName != nil {
			cli.updateBusinessName(ctx, jid, info.LID, nil, verifiedName.Details.GetVerifiedName())
		}
		respData[jid] = info
	}

	err = cli.Store.LIDs.PutManyLIDMappings(ctx, mappings)
	if err != nil {
		// not worth returning on the error, instead just post a log
		cli.Log.Errorf("Failed to place LID mappings from USync call")
	}

	return respData, nil
}

func (cli *Client) handleHistoricalPushNames(ctx context.Context, names []*waHistorySync.Pushname) {
	if cli.Store.Contacts == nil {
		return
	}
	cli.Log.Infof("Updating contact store with %d push names from history sync", len(names))
	for _, user := range names {
		if user.GetPushname() == "-" {
			continue
		}
		var changed bool
		if jid, err := types.ParseJID(user.GetID()); err != nil {
			cli.Log.Warnf("Failed to parse user ID '%s' in push name history sync: %v", user.GetID(), err)
		} else if changed, _, err = cli.Store.Contacts.PutPushName(ctx, jid, user.GetPushname()); err != nil {
			cli.Log.Warnf("Failed to store push name of %s from history sync: %v", jid, err)
		} else if changed {
			cli.Log.Debugf("Got push name %s for %s in history sync", user.GetPushname(), jid)
		}
	}
}

func (cli *Client) updatePushName(ctx context.Context, user, userAlt types.JID, messageInfo *types.MessageInfo, name string) {
	if cli.Store.Contacts == nil {
		return
	}
	user = user.ToNonAD()
	changed, previousName, err := cli.Store.Contacts.PutPushName(ctx, user, name)
	if err != nil {
		cli.Log.Errorf("Failed to save push name of %s in device store: %v", user, err)
	} else if changed {
		userAlt = userAlt.ToNonAD()
		if userAlt.IsEmpty() {
			userAlt, _ = cli.Store.GetAltJID(ctx, user)
		}
		if !userAlt.IsEmpty() {
			_, _, err = cli.Store.Contacts.PutPushName(ctx, userAlt, name)
			if err != nil {
				cli.Log.Errorf("Failed to save push name of %s in device store: %v", userAlt, err)
			}
		}
		cli.Log.Debugf("Push name of %s changed from %s to %s, dispatching event", user, previousName, name)
		cli.dispatchEvent(&events.PushName{
			JID:         user,
			JIDAlt:      userAlt,
			Message:     messageInfo,
			OldPushName: previousName,
			NewPushName: name,
		})
	}
}
