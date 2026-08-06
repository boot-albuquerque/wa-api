// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"wa-api/internal/waclient/appstate"
	"wa-api/internal/waclient/proto/waServerSync"
	"wa-api/internal/waclient/store"
	"wa-api/internal/waclient/types"
	"wa-api/internal/waclient/types/events"
)

func (cli *Client) collectEventsToDispatch(
	ctx context.Context,
	name appstate.WAPatchName,
	mutations []appstate.Mutation,
	fullSync bool,
	eventsToDispatch *[]any,
) error {
	if name == appstate.WAPatchCriticalUnblockLow && fullSync && !cli.EmitAppStateEventsOnFullSync {
		var contacts []store.ContactEntry
		mutations, contacts = cli.filterContacts(mutations)
		cli.Log.Debugf("Mass inserting app state snapshot with %d contacts into the store", len(contacts))
		err := cli.Store.Contacts.PutAllContactNames(ctx, contacts)
		if err != nil {
			// This is a fairly serious failure, so just abort the whole thing
			return fmt.Errorf("failed to update contact store with data from snapshot: %v", err)
		}
	}
	for _, mutation := range mutations {
		if eventsToDispatch != nil && mutation.Operation == waServerSync.SyncdMutation_SET {
			*eventsToDispatch = append(*eventsToDispatch, &events.AppState{Index: mutation.Index, SyncActionValue: mutation.Action})
		}
		evt := cli.dispatchAppState(ctx, name, mutation, fullSync)
		if eventsToDispatch != nil && evt != nil {
			*eventsToDispatch = append(*eventsToDispatch, evt)
		}
	}
	return nil
}

func (cli *Client) filterContacts(mutations []appstate.Mutation) ([]appstate.Mutation, []store.ContactEntry) {
	filteredMutations := mutations[:0]
	contacts := make([]store.ContactEntry, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.Index[0] == "contact" && len(mutation.Index) > 1 {
			jid, _ := types.ParseJID(mutation.Index[1])
			act := mutation.Action.GetContactAction()
			contacts = append(contacts, store.ContactEntry{
				JID:       jid,
				FirstName: act.GetFirstName(),
				FullName:  act.GetFullName(),
			})
		} else {
			filteredMutations = append(filteredMutations, mutation)
		}
	}
	return filteredMutations, contacts
}

func (cli *Client) dispatchAppState(ctx context.Context, name appstate.WAPatchName, mutation appstate.Mutation, fullSync bool) (eventToDispatch any) {
	logLevel := zerolog.TraceLevel
	log := zerolog.Ctx(ctx)
	if cli.AppStateDebugLogs && log.GetLevel() != zerolog.TraceLevel {
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

	if len(mutation.Index) == 1 && mutation.Index[0] == appstate.IndexNCTSaltSync {
		var err error
		if mutation.Operation == waServerSync.SyncdMutation_SET {
			err = cli.storeNCTSalt(ctx, mutation.Action.GetNctSaltSyncAction().GetSalt())
		} else if mutation.Operation == waServerSync.SyncdMutation_REMOVE {
			err = cli.clearNCTSalt(ctx)
		}
		if err != nil {
			cli.Log.Warnf("Failed to update NCT salt from app state mutation: %v", err)
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
		if cli.Store.ChatSettings != nil {
			storeUpdateError = cli.Store.ChatSettings.PutMutedUntil(ctx, jid, mutedUntil)
		}
	case appstate.IndexPin:
		act := mutation.Action.GetPinAction()
		eventToDispatch = &events.Pin{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		if cli.Store.ChatSettings != nil {
			storeUpdateError = cli.Store.ChatSettings.PutPinned(ctx, jid, act.GetPinned())
		}
	case appstate.IndexArchive:
		act := mutation.Action.GetArchiveChatAction()
		eventToDispatch = &events.Archive{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		if cli.Store.ChatSettings != nil {
			storeUpdateError = cli.Store.ChatSettings.PutArchived(ctx, jid, act.GetArchived())
		}
	case appstate.IndexContact:
		act := mutation.Action.GetContactAction()
		eventToDispatch = &events.Contact{JID: jid, Timestamp: ts, Action: act, FromFullSync: fullSync}
		if cli.Store.Contacts != nil {
			storeUpdateError = cli.Store.Contacts.PutContactName(ctx, jid, act.GetFirstName(), act.GetFullName())
		}
	case appstate.IndexClearChat:
		act := mutation.Action.GetClearChatAction()
		var deleteMedia bool
		// TODO what's index 2 here?
		if len(mutation.Index) > 3 && mutation.Index[3] == "1" {
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
		if len(mutation.Index) > 2 && mutation.Index[2] == "1" {
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
		if len(mutation.Index) < 5 {
			return
		}
		evt := events.Star{
			ChatJID:      jid,
			MessageID:    mutation.Index[2],
			Timestamp:    ts,
			Action:       mutation.Action.GetStarAction(),
			IsFromMe:     mutation.Index[3] == "1",
			FromFullSync: fullSync,
		}
		if mutation.Index[4] != "0" {
			evt.SenderJID, _ = types.ParseJID(mutation.Index[4])
		}
		eventToDispatch = &evt
	case appstate.IndexDeleteMessageForMe:
		if len(mutation.Index) < 5 {
			return
		}
		evt := events.DeleteForMe{
			ChatJID:      jid,
			MessageID:    mutation.Index[2],
			Timestamp:    ts,
			Action:       mutation.Action.GetDeleteMessageForMeAction(),
			IsFromMe:     mutation.Index[3] == "1",
			FromFullSync: fullSync,
		}
		if mutation.Index[4] != "0" {
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
		cli.Store.PushName = mutation.Action.GetPushNameSetting().GetName()
		err := cli.Store.Save(ctx)
		if err != nil {
			cli.Log.Errorf("Failed to save device store after updating push name: %v", err)
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
		act := mutation.Action.GetLabelEditAction()
		eventToDispatch = &events.LabelEdit{
			Timestamp:    ts,
			LabelID:      mutation.Index[1],
			Action:       act,
			FromFullSync: fullSync,
		}
	case appstate.IndexLabelAssociationChat:
		if len(mutation.Index) < 3 {
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
		if len(mutation.Index) < 6 {
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
		cli.Log.Errorf("Failed to update device store after app state mutation: %v", storeUpdateError)
	}
	return
}
