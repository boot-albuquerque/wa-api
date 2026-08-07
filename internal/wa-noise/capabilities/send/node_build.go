package send

import (
	"context"
	"fmt"
	"slices"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/msgattrs"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// PreparePeerMessageNode monta o <message> de uma mensagem de protocolo para os
// proprios dispositivos. Era Client.preparePeerMessageNode.
func PreparePeerMessageNode(
	ctx context.Context,
	t Transport,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *DebugTimings,
) (*waBinary.Node, error) {
	attrs := waBinary.Attrs{
		msgAttrID:       id,
		msgAttrType:     msgTypeText,
		msgAttrCategory: msgCategoryPeer,
		msgAttrTo:       to,
	}
	if message.GetProtocolMessage().GetType() == waE2E.ProtocolMessage_APP_STATE_SYNC_KEY_REQUEST {
		attrs[msgAttrPushPriority] = pushPriorityHigh
	} else if message.GetProtocolMessage().GetPeerDataOperationRequestMessage().GetPeerDataOperationRequestType() == waE2E.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND {
		attrs[msgAttrPushPriority] = pushPriorityHighForce
		attrs[msgAttrPrivacySensitive] = privacySensitiveOn
	}
	start := time.Now()
	plaintext, err := proto.Marshal(message)
	timings.Marshal = time.Since(start)
	if err != nil {
		err = fmt.Errorf("failed to marshal message: %w", err)
		return nil, err
	}
	encryptionIdentity := to
	if to.Server == types.DefaultUserServer {
		encryptionIdentity, err = t.Store().LIDs.GetLIDForPN(ctx, to)
		if err != nil {
			return nil, fmt.Errorf("failed to get LID for PN %s: %w", to, err)
		} else if encryptionIdentity.IsEmpty() {
			// GetLIDForPN returns a zero JID (no error) when the mapping is
			// unknown. Encrypting for the zero JID would build a Signal
			// address with an empty user, which can never match a stored
			// session — fail loudly instead of silently addressing nothing.
			return nil, fmt.Errorf("failed to get LID for PN %s: %w", to, ErrNoSession)
		}
	}
	start = time.Now()
	encrypted, isPreKey, err := EncryptForDevice(ctx, t, plaintext, encryptionIdentity, nil, nil, nil)
	timings.PeerEncrypt = time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt peer message for %s: %v", to, err)
	}
	content := []waBinary.Node{{
		Tag: metaNodeTag,
		Attrs: waBinary.Attrs{
			metaAttrAppData: metaAppDataDefault,
		},
	}, *encrypted}
	if isPreKey && !t.IsMessenger() {
		content = append(content, MakeDeviceIdentityNode(t))
	}
	return &waBinary.Node{
		Tag:     messageNodeTag,
		Attrs:   attrs,
		Content: content,
	}, nil
}

// MessageContent monta a lista de filhos do <message>. Era
// Client.getMessageContent.
func MessageContent(
	t Transport,
	baseNode waBinary.Node,
	message *waE2E.Message,
	msgAttrs waBinary.Attrs,
	includeIdentity bool,
	extraParams NodeExtraParams,
) []waBinary.Node {
	content := []waBinary.Node{baseNode}
	if includeIdentity {
		content = append(content, MakeDeviceIdentityNode(t))
	}
	if msgAttrs[msgAttrType] == msgTypePoll {
		pollType := pollTypeCreation
		if message.PollUpdateMessage != nil {
			pollType = pollTypeVote
		}
		content = append(content, waBinary.Node{
			Tag: metaNodeTag,
			Attrs: waBinary.Attrs{
				metaAttrPollType: pollType,
			},
		})
	}

	if extraParams.botNode != nil {
		content = append(content, *extraParams.botNode)
	}
	if extraParams.metaNode != nil {
		content = append(content, *extraParams.metaNode)
	}
	if extraParams.additionalNodes != nil {
		content = append(content, *extraParams.additionalNodes...)
	}

	if buttonType := msgattrs.GetButtonTypeFromMessage(message); buttonType != "" {
		content = append(content, waBinary.Node{
			Tag: bizNodeTag,
			Content: []waBinary.Node{{
				Tag:   buttonType,
				Attrs: msgattrs.GetButtonAttributes(message),
			}},
		})
	}
	return content
}

// PrepareMessageNode monta o <message> waE2E ja' cifrado por dispositivo,
// devolvendo tambem a lista de dispositivos usada (base do phash). Era
// Client.prepareMessageNode.
func PrepareMessageNode(
	ctx context.Context,
	t Transport,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	participants []types.JID,
	plaintext, dsmPlaintext []byte,
	timings *DebugTimings,
	extraParams NodeExtraParams,
) (*waBinary.Node, []types.JID, error) {
	start := time.Now()
	allDevices, err := t.UserDevices(ctx, participants)
	timings.GetDevices = time.Since(start)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get device list: %w", err)
	}

	if to.Server == types.GroupServer {
		allDevices = slices.DeleteFunc(allDevices, func(jid types.JID) bool {
			return jid.Server == types.HostedServer || jid.Server == types.HostedLIDServer
		})
	}

	msgType := msgattrs.GetTypeFromMessage(message)
	encAttrs := waBinary.Attrs{}
	// Only include encMediaType for 1:1 messages (groups don't have a device-sent message plaintext)
	if encMediaType := msgattrs.GetMediaTypeFromMessage(message); dsmPlaintext != nil && encMediaType != "" {
		encAttrs[encAttrMediaType] = encMediaType
	}
	attrs := waBinary.Attrs{
		msgAttrID:   id,
		msgAttrType: msgType,
		msgAttrTo:   to,
	}
	// TODO this is a very hacky hack for announcement group messages, why is it pn anyway?
	if extraParams.addressingMode != "" {
		attrs[msgAttrAddressingMode] = string(extraParams.addressingMode)
	}
	if editAttr := msgattrs.GetEditAttribute(message); editAttr != "" {
		attrs[msgAttrEdit] = string(editAttr)
		encAttrs[encAttrDecryptFail] = string(events.DecryptFailHide)
	}
	if msgType == msgTypeReaction || message.GetPollUpdateMessage() != nil {
		encAttrs[encAttrDecryptFail] = string(events.DecryptFailHide)
	}

	start = time.Now()
	participantNodes, includeIdentity, err := EncryptForDevices(
		ctx, t, allDevices, id, plaintext, dsmPlaintext, encAttrs,
	)
	timings.PeerEncrypt = time.Since(start)
	if err != nil {
		return nil, nil, err
	}
	participantNode := waBinary.Node{
		Tag:     participantsNodeTag,
		Content: participantNodes,
	}
	return &waBinary.Node{
		Tag:   messageNodeTag,
		Attrs: attrs,
		Content: MessageContent(
			t, participantNode, message, attrs, includeIdentity, extraParams,
		),
	}, allDevices, nil
}

// MarshalMessage serializa a mensagem e, quando cabe, a copia device-sent para
// os proprios dispositivos. Era marshalMessage (funcao livre ja' na raiz).
func MarshalMessage(to types.JID, message *waE2E.Message) (plaintext, dsmPlaintext []byte, err error) {
	if message == nil && to.Server == types.NewsletterServer {
		return
	}
	plaintext, err = proto.Marshal(message)
	if err != nil {
		err = fmt.Errorf("failed to marshal message: %w", err)
		return
	}

	if to.Server != types.GroupServer && to.Server != types.NewsletterServer {
		dsmPlaintext, err = proto.Marshal(&waE2E.Message{
			DeviceSentMessage: &waE2E.DeviceSentMessage{
				DestinationJID: proto.String(to.String()),
				Message:        message,
			},
			MessageContextInfo: message.MessageContextInfo,
		})
		if err != nil {
			err = fmt.Errorf("failed to marshal message (for own devices): %w", err)
			return
		}
	}

	return
}

// MakeDeviceIdentityNode monta o <device-identity>. Era
// Client.makeDeviceIdentityNode — incluindo o panic, preservado literalmente:
// falhar ao serializar a propria conta e' corrupcao de estado local, nao erro
// de protocolo, e o comportamento historico e' visivel via
// DangerousInternalClient.MakeDeviceIdentityNode.
func MakeDeviceIdentityNode(t Transport) waBinary.Node {
	deviceIdentity, err := proto.Marshal(t.Store().Account)
	if err != nil {
		panic(fmt.Errorf("failed to marshal device identity: %w", err))
	}
	return waBinary.Node{
		Tag:     deviceIdentityNodeTag,
		Content: deviceIdentity,
	}
}
