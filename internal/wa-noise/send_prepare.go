// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/util/random"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waAICommon"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
)

const (
	// messageSecretSize is the length in bytes of the random message secret
	// generated for messages that need one (bot messages and messages that
	// carry a reporting token).
	messageSecretSize = 32
	// defaultBotPersonaID is the persona identifier WhatsApp expects in
	// BotMetadata when no explicit persona was configured for the request.
	defaultBotPersonaID = "867051314767696$760019659443059"
	// botNodeTag is the tag of the child node carrying the encrypted inline
	// bot invocation inside an outgoing message stanza.
	botNodeTag = "bot"
	// metaNodeTag is the tag of the child node carrying per-request metadata.
	metaNodeTag = "meta"
)

// Attribute names of the <meta> node built from SendRequestExtra.Meta.
const (
	metaAttrDeprecatedLIDSession = "deprecated_lid_session"
	metaAttrThreadMsgID          = "thread_msg_id"
	metaAttrThreadMsgSenderJID   = "thread_msg_sender_jid"
)

// prepareBotMessage fills in the bot-related parts of an outgoing message:
// the message secret, the bot metadata, and — for inline bot mode — the
// rewritten BotInvokeMessage envelope plus the encrypted <bot> node stored in
// extraParams. It returns the message that should actually be sent, which is a
// new envelope in inline bot mode and the original message otherwise.
func (cli *Client) prepareBotMessage(
	ctx context.Context,
	req *SendRequestExtra,
	to types.JID,
	message *waE2E.Message,
	msgID types.MessageID,
	extraParams *nodeExtraParams,
) (*waE2E.Message, error) {
	isInlineBotMode := false
	if !req.InlineBotJID.IsEmpty() {
		if !req.InlineBotJID.IsBot() {
			return nil, ErrInvalidInlineBotID
		}
		isInlineBotMode = true
	}

	isBotMode := isInlineBotMode || to.IsBot()
	needsMessageSecret := isBotMode || cli.shouldIncludeReportingToken(message)

	if needsMessageSecret {
		if message.MessageContextInfo == nil {
			message.MessageContextInfo = &waE2E.MessageContextInfo{}
		}
		if message.MessageContextInfo.MessageSecret == nil {
			message.MessageContextInfo.MessageSecret = random.Bytes(messageSecretSize)
		}
	}

	if !isBotMode {
		return message, nil
	}

	if message.MessageContextInfo.BotMetadata == nil {
		message.MessageContextInfo.BotMetadata = &waAICommon.BotMetadata{
			PersonaID: proto.String(defaultBotPersonaID),
		}
	}
	if !isInlineBotMode {
		return message, nil
	}

	// inline mode specific code
	messageSecret := message.GetMessageContextInfo().GetMessageSecret()
	message = &waE2E.Message{
		BotInvokeMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ExtendedTextMessage: message.ExtendedTextMessage,
				MessageContextInfo: &waE2E.MessageContextInfo{
					BotMetadata: message.MessageContextInfo.BotMetadata,
				},
			},
		},
		MessageContextInfo: message.MessageContextInfo,
	}

	botMessage := &waE2E.Message{
		BotInvokeMessage: message.BotInvokeMessage,
		MessageContextInfo: &waE2E.MessageContextInfo{
			BotMetadata:      message.MessageContextInfo.BotMetadata,
			BotMessageSecret: applyBotMessageHKDF(messageSecret),
		},
	}

	messagePlaintext, _, marshalErr := marshalMessage(req.InlineBotJID, botMessage)
	if marshalErr != nil {
		return nil, marshalErr
	}

	participantNodes, _, err := cli.encryptMessageForDevices(ctx, []types.JID{req.InlineBotJID}, msgID, messagePlaintext, nil, waBinary.Attrs{})
	if err != nil {
		return nil, err
	}
	extraParams.botNode = &waBinary.Node{
		Tag:     botNodeTag,
		Attrs:   nil,
		Content: participantNodes,
	}
	return message, nil
}

// resolveSendTarget resolves the effective destination and own identity for an
// outgoing message. For groups and broadcast lists it returns the participant
// list; for LID-migrated direct chats it rewrites `to` to the LID address. It
// mutates `to`, `ownID`, resp.DebugTimings and extraParams in place.
func (cli *Client) resolveSendTarget(
	ctx context.Context,
	to *types.JID,
	ownID *types.JID,
	req *SendRequestExtra,
	resp *SendResponse,
	extraParams *nodeExtraParams,
) ([]types.JID, error) {
	switch {
	case to.Server == types.GroupServer || to.Server == types.BroadcastServer:
		return cli.resolveGroupSendTarget(ctx, *to, ownID, req, resp, extraParams)
	case to.Server == types.HiddenUserServer:
		*ownID = cli.getOwnLID()
	case to.Server == types.DefaultUserServer && cli.Store.LIDMigrationTimestamp > 0 && !req.Peer:
		if err := cli.migrateSendTargetToLID(ctx, to, ownID, resp); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// resolveGroupSendTarget resolves the participant list of a group or broadcast
// list destination, picking the addressing mode (PN/LID) the group expects.
func (cli *Client) resolveGroupSendTarget(
	ctx context.Context,
	to types.JID,
	ownID *types.JID,
	req *SendRequestExtra,
	resp *SendResponse,
	extraParams *nodeExtraParams,
) ([]types.JID, error) {
	start := time.Now()

	if to.Server != types.GroupServer {
		groupParticipants, err := cli.getBroadcastListParticipants(ctx, to)
		if err != nil {
			return nil, fmt.Errorf("failed to get broadcast list members: %w", err)
		}
		resp.DebugTimings.GetParticipants = time.Since(start)
		return groupParticipants, nil
	}

	cachedData, err := cli.getCachedGroupData(ctx, to)
	if err != nil {
		return nil, fmt.Errorf("failed to get group members: %w", err)
	}
	// TODO this is fairly hacky, is there a proper way to determine which identity the message is sent with?
	if cachedData.AddressingMode == types.AddressingModeLID {
		*ownID = cli.getOwnLID()
		extraParams.addressingMode = types.AddressingModeLID
	} else if cachedData.CommunityAnnouncementGroup && req.Meta != nil {
		*ownID = cli.getOwnLID()
		// Why is this set to PN?
		extraParams.addressingMode = types.AddressingModePN
	}
	resp.DebugTimings.GetParticipants = time.Since(start)
	return cachedData.Members, nil
}

// migrateSendTargetToLID replaces a phone-number destination with its LID
// address when the account has already migrated to LID addressing.
func (cli *Client) migrateSendTargetToLID(ctx context.Context, to *types.JID, ownID *types.JID, resp *SendResponse) error {
	start := time.Now()
	toLID, err := cli.Store.LIDs.GetLIDForPN(ctx, *to)
	if err != nil {
		return fmt.Errorf("failed to get LID for PN %s: %w", *to, err)
	} else if toLID.IsEmpty() {
		info, err := cli.GetUserInfo(ctx, []types.JID{*to})
		if err != nil {
			return fmt.Errorf("failed to get user info for %s to fill LID cache: %w", *to, err)
		} else if toLID = info[*to].LID; toLID.IsEmpty() {
			return fmt.Errorf("no LID found for %s from server", *to)
		}
	}
	resp.DebugTimings.LIDFetch = time.Since(start)
	cli.Log.Debugf("Replacing SendMessage destination with LID as migration timestamp is set %s -> %s", *to, toLID)
	*to = toLID
	*ownID = cli.getOwnLID()
	return nil
}

// applyRequestExtraNodes translates the optional Meta and AdditionalNodes of a
// send request into the extra child nodes of the outgoing message stanza.
func applyRequestExtraNodes(req *SendRequestExtra, extraParams *nodeExtraParams) {
	if req.Meta != nil {
		extraParams.metaNode = &waBinary.Node{
			Tag:   metaNodeTag,
			Attrs: waBinary.Attrs{},
		}
		if req.Meta.DeprecatedLIDSession != nil {
			extraParams.metaNode.Attrs[metaAttrDeprecatedLIDSession] = *req.Meta.DeprecatedLIDSession
		}
		if req.Meta.ThreadMessageID != "" {
			extraParams.metaNode.Attrs[metaAttrThreadMsgID] = req.Meta.ThreadMessageID
			extraParams.metaNode.Attrs[metaAttrThreadMsgSenderJID] = req.Meta.ThreadMessageSenderJID
		}
	}

	if req.AdditionalNodes != nil {
		extraParams.additionalNodes = req.AdditionalNodes
	}
}
