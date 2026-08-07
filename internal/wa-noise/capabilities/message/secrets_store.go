package message

import (
	"context"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waLidMigrationSyncPayload"
	"wa-api/internal/wa-noise/protocol/types"
)

// StoreSecret grava o segredo de mensagem embutido no MessageContextInfo, se
// houver.
func StoreSecret(ctx context.Context, t Transport, info *types.MessageInfo, msg *waE2E.Message) {
	if msgSecret := msg.GetMessageContextInfo().GetMessageSecret(); len(msgSecret) > 0 {
		err := t.Store().MsgSecrets.PutMessageSecret(ctx, info.Chat, info.Sender, info.ID, msgSecret)
		if err != nil {
			t.Log().Errorf("Failed to store message secret key for %s: %v", info.ID, err)
		} else {
			t.Log().Debugf("Stored message secret key for %s", info.ID)
		}
	}
}

// StoreHistoricalSecrets varre as conversas de um history sync e grava os
// segredos de mensagem e os privacy tokens que elas trazem.
func StoreHistoricalSecrets(ctx context.Context, t Transport, conversations []*waHistorySync.Conversation) {
	var secrets []store.MessageSecretInsert
	var privacyTokens []store.PrivacyToken
	ownID := t.OwnID().ToNonAD()
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
		t.Log().Debugf("Storing %d message secret keys in history sync", len(secrets))
		err := t.Store().MsgSecrets.PutMessageSecrets(ctx, secrets)
		if err != nil {
			t.Log().Errorf("Failed to store message secret keys in history sync: %v", err)
		} else {
			t.Log().Infof("Stored %d message secret keys from history sync", len(secrets))
		}
	}
	if len(privacyTokens) > 0 {
		t.Log().Debugf("Storing %d privacy tokens in history sync", len(privacyTokens))
		err := t.Store().PrivacyTokens.PutPrivacyTokens(ctx, privacyTokens...)
		if err != nil {
			t.Log().Errorf("Failed to store privacy tokens in history sync: %v", err)
		} else {
			t.Log().Infof("Stored %d privacy tokens from history sync", len(privacyTokens))
		}
	}
}

// StoreLIDSyncMessage decodifica e grava o payload de migracao LID recebido
// como protocol message.
func StoreLIDSyncMessage(ctx context.Context, t Transport, msg []byte) {
	var decoded waLidMigrationSyncPayload.LIDMigrationMappingSyncPayload
	err := proto.Unmarshal(msg, &decoded)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to unmarshal LID migration mapping sync payload")
		return
	}
	dev := t.Store()
	if dev.LIDMigrationTimestamp == 0 && decoded.GetChatDbMigrationTimestamp() > 0 {
		dev.LIDMigrationTimestamp = int64(decoded.GetChatDbMigrationTimestamp())
		err = dev.Save(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).
				Int64("lid_migration_timestamp", dev.LIDMigrationTimestamp).
				Msg("Failed to save chat DB LID migration timestamp")
		} else {
			zerolog.Ctx(ctx).Debug().
				Int64("lid_migration_timestamp", dev.LIDMigrationTimestamp).
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
	err = dev.LIDs.PutManyLIDMappings(ctx, lidPairs)
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

// StoreGlobalSettings grava o timestamp de migracao LID vindo das
// configuracoes globais do history sync.
func StoreGlobalSettings(ctx context.Context, t Transport, settings *waHistorySync.GlobalSettings) {
	dev := t.Store()
	if dev.LIDMigrationTimestamp == 0 && settings.GetChatDbLidMigrationTimestamp() > 0 {
		dev.LIDMigrationTimestamp = settings.GetChatDbLidMigrationTimestamp()
		err := dev.Save(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).
				Int64("lid_migration_timestamp", dev.LIDMigrationTimestamp).
				Msg("Failed to save chat DB LID migration timestamp")
		} else {
			zerolog.Ctx(ctx).Debug().
				Int64("lid_migration_timestamp", dev.LIDMigrationTimestamp).
				Msg("Saved chat DB LID migration timestamp")
		}
	}
}

// StoreHistoricalPNLIDMappings grava os pares PN/LID que vieram no history sync.
func StoreHistoricalPNLIDMappings(ctx context.Context, t Transport, mappings []*waHistorySync.PhoneNumberToLIDMapping) {
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
	err := t.Store().LIDs.PutManyLIDMappings(ctx, lidPairs)
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
