// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

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

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
)

func participantListHashV2(participants []types.JID) string {
	participantsStrings := make([]string, len(participants))
	for i, part := range participants {
		participantsStrings[i] = part.ADString()
	}

	sort.Strings(participantsStrings)
	hash := sha256.Sum256([]byte(strings.Join(participantsStrings, "")))
	return fmt.Sprintf("2:%s", base64.RawStdEncoding.EncodeToString(hash[:6]))
}

func (cli *Client) sendNewsletter(
	ctx context.Context,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	mediaID string,
	timings *MessageDebugTimings,
) ([]byte, error) {
	attrs := waBinary.Attrs{
		"to":   to,
		"id":   id,
		"type": getTypeFromMessage(message),
	}
	if mediaID != "" {
		attrs["media_id"] = mediaID
	}
	if message.EditedMessage != nil {
		attrs["edit"] = string(types.EditAttributeAdminEdit)
		message = message.GetEditedMessage().GetMessage().GetProtocolMessage().GetEditedMessage()
	} else if message.ProtocolMessage != nil && message.ProtocolMessage.GetType() == waE2E.ProtocolMessage_REVOKE {
		attrs["edit"] = string(types.EditAttributeAdminRevoke)
		message = nil
	}
	start := time.Now()
	plaintext, _, err := marshalMessage(to, message)
	timings.Marshal = time.Since(start)
	if err != nil {
		return nil, err
	}
	plaintextNode := waBinary.Node{
		Tag:     "plaintext",
		Content: plaintext,
		Attrs:   waBinary.Attrs{},
	}
	if message != nil {
		if mediaType := getMediaTypeFromMessage(message); mediaType != "" {
			plaintextNode.Attrs["mediatype"] = mediaType
		}
	}
	node := waBinary.Node{
		Tag:     "message",
		Attrs:   attrs,
		Content: []waBinary.Node{plaintextNode},
	}
	start = time.Now()
	data, err := cli.sendNodeAndGetData(ctx, node)
	timings.Send = time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("failed to send message node: %w", err)
	}
	return data, nil
}

type nodeExtraParams struct {
	botNode         *waBinary.Node
	metaNode        *waBinary.Node
	additionalNodes *[]waBinary.Node
	addressingMode  types.AddressingMode
}

func (cli *Client) sendGroup(
	ctx context.Context,
	ownID,
	to types.JID,
	participants []types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *MessageDebugTimings,
	extraParams nodeExtraParams,
) (string, []byte, error) {
	start := time.Now()
	plaintext, _, err := marshalMessage(to, message)
	timings.Marshal = time.Since(start)
	if err != nil {
		return "", nil, err
	}

	start = time.Now()
	builder := groups.NewGroupSessionBuilder(cli.Store, pbSerializer)
	senderKeyName := protocol.NewSenderKeyName(to.String(), cli.getOwnLID().SignalAddress())
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

	cipher := groups.NewGroupCipher(builder, senderKeyName, cli.Store)
	encrypted, err := cipher.Encrypt(ctx, padMessage(plaintext))
	if err != nil {
		return "", nil, fmt.Errorf("failed to encrypt group message to send %s to %s: %w", id, to, err)
	}
	ciphertext := encrypted.SignedSerialize()
	timings.GroupEncrypt = time.Since(start)

	node, allDevices, err := cli.prepareMessageNode(
		ctx, to, id, message, participants, skdPlaintext, nil, timings, extraParams,
	)
	if err != nil {
		return "", nil, err
	}

	phash := participantListHashV2(allDevices)
	node.Attrs["phash"] = phash
	skMsg := waBinary.Node{
		Tag:     "enc",
		Content: ciphertext,
		Attrs:   waBinary.Attrs{"v": "2", "type": "skmsg"},
	}
	if mediaType := getMediaTypeFromMessage(message); mediaType != "" {
		skMsg.Attrs["mediatype"] = mediaType
	}
	node.Content = append(node.GetChildren(), skMsg)
	if cli.shouldIncludeReportingToken(message) && message.GetMessageContextInfo().GetMessageSecret() != nil {
		node.Content = append(node.GetChildren(), cli.getMessageReportingToken(plaintext, message, ownID, to, id))
	}

	start = time.Now()
	data, err := cli.sendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return "", nil, fmt.Errorf("failed to send message node: %w", err)
	}
	return phash, data, nil
}

func (cli *Client) sendPeerMessage(
	ctx context.Context,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *MessageDebugTimings,
) ([]byte, error) {
	node, err := cli.preparePeerMessageNode(ctx, to, id, message, timings)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	data, err := cli.sendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("failed to send message node: %w", err)
	}
	return data, nil
}

func (cli *Client) sendDM(
	ctx context.Context,
	ownID,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *MessageDebugTimings,
	extraParams nodeExtraParams,
) (string, []byte, error) {
	start := time.Now()
	messagePlaintext, deviceSentMessagePlaintext, err := marshalMessage(to, message)
	timings.Marshal = time.Since(start)
	if err != nil {
		return "", nil, err
	}

	node, allDevices, err := cli.prepareMessageNode(
		ctx, to, id, message, []types.JID{to, ownID.ToNonAD()},
		messagePlaintext, deviceSentMessagePlaintext, timings, extraParams,
	)
	if err != nil {
		return "", nil, err
	}
	phash := participantListHashV2(allDevices)

	if cli.shouldIncludeReportingToken(message) && message.GetMessageContextInfo().GetMessageSecret() != nil {
		node.Content = append(node.GetChildren(), cli.getMessageReportingToken(messagePlaintext, message, ownID, to, id))
	}

	tcTokenBytes, tcErr := cli.ensureTCToken(ctx, to)
	if tcErr != nil {
		cli.Log.Warnf("Failed to get privacy token for %s: %v", to, tcErr)
	}
	if len(tcTokenBytes) > 0 {
		node.Content = append(node.GetChildren(), waBinary.Node{
			Tag:     "tctoken",
			Content: tcTokenBytes,
		})
	} else if csToken := cli.generateCsToken(ctx, to); len(csToken) > 0 {
		node.Content = append(node.GetChildren(), waBinary.Node{
			Tag:     "cstoken",
			Content: csToken,
		})
	}

	start = time.Now()
	data, err := cli.sendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return "", nil, fmt.Errorf("failed to send message node: %w", err)
	}

	storageJID := cli.resolveTCTokenStorageLID(ctx, to)
	if shouldSendTCTokenInChatAction(to) && shouldSendNewTCToken(cli.getTCTokenSenderTS(storageJID)) {
		go cli.issuePrivacyTokenAndSave(storageJID, time.Now())
	}

	return phash, data, nil
}
