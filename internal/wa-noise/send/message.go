// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"context"
	"fmt"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
)

// Message e' o corpo de Client.SendMessage a partir do ponto em que o
// parametro variadico ja' foi resolvido.
//
// As DUAS primeiras guardas de SendMessage (receptor nil e "so' um extra") nao
// vieram junto e continuam na raiz: a checagem de nil so' pode existir la'
// (um *Client nil nao produz Transport) e a de aridade e' sobre o variadico,
// que a fachada consome. A ORDEM das guardas seguintes foi preservada
// exatamente — `to.Device` antes de `ownID.IsEmpty()` —, porque trocar as duas
// mudaria qual erro um chamador recebe quando as duas condicoes valem.
func Message(
	ctx context.Context,
	t Transport,
	to types.JID,
	message *waE2E.Message,
	req RequestExtra,
) (resp Response, err error) {
	if to.Device > 0 && !req.Peer {
		err = ErrRecipientADJID
		return
	}
	ownID := t.OwnID()
	if ownID.IsEmpty() {
		err = t.Errors().NotLoggedIn
		return
	}

	if req.Timeout == 0 {
		req.Timeout = t.DefaultRequestTimeout()
	}
	if len(req.ID) == 0 {
		req.ID = t.GenerateMessageID()
	}
	if to.Server == types.NewsletterServer {
		// TODO somehow deduplicate this with the code in sendNewsletter?
		if message.EditedMessage != nil {
			req.ID = types.MessageID(message.GetEditedMessage().GetMessage().GetProtocolMessage().GetKey().GetID())
		} else if message.ProtocolMessage != nil && message.ProtocolMessage.GetType() == waE2E.ProtocolMessage_REVOKE {
			req.ID = types.MessageID(message.GetProtocolMessage().GetKey().GetID())
		}
	}
	resp.ID = req.ID

	var extraParams NodeExtraParams
	message, err = prepareBotMessage(ctx, t, &req, to, message, resp.ID, &extraParams)
	if err != nil {
		return
	}

	var groupParticipants []types.JID
	groupParticipants, err = resolveSendTarget(ctx, t, &to, &ownID, &req, &resp, &extraParams)
	if err != nil {
		return
	}
	applyRequestExtraNodes(&req, &extraParams)

	resp.Sender = ownID

	start := time.Now()
	// Sending multiple messages at a time can cause weird issues and makes it harder to retry safely
	// This is also required for the session prefetching that makes group sends faster
	// (everything will explode if you send a message to the same user twice in parallel)
	lock := t.SendLock()
	lock.Lock()
	resp.DebugTimings.Queue = time.Since(start)
	defer lock.Unlock()

	// Peer message retries aren't implemented yet
	if !req.Peer {
		err = t.AddRecentMessage(ctx, to, req.ID, message, nil)
		if err != nil {
			return
		}
	}

	if message.GetMessageContextInfo().GetMessageSecret() != nil {
		err = t.Store().MsgSecrets.PutMessageSecret(ctx, to, ownID, req.ID, message.GetMessageContextInfo().GetMessageSecret())
		if err != nil {
			t.Log().Warnf("Failed to store message secret key for outgoing message %s: %v", req.ID, err)
		} else {
			t.Log().Debugf("Stored message secret key for outgoing message %s", req.ID)
		}
	}

	respChan := t.WaitResponse(req.ID)
	var phash string
	var data []byte
	switch to.Server {
	case types.GroupServer, types.BroadcastServer:
		phash, data, err = Group(ctx, t, ownID, to, groupParticipants, req.ID, message, &resp.DebugTimings, extraParams)
	case types.DefaultUserServer, types.BotServer, types.HiddenUserServer:
		if req.Peer {
			data, err = PeerMessage(ctx, t, to, req.ID, message, &resp.DebugTimings)
		} else {
			phash, data, err = DM(ctx, t, ownID, to, req.ID, message, &resp.DebugTimings, extraParams)
		}
	case types.NewsletterServer:
		data, err = Newsletter(ctx, t, to, req.ID, message, req.MediaHandle, &resp.DebugTimings)
	default:
		err = fmt.Errorf("%w %s", ErrUnknownServer, to.Server)
	}
	start = time.Now()
	if err != nil {
		t.CancelResponse(req.ID, respChan)
		return
	}
	var respNode *waBinary.Node
	respNode, err = AwaitAck(ctx, t, &req, &resp, respChan, data, start)
	if err != nil {
		return
	}
	err = ApplyAck(t, respNode, to, phash, &resp)
	return
}
