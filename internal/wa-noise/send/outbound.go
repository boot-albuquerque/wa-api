// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mau.fi/libsignal/groups"
	"go.mau.fi/libsignal/protocol"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/msgattrs"
	"wa-api/internal/wa-noise/protocol/msgpad"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/capabilities/tctoken"
	"wa-api/internal/wa-noise/protocol/types"
)

// ParticipantListHashV2 calcula o `phash` de uma lista de dispositivos.
//
// Continua morando no dominio de ENVIO — e nao em user/ — porque hasheia
// listas de participantes de grupo tanto quanto listas de dispositivo. O lote 7
// ja' registrava essa pendencia em user.Transport.ParticipantListHash, que
// agora atravessa a fachada da raiz ate' aqui.
func ParticipantListHashV2(participants []types.JID) string {
	participantsStrings := make([]string, len(participants))
	for i, part := range participants {
		participantsStrings[i] = part.ADString()
	}

	sort.Strings(participantsStrings)
	hash := sha256.Sum256([]byte(strings.Join(participantsStrings, "")))
	return fmt.Sprintf(
		"%s:%s",
		participantListHashPrefix,
		base64.RawStdEncoding.EncodeToString(hash[:participantListHashLength]),
	)
}

// Newsletter envia uma mensagem de canal (texto plano, sem Signal).
// Era Client.sendNewsletter.
func Newsletter(
	ctx context.Context,
	t Transport,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	mediaID string,
	timings *DebugTimings,
) ([]byte, error) {
	attrs := waBinary.Attrs{
		msgAttrTo:   to,
		msgAttrID:   id,
		msgAttrType: msgattrs.GetTypeFromMessage(message),
	}
	if mediaID != "" {
		attrs[msgAttrMediaID] = mediaID
	}
	if message.EditedMessage != nil {
		attrs[msgAttrEdit] = string(types.EditAttributeAdminEdit)
		message = message.GetEditedMessage().GetMessage().GetProtocolMessage().GetEditedMessage()
	} else if message.ProtocolMessage != nil && message.ProtocolMessage.GetType() == waE2E.ProtocolMessage_REVOKE {
		attrs[msgAttrEdit] = string(types.EditAttributeAdminRevoke)
		message = nil
	}
	start := time.Now()
	plaintext, _, err := MarshalMessage(to, message)
	timings.Marshal = time.Since(start)
	if err != nil {
		return nil, err
	}
	plaintextNode := waBinary.Node{
		Tag:     plaintextNodeTag,
		Content: plaintext,
		Attrs:   waBinary.Attrs{},
	}
	if message != nil {
		if mediaType := msgattrs.GetMediaTypeFromMessage(message); mediaType != "" {
			plaintextNode.Attrs[encAttrMediaType] = mediaType
		}
	}
	node := waBinary.Node{
		Tag:     messageNodeTag,
		Attrs:   attrs,
		Content: []waBinary.Node{plaintextNode},
	}
	start = time.Now()
	data, err := t.SendNodeAndGetData(ctx, node)
	timings.Send = time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("failed to send message node: %w", err)
	}
	return data, nil
}

// Group envia uma mensagem waE2E a um grupo ou lista de transmissao.
// Era Client.sendGroup.
func Group(
	ctx context.Context,
	t Transport,
	ownID,
	to types.JID,
	participants []types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *DebugTimings,
	extraParams NodeExtraParams,
) (string, []byte, error) {
	start := time.Now()
	plaintext, _, err := MarshalMessage(to, message)
	timings.Marshal = time.Since(start)
	if err != nil {
		return "", nil, err
	}

	start = time.Now()
	builder := groups.NewGroupSessionBuilder(t.Store(), store.SignalProtobufSerializer)
	senderKeyName := protocol.NewSenderKeyName(to.String(), t.OwnLID().SignalAddress())
	signalSKDMessage, err := builder.Create(ctx, senderKeyName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create sender key distribution message to send %s to %s: %w", id, to, err)
	}
	skdMessage := &waE2E.Message{
		SenderKeyDistributionMessage: &waE2E.SenderKeyDistributionMessage{
			GroupID:                             proto.String(to.String()),
			AxolotlSenderKeyDistributionMessage: signalSKDMessage.Serialize(),
		},
	}
	skdPlaintext, err := proto.Marshal(skdMessage)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal sender key distribution message to send %s to %s: %w", id, to, err)
	}

	cipher := groups.NewGroupCipher(builder, senderKeyName, t.Store())
	encrypted, err := cipher.Encrypt(ctx, msgpad.Pad(plaintext))
	if err != nil {
		return "", nil, fmt.Errorf("failed to encrypt group message to send %s to %s: %w", id, to, err)
	}
	ciphertext := encrypted.SignedSerialize()
	timings.GroupEncrypt = time.Since(start)

	node, allDevices, err := PrepareMessageNode(
		ctx, t, to, id, message, participants, skdPlaintext, nil, timings, extraParams,
	)
	if err != nil {
		return "", nil, err
	}

	phash := ParticipantListHashV2(allDevices)
	node.Attrs[msgAttrPHash] = phash
	skMsg := waBinary.Node{
		Tag:     encNodeTag,
		Content: ciphertext,
		Attrs: waBinary.Attrs{
			encAttrVersion: encVersionSignal,
			encAttrType:    encTypeSenderKey,
		},
	}
	if mediaType := msgattrs.GetMediaTypeFromMessage(message); mediaType != "" {
		skMsg.Attrs[encAttrMediaType] = mediaType
	}
	node.Content = append(node.GetChildren(), skMsg)
	if t.ShouldIncludeReportingToken(message) && message.GetMessageContextInfo().GetMessageSecret() != nil {
		node.Content = append(node.GetChildren(), t.MessageReportingToken(plaintext, message, ownID, to, id))
	}

	start = time.Now()
	data, err := t.SendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return "", nil, fmt.Errorf("failed to send message node: %w", err)
	}
	return phash, data, nil
}

// PeerMessage envia uma mensagem de protocolo aos proprios dispositivos.
// Era Client.sendPeerMessage.
func PeerMessage(
	ctx context.Context,
	t Transport,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *DebugTimings,
) ([]byte, error) {
	node, err := PreparePeerMessageNode(ctx, t, to, id, message, timings)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	data, err := t.SendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("failed to send message node: %w", err)
	}
	return data, nil
}

// DM envia uma mensagem waE2E a um usuario. Era Client.sendDM.
func DM(
	ctx context.Context,
	t Transport,
	ownID,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *DebugTimings,
	extraParams NodeExtraParams,
) (string, []byte, error) {
	start := time.Now()
	messagePlaintext, deviceSentMessagePlaintext, err := MarshalMessage(to, message)
	timings.Marshal = time.Since(start)
	if err != nil {
		return "", nil, err
	}

	node, allDevices, err := PrepareMessageNode(
		ctx, t, to, id, message, []types.JID{to, ownID.ToNonAD()},
		messagePlaintext, deviceSentMessagePlaintext, timings, extraParams,
	)
	if err != nil {
		return "", nil, err
	}
	phash := ParticipantListHashV2(allDevices)

	if t.ShouldIncludeReportingToken(message) && message.GetMessageContextInfo().GetMessageSecret() != nil {
		node.Content = append(node.GetChildren(), t.MessageReportingToken(messagePlaintext, message, ownID, to, id))
	}

	tcTokenBytes, tcErr := t.EnsureTCToken(ctx, to)
	if tcErr != nil {
		t.Log().Warnf("Failed to get privacy token for %s: %v", to, tcErr)
	}
	if len(tcTokenBytes) > 0 {
		node.Content = append(node.GetChildren(), waBinary.Node{
			Tag:     tcTokenNodeTag,
			Content: tcTokenBytes,
		})
	} else if csToken := t.GenerateCsToken(ctx, to); len(csToken) > 0 {
		node.Content = append(node.GetChildren(), waBinary.Node{
			Tag:     csTokenNodeTag,
			Content: csToken,
		})
	}

	start = time.Now()
	data, err := t.SendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return "", nil, fmt.Errorf("failed to send message node: %w", err)
	}

	storageJID := t.ResolveTCTokenStorageLID(ctx, to)
	if tctoken.ShouldSendInChatAction(to) && tctoken.ShouldSendNew(t.TCTokenSenderTS(storageJID)) {
		go t.IssuePrivacyTokenAndSave(storageJID, time.Now())
	}

	return phash, data, nil
}
