package send

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/util/random"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/capabilities/group"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waAICommon"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
)

// prepareBotMessage fills in the bot-related parts of an outgoing message:
// the message secret, the bot metadata, and — for inline bot mode — the
// rewritten BotInvokeMessage envelope plus the encrypted <bot> node stored in
// extraParams. It returns the message that should actually be sent, which is a
// new envelope in inline bot mode and the original message otherwise.
func prepareBotMessage(
	ctx context.Context,
	t Transport,
	req *RequestExtra,
	to types.JID,
	message *waE2E.Message,
	msgID types.MessageID,
	extraParams *NodeExtraParams,
) (*waE2E.Message, error) {
	isInlineBotMode := false
	if !req.InlineBotJID.IsEmpty() {
		if !req.InlineBotJID.IsBot() {
			return nil, ErrInvalidInlineBotID
		}
		isInlineBotMode = true
	}

	isBotMode := isInlineBotMode || to.IsBot()
	needsMessageSecret := isBotMode || t.ShouldIncludeReportingToken(message)

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
			BotMessageSecret: t.ApplyBotMessageHKDF(messageSecret),
		},
	}

	messagePlaintext, _, marshalErr := MarshalMessage(req.InlineBotJID, botMessage)
	if marshalErr != nil {
		return nil, marshalErr
	}

	participantNodes, _, err := EncryptForDevices(ctx, t, []types.JID{req.InlineBotJID}, msgID, messagePlaintext, nil, waBinary.Attrs{})
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
func resolveSendTarget(
	ctx context.Context,
	t Transport,
	to *types.JID,
	ownID *types.JID,
	req *RequestExtra,
	resp *Response,
	extraParams *NodeExtraParams,
) ([]types.JID, error) {
	switch {
	case to.Server == types.GroupServer || to.Server == types.BroadcastServer:
		return resolveGroupSendTarget(ctx, t, *to, ownID, req, resp, extraParams)
	case to.Server == types.HiddenUserServer:
		*ownID = t.OwnLID()
	case to.Server == types.DefaultUserServer && t.Store().LIDMigrationTimestamp > 0 && !req.Peer:
		if err := migrateSendTargetToLID(ctx, t, to, ownID, resp); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// resolveGroupSendTarget resolves the participant list of a group or broadcast
// list destination, picking the addressing mode (PN/LID) the group expects.
func resolveGroupSendTarget(
	ctx context.Context,
	t Transport,
	to types.JID,
	ownID *types.JID,
	req *RequestExtra,
	resp *Response,
	extraParams *NodeExtraParams,
) ([]types.JID, error) {
	start := time.Now()

	if to.Server != types.GroupServer {
		groupParticipants, err := t.BroadcastListParticipants(ctx, to)
		if err != nil {
			return nil, fmt.Errorf("failed to get broadcast list members: %w", err)
		}
		resp.DebugTimings.GetParticipants = time.Since(start)
		return groupParticipants, nil
	}

	cachedData, err := t.CachedGroupData(ctx, to)
	if err != nil {
		return nil, fmt.Errorf("failed to get group members: %w", err)
	} else if cachedData == nil {
		// CachedGroupData returns (nil, nil) when the group info query
		// succeeded but the response was cached under a different JID than the
		// one requested (the cache is keyed by the `id` the *server* echoes
		// back). Dereferencing it here would be a server-triggerable panic.
		return nil, fmt.Errorf("failed to get group members: %w", group.ErrNotFound)
	}
	// TODO this is fairly hacky, is there a proper way to determine which identity the message is sent with?
	if cachedData.AddressingMode == types.AddressingModeLID {
		*ownID = t.OwnLID()
		extraParams.addressingMode = types.AddressingModeLID
	} else if cachedData.CommunityAnnouncementGroup && req.Meta != nil {
		*ownID = t.OwnLID()
		// Why is this set to PN?
		extraParams.addressingMode = types.AddressingModePN
	}
	resp.DebugTimings.GetParticipants = time.Since(start)
	return cachedData.Members, nil
}

// migrateSendTargetToLID replaces a phone-number destination with its LID
// address when the account has already migrated to LID addressing.
func migrateSendTargetToLID(ctx context.Context, t Transport, to *types.JID, ownID *types.JID, resp *Response) error {
	start := time.Now()
	toLID, err := t.Store().LIDs.GetLIDForPN(ctx, *to)
	if err != nil {
		return fmt.Errorf("failed to get LID for PN %s: %w", *to, err)
	} else if toLID.IsEmpty() {
		info, err := t.UserInfo(ctx, []types.JID{*to})
		if err != nil {
			return fmt.Errorf("failed to get user info for %s to fill LID cache: %w", *to, err)
		} else if toLID = info[*to].LID; toLID.IsEmpty() {
			return fmt.Errorf("no LID found for %s from server", *to)
		}
	}
	resp.DebugTimings.LIDFetch = time.Since(start)
	t.Log().Debugf("Replacing SendMessage destination with LID as migration timestamp is set %s -> %s", *to, toLID)
	*to = toLID
	*ownID = t.OwnLID()
	return nil
}

// applyRequestExtraNodes translates the optional Meta and AdditionalNodes of a
// send request into the extra child nodes of the outgoing message stanza.
func applyRequestExtraNodes(req *RequestExtra, extraParams *NodeExtraParams) {
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
