// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/waclient/proto/waE2E"
	"wa-api/internal/waclient/proto/waHistorySync"
	"wa-api/internal/waclient/proto/waLidMigrationSyncPayload"
	"wa-api/internal/waclient/store"
	"wa-api/internal/waclient/types"
)

func (cli *Client) storeMessageSecret(ctx context.Context, info *types.MessageInfo, msg *waE2E.Message) {
	if msgSecret := msg.GetMessageContextInfo().GetMessageSecret(); len(msgSecret) > 0 {
		err := cli.Store.MsgSecrets.PutMessageSecret(ctx, info.Chat, info.Sender, info.ID, msgSecret)
		if err != nil {
			cli.Log.Errorf("Failed to store message secret key for %s: %v", info.ID, err)
		} else {
			cli.Log.Debugf("Stored message secret key for %s", info.ID)
		}
	}
}

func (cli *Client) storeHistoricalMessageSecrets(ctx context.Context, conversations []*waHistorySync.Conversation) {
	var secrets []store.MessageSecretInsert
	var privacyTokens []store.PrivacyToken
	ownID := cli.getOwnID().ToNonAD()
	if ownID.IsEmpty() {
		return
	}
	for _, conv := range conversations {
		chatJID, _ := types.ParseJID(conv.GetID())
		if chatJID.IsEmpty() {
			continue
		}
		if chatJID.Server == types.DefaultUserServer && conv.GetTcToken() != nil {
			privacyTokens = append(privacyTokens, store.PrivacyToken{
				User:            chatJID,
				Token:           conv.GetTcToken(),
				Timestamp:       time.Unix(int64(conv.GetTcTokenTimestamp()), 0),
				SenderTimestamp: time.Unix(int64(conv.GetTcTokenSenderTimestamp()), 0),
			})
		}
		for _, msg := range conv.GetMessages() {
			if secret := msg.GetMessage().GetMessageSecret(); secret != nil {
				var senderJID types.JID
				msgKey := msg.GetMessage().GetKey()
				if msgKey.GetFromMe() {
					senderJID = ownID
				} else if chatJID.Server == types.DefaultUserServer {
					senderJID = chatJID
				} else if msgKey.GetParticipant() != "" {
					senderJID, _ = types.ParseJID(msgKey.GetParticipant())
				} else if msg.GetMessage().GetParticipant() != "" {
					senderJID, _ = types.ParseJID(msg.GetMessage().GetParticipant())
				}
				if senderJID.IsEmpty() || msgKey.GetID() == "" {
					continue
				}
				secrets = append(secrets, store.MessageSecretInsert{
					Chat:   chatJID,
					Sender: senderJID,
					ID:     msgKey.GetID(),
					Secret: secret,
				})
			}
		}
	}
	if len(secrets) > 0 {
		cli.Log.Debugf("Storing %d message secret keys in history sync", len(secrets))
		err := cli.Store.MsgSecrets.PutMessageSecrets(ctx, secrets)
		if err != nil {
			cli.Log.Errorf("Failed to store message secret keys in history sync: %v", err)
		} else {
			cli.Log.Infof("Stored %d message secret keys from history sync", len(secrets))
		}
	}
	if len(privacyTokens) > 0 {
		cli.Log.Debugf("Storing %d privacy tokens in history sync", len(privacyTokens))
		err := cli.Store.PrivacyTokens.PutPrivacyTokens(ctx, privacyTokens...)
		if err != nil {
			cli.Log.Errorf("Failed to store privacy tokens in history sync: %v", err)
		} else {
			cli.Log.Infof("Stored %d privacy tokens from history sync", len(privacyTokens))
		}
	}
}

func (cli *Client) storeLIDSyncMessage(ctx context.Context, msg []byte) {
	var decoded waLidMigrationSyncPayload.LIDMigrationMappingSyncPayload
	err := proto.Unmarshal(msg, &decoded)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to unmarshal LID migration mapping sync payload")
		return
	}
	if cli.Store.LIDMigrationTimestamp == 0 && decoded.GetChatDbMigrationTimestamp() > 0 {
		cli.Store.LIDMigrationTimestamp = int64(decoded.GetChatDbMigrationTimestamp())
		err = cli.Store.Save(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).
				Int64("lid_migration_timestamp", cli.Store.LIDMigrationTimestamp).
				Msg("Failed to save chat DB LID migration timestamp")
		} else {
			zerolog.Ctx(ctx).Debug().
				Int64("lid_migration_timestamp", cli.Store.LIDMigrationTimestamp).
				Msg("Saved chat DB LID migration timestamp")
		}
	}
	lidPairs := make([]store.LIDMapping, len(decoded.PnToLidMappings))
	for i, mapping := range decoded.PnToLidMappings {
		lidPairs[i] = store.LIDMapping{
			LID: types.JID{User: strconv.FormatUint(mapping.GetAssignedLid(), 10), Server: types.HiddenUserServer},
			PN:  types.JID{User: strconv.FormatUint(mapping.GetPn(), 10), Server: types.DefaultUserServer},
		}
	}
	err = cli.Store.LIDs.PutManyLIDMappings(ctx, lidPairs)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).
			Int("pair_count", len(lidPairs)).
			Msg("Failed to store phone number to LID mappings from sync message")
	} else {
		zerolog.Ctx(ctx).Debug().
			Int("pair_count", len(lidPairs)).
			Msg("Stored PN-LID mappings from sync message")
	}
}

func (cli *Client) storeGlobalSettings(ctx context.Context, settings *waHistorySync.GlobalSettings) {
	if cli.Store.LIDMigrationTimestamp == 0 && settings.GetChatDbLidMigrationTimestamp() > 0 {
		cli.Store.LIDMigrationTimestamp = settings.GetChatDbLidMigrationTimestamp()
		err := cli.Store.Save(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).
				Int64("lid_migration_timestamp", cli.Store.LIDMigrationTimestamp).
				Msg("Failed to save chat DB LID migration timestamp")
		} else {
			zerolog.Ctx(ctx).Debug().
				Int64("lid_migration_timestamp", cli.Store.LIDMigrationTimestamp).
				Msg("Saved chat DB LID migration timestamp")
		}
	}
}

func (cli *Client) storeHistoricalPNLIDMappings(ctx context.Context, mappings []*waHistorySync.PhoneNumberToLIDMapping) {
	lidPairs := make([]store.LIDMapping, 0, len(mappings))
	for _, mapping := range mappings {
		pn, err := types.ParseJID(mapping.GetPnJID())
		if err != nil {
			zerolog.Ctx(ctx).Err(err).
				Str("pn_jid", mapping.GetPnJID()).
				Str("lid_jid", mapping.GetLidJID()).
				Msg("Failed to parse phone number from history sync")
			continue
		}
		if pn.Server == types.LegacyUserServer {
			pn.Server = types.DefaultUserServer
		}
		lid, err := types.ParseJID(mapping.GetLidJID())
		if err != nil {
			zerolog.Ctx(ctx).Err(err).
				Str("pn_jid", mapping.GetPnJID()).
				Str("lid_jid", mapping.GetLidJID()).
				Msg("Failed to parse LID from history sync")
			continue
		}
		lidPairs = append(lidPairs, store.LIDMapping{
			LID: lid,
			PN:  pn,
		})
	}
	err := cli.Store.LIDs.PutManyLIDMappings(ctx, lidPairs)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).
			Int("pair_count", len(lidPairs)).
			Msg("Failed to store phone number to LID mappings from history sync")
	} else {
		zerolog.Ctx(ctx).Debug().
			Int("pair_count", len(lidPairs)).
			Msg("Stored PN-LID mappings from history sync")
	}
}
