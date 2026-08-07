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
		Namespace: statusIQNamespace,
		Type:      iqSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:     statusNodeTag,
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
	list, err := cli.usync(ctx, jids, usyncModeQuery, usyncContextInteractive, []waBinary.Node{
		{Tag: businessNodeTag, Content: []waBinary.Node{{Tag: verifiedNameNodeTag}}},
		{Tag: contactNodeTag},
	})
	if err != nil {
		return nil, err
	}
	output := make([]types.IsOnWhatsAppResponse, 0, len(jids))
	querySuffix := "@" + types.LegacyUserServer
	for _, child := range list.GetChildren() {
		jid, jidOK := child.Attrs["jid"].(types.JID)
		if child.Tag != usyncUserTag || !jidOK {
			continue
		}
		var info types.IsOnWhatsAppResponse
		info.JID = jid
		info.VerifiedName, err = parseVerifiedName(child.GetChildByTag(businessNodeTag))
		if err != nil {
			cli.Log.Warnf("Failed to parse %s's verified name details: %v", jid, err)
		}
		contactNode := child.GetChildByTag(contactNodeTag)
		info.IsIn = contactNode.AttrGetter().String("type") == contactTypeIn
		contactQuery, _ := contactNode.Content.([]byte)
		info.Query = strings.TrimSuffix(string(contactQuery), querySuffix)
		output = append(output, info)
	}
	return output, nil
}

// isValidLIDMapping diz se o par (PN, LID) é um mapeamento que
// `store.LIDStore.PutManyLIDMappings` aceita: PN em `s.whatsapp.net` e LID em
// `lid`, ambos não vazios.
//
// A consulta usync aceita LID como entrada (ver o `case` de `HiddenUserServer`
// em `usync`), então o `jid` de `<user>` na resposta pode ser um LID — e nesse
// caso o par montado seria LID/LID, não PN/LID. Filtrar aqui evita entregar ao
// store um mapeamento que ele só pode descartar.
func isValidLIDMapping(pn, lid types.JID) bool {
	return pn.Server == types.DefaultUserServer && lid.Server == types.HiddenUserServer &&
		pn.User != "" && lid.User != ""
}

// GetUserInfo gets basic user info (avatar, status, verified business name, device list).
func (cli *Client) GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	list, err := cli.usync(ctx, jids, usyncModeFull, usyncContextBackground, []waBinary.Node{
		{Tag: businessNodeTag, Content: []waBinary.Node{{Tag: verifiedNameNodeTag}}},
		{Tag: statusNodeTag},
		{Tag: pictureNodeTag},
		{Tag: devicesNodeTag, Attrs: waBinary.Attrs{"version": deviceListVersion}},
		{Tag: lidNodeTag},
	})
	if err != nil {
		return nil, err
	}
	respData := make(map[types.JID]types.UserInfo, len(jids))
	mappings := make([]store.LIDMapping, 0, len(jids))
	for _, child := range list.GetChildren() {
		jid, jidOK := child.Attrs["jid"].(types.JID)
		if child.Tag != usyncUserTag || !jidOK {
			continue
		}
		var info types.UserInfo
		verifiedName, err := parseVerifiedName(child.GetChildByTag(businessNodeTag))
		if err != nil {
			cli.Log.Warnf("Failed to parse %s's verified name details: %v", jid, err)
		}
		info.Status = nodeContentString(child.GetChildByTag(statusNodeTag))
		info.PictureID, _ = child.GetChildByTag(pictureNodeTag).Attrs["id"].(string)
		info.Devices = parseDeviceList(jid, child.GetChildByTag(devicesNodeTag))

		lidTag := child.GetChildByTag(lidNodeTag)
		info.LID = lidTag.AttrGetter().OptionalJIDOrEmpty("val")

		if isValidLIDMapping(jid, info.LID) {
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
		cli.Log.Errorf("Failed to place LID mappings from USync call: %v", err)
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
