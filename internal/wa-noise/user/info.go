// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"context"
	"strings"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// SetStatusMessage updates the current user's status text, which is shown in the "About" section in the user profile.
//
// This is different from the ephemeral status broadcast messages. Use SendMessage to types.StatusBroadcastJID to send
// such messages.
func SetStatusMessage(ctx context.Context, t Transport, msg string) error {
	_, err := t.SendIQ(ctx, IQ{
		Namespace: statusIQNamespace,
		Type:      IQSet,
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
func IsOnWhatsApp(ctx context.Context, t Transport, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	jids := make([]types.JID, len(phones))
	for i := range jids {
		jids[i] = types.NewJID(phones[i], types.LegacyUserServer)
	}
	list, err := USync(ctx, t, jids, ModeQuery, ContextInteractive, []waBinary.Node{
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
		info.VerifiedName, err = ParseVerifiedName(child.GetChildByTag(businessNodeTag))
		if err != nil {
			t.Log().Warnf("Failed to parse %s's verified name details: %v", jid, err)
		}
		contactNode := child.GetChildByTag(contactNodeTag)
		info.IsIn = contactNode.AttrGetter().String("type") == contactTypeIn
		contactQuery, _ := contactNode.Content.([]byte)
		info.Query = strings.TrimSuffix(string(contactQuery), querySuffix)
		output = append(output, info)
	}
	return output, nil
}

// IsValidLIDMapping diz se o par (PN, LID) e' um mapeamento que
// store.LIDStore.PutManyLIDMappings aceita: PN em s.whatsapp.net e LID em
// lid, ambos nao vazios.
//
// A consulta usync aceita LID como entrada (ver o `case` de HiddenUserServer em
// USync), entao o `jid` de <user> na resposta pode ser um LID — e nesse caso o
// par montado seria LID/LID, nao PN/LID. Filtrar aqui evita entregar ao store
// um mapeamento que ele so' pode descartar.
//
// Regressao do lote 7 da Fase E: sem esta guarda, GetInfo entregava pares
// LID/LID a PutManyLIDMappings, que os descartava logando erro.
func IsValidLIDMapping(pn, lid types.JID) bool {
	return pn.Server == types.DefaultUserServer && lid.Server == types.HiddenUserServer &&
		pn.User != "" && lid.User != ""
}

// GetInfo gets basic user info (avatar, status, verified business name, device list).
func GetInfo(ctx context.Context, t Transport, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	list, err := USync(ctx, t, jids, ModeFull, ContextBackground, []waBinary.Node{
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
		verifiedName, err := ParseVerifiedName(child.GetChildByTag(businessNodeTag))
		if err != nil {
			t.Log().Warnf("Failed to parse %s's verified name details: %v", jid, err)
		}
		info.Status = NodeContentString(child.GetChildByTag(statusNodeTag))
		info.PictureID, _ = child.GetChildByTag(pictureNodeTag).Attrs["id"].(string)
		info.Devices = ParseDeviceList(jid, child.GetChildByTag(devicesNodeTag))

		lidTag := child.GetChildByTag(lidNodeTag)
		info.LID = lidTag.AttrGetter().OptionalJIDOrEmpty("val")

		if IsValidLIDMapping(jid, info.LID) {
			mappings = append(mappings, store.LIDMapping{PN: jid, LID: info.LID})
		}

		if verifiedName != nil {
			UpdateBusinessName(ctx, t, jid, info.LID, nil, verifiedName.Details.GetVerifiedName())
		}
		respData[jid] = info
	}

	err = t.Store().LIDs.PutManyLIDMappings(ctx, mappings)
	if err != nil {
		// not worth returning on the error, instead just post a log
		t.Log().Errorf("Failed to place LID mappings from USync call: %v", err)
	}

	return respData, nil
}

// HandleHistoricalPushNames grava no contact store os push names que vieram num
// history sync.
func HandleHistoricalPushNames(ctx context.Context, t Transport, names []*waHistorySync.Pushname) {
	if t.Store().Contacts == nil {
		return
	}
	t.Log().Infof("Updating contact store with %d push names from history sync", len(names))
	for _, u := range names {
		if u.GetPushname() == "-" {
			continue
		}
		var changed bool
		if jid, err := types.ParseJID(u.GetID()); err != nil {
			t.Log().Warnf("Failed to parse user ID '%s' in push name history sync: %v", u.GetID(), err)
		} else if changed, _, err = t.Store().Contacts.PutPushName(ctx, jid, u.GetPushname()); err != nil {
			t.Log().Warnf("Failed to store push name of %s from history sync: %v", jid, err)
		} else if changed {
			t.Log().Debugf("Got push name %s for %s in history sync", u.GetPushname(), jid)
		}
	}
}

// UpdatePushName grava o push name de um usuario e, se mudou, replica para o
// JID alternativo (LID <-> PN) e emite events.PushName.
func UpdatePushName(
	ctx context.Context, t Transport,
	jid, jidAlt types.JID, messageInfo *types.MessageInfo, name string,
) {
	if t.Store().Contacts == nil {
		return
	}
	jid = jid.ToNonAD()
	changed, previousName, err := t.Store().Contacts.PutPushName(ctx, jid, name)
	if err != nil {
		t.Log().Errorf("Failed to save push name of %s in device store: %v", jid, err)
	} else if changed {
		jidAlt = jidAlt.ToNonAD()
		if jidAlt.IsEmpty() {
			jidAlt, _ = t.Store().GetAltJID(ctx, jid)
		}
		if !jidAlt.IsEmpty() {
			_, _, err = t.Store().Contacts.PutPushName(ctx, jidAlt, name)
			if err != nil {
				t.Log().Errorf("Failed to save push name of %s in device store: %v", jidAlt, err)
			}
		}
		t.Log().Debugf("Push name of %s changed from %s to %s, dispatching event", jid, previousName, name)
		t.DispatchEvent(&events.PushName{
			JID:         jid,
			JIDAlt:      jidAlt,
			Message:     messageInfo,
			OldPushName: previousName,
			NewPushName: name,
		})
	}
}
