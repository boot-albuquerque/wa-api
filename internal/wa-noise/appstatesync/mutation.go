// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstatesync

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waServerSync"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// DispatchMutation aplica uma mutacao ao store e devolve o evento
// correspondente, ou nil quando a mutacao nao gera evento (operacao diferente
// de SET, indice desconhecido, ou indice curto demais).
//
// Todo acesso posicional a mutation.Index e' guardado por um comprimento
// minimo: o indice vem do servidor e um indice curto demais causaria panico
// dentro do handler de nos, que roda em goroutine sem recover.
func DispatchMutation(ctx context.Context, t Transport, name appstate.WAPatchName, mutation appstate.Mutation, fullSync bool) (eventToDispatch any) {
	logMutation(ctx, t, name, mutation)

	if len(mutation.Index) == 1 && mutation.Index[0] == appstate.IndexNCTSaltSync {
		var err error
		if mutation.Operation == waServerSync.SyncdMutation_SET {
			err = t.StoreNCTSalt(ctx, mutation.Action.GetNctSaltSyncAction().GetSalt())
		} else if mutation.Operation == waServerSync.SyncdMutation_REMOVE {
			err = t.ClearNCTSalt(ctx)
		}
		if err != nil {
			t.Log().Warnf("Failed to update NCT salt from app state mutation: %v", err)
		}
		return
	}

	if mutation.Operation != waServerSync.SyncdMutation_SET {
		return
	}

	var jid types.JID
	if len(mutation.Index) > 1 {
		jid, _ = types.ParseJID(mutation.Index[1])
	}
	ts := time.UnixMilli(mutation.Action.GetTimestamp())

	var storeUpdateError error
	switch mutation.Index[0] {
	case appstate.IndexMute:
		act := mutation.Action.GetMuteAction()
		eventToDispatch = &events.Mute{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		var mutedUntil time.Time
		if act.GetMuted() {
			if act.GetMuteEndTimestamp() < 0 {
				mutedUntil = store.MutedForever
			} else {
				mutedUntil = time.UnixMilli(act.GetMuteEndTimestamp())
			}
		}
		if t.Store().ChatSettings != nil {
			storeUpdateError = t.Store().ChatSettings.PutMutedUntil(ctx, jid, mutedUntil)
		}
	case appstate.IndexPin:
		act := mutation.Action.GetPinAction()
		eventToDispatch = &events.Pin{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		if t.Store().ChatSettings != nil {
			storeUpdateError = t.Store().ChatSettings.PutPinned(ctx, jid, act.GetPinned())
		}
	case appstate.IndexArchive:
		act := mutation.Action.GetArchiveChatAction()
		eventToDispatch = &events.Archive{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		if t.Store().ChatSettings != nil {
			storeUpdateError = t.Store().ChatSettings.PutArchived(ctx, jid, act.GetArchived())
		}
	case appstate.IndexContact:
		act := mutation.Action.GetContactAction()
		eventToDispatch = &events.Contact{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		if t.Store().Contacts != nil {
			storeUpdateError = t.Store().Contacts.PutContactName(ctx, jid, act.GetFirstName(), act.GetFullName())
		}
	case appstate.IndexClearChat:
		act := mutation.Action.GetClearChatAction()
		var deleteMedia bool
		// TODO what's index 2 here?
		if len(mutation.Index) >= indexMinLenClearChatMedia && mutation.Index[3] == indexTrue {
			deleteMedia = true
		}
		eventToDispatch = &events.ClearChat{
			JID:          jid,
			Timestamp:    ts,
			Action:       act,
			DeleteMedia:  deleteMedia,
			FromFullSync: fullSync,
		}
	case appstate.IndexDeleteChat:
		act := mutation.Action.GetDeleteChatAction()
		var deleteMedia bool
		if len(mutation.Index) >= indexMinLenDeleteChatMedia && mutation.Index[2] == indexTrue {
			deleteMedia = true
		}
		eventToDispatch = &events.DeleteChat{
			JID:          jid,
			Timestamp:    ts,
			Action:       act,
			DeleteMedia:  deleteMedia,
			FromFullSync: fullSync,
		}
	case appstate.IndexStar:
		if len(mutation.Index) < indexMinLenStar {
			return
		}
		evt := events.Star{
			ChatJID:      jid,
			MessageID:    mutation.Index[2],
			Timestamp:    ts,
			Action:       mutation.Action.GetStarAction(),
			IsFromMe:     mutation.Index[3] == indexTrue,
			FromFullSync: fullSync,
		}
		if mutation.Index[4] != indexSelfSender {
			evt.SenderJID, _ = types.ParseJID(mutation.Index[4])
		}
		eventToDispatch = &evt
	case appstate.IndexDeleteMessageForMe:
		if len(mutation.Index) < indexMinLenDeleteForMe {
			return
		}
		evt := events.DeleteForMe{
			ChatJID:      jid,
			MessageID:    mutation.Index[2],
			Timestamp:    ts,
			Action:       mutation.Action.GetDeleteMessageForMeAction(),
			IsFromMe:     mutation.Index[3] == indexTrue,
			FromFullSync: fullSync,
		}
		if mutation.Index[4] != indexSelfSender {
			evt.SenderJID, _ = types.ParseJID(mutation.Index[4])
		}
		eventToDispatch = &evt
	case appstate.IndexMarkChatAsRead:
		eventToDispatch = &events.MarkChatAsRead{
			JID:          jid,
			Timestamp:    ts,
			Action:       mutation.Action.GetMarkChatAsReadAction(),
			FromFullSync: fullSync,
		}
	case appstate.IndexSettingPushName:
		eventToDispatch = &events.PushNameSetting{
			Timestamp:    ts,
			Action:       mutation.Action.GetPushNameSetting(),
			FromFullSync: fullSync,
		}
		t.Store().PushName = mutation.Action.GetPushNameSetting().GetName()
		err := t.Store().Save(ctx)
		if err != nil {
			t.Log().Errorf("Failed to save device store after updating push name: %v", err)
		}
	case appstate.IndexSettingUnarchiveChats:
		eventToDispatch = &events.UnarchiveChatsSetting{
			Timestamp:    ts,
			Action:       mutation.Action.GetUnarchiveChatsSetting(),
			FromFullSync: fullSync,
		}
	case appstate.IndexUserStatusMute:
		eventToDispatch = &events.UserStatusMute{
			JID:          jid,
			Timestamp:    ts,
			Action:       mutation.Action.GetUserStatusMuteAction(),
			FromFullSync: fullSync,
		}
	case appstate.IndexLabelEdit:
		// O indice vem do servidor: sem esta guarda, um `label_edit` sem o
		// campo de labelID causaria panico (index out of range) dentro do
		// handler de nos, que roda em goroutine sem recover.
		if len(mutation.Index) < indexMinLenLabelEdit {
			return
		}
		act := mutation.Action.GetLabelEditAction()
		eventToDispatch = &events.LabelEdit{
			Timestamp:    ts,
			LabelID:      mutation.Index[1],
			Action:       act,
			FromFullSync: fullSync,
		}
	case appstate.IndexLabelAssociationChat:
		if len(mutation.Index) < indexMinLenLabelAssocChat {
			return
		}
		jid, _ = types.ParseJID(mutation.Index[2])
		act := mutation.Action.GetLabelAssociationAction()
		eventToDispatch = &events.LabelAssociationChat{
			JID:          jid,
			Timestamp:    ts,
			LabelID:      mutation.Index[1],
			Action:       act,
			FromFullSync: fullSync,
		}
	case appstate.IndexLabelAssociationMessage:
		if len(mutation.Index) < indexMinLenLabelAssocMessage {
			return
		}
		jid, _ = types.ParseJID(mutation.Index[2])
		act := mutation.Action.GetLabelAssociationAction()
		eventToDispatch = &events.LabelAssociationMessage{
			JID:          jid,
			Timestamp:    ts,
			LabelID:      mutation.Index[1],
			MessageID:    mutation.Index[3],
			Action:       act,
			FromFullSync: fullSync,
		}
	}
	if storeUpdateError != nil {
		t.Log().Errorf("Failed to update device store after app state mutation: %v", storeUpdateError)
	}
	return
}

// logMutation registra a mutacao recebida. Normalmente em nivel trace; com
// AppStateDebugLogs ligado sobe para debug, mas o conteudo da acao so' sai em
// trace (e' o dado mais volumoso e o mais sensivel).
func logMutation(ctx context.Context, t Transport, name appstate.WAPatchName, mutation appstate.Mutation) {
	logLevel := zerolog.TraceLevel
	log := zerolog.Ctx(ctx)
	if t.DebugLogs() && log.GetLevel() != zerolog.TraceLevel {
		logLevel = zerolog.DebugLevel
	}
	logEvt := log.WithLevel(logLevel).
		Str("patch_name", string(name)).
		Uint64("patch_version", mutation.PatchVersion).
		Stringer("operation", mutation.Operation).
		Int32("version", mutation.Version).
		Strs("index", mutation.Index).
		Hex("index_mac", mutation.IndexMAC).
		Hex("value_mac", mutation.ValueMAC)
	if logLevel == zerolog.TraceLevel {
		logEvt.Any("action", mutation.Action)
	}
	logEvt.Msg("Received app state mutation")
}
