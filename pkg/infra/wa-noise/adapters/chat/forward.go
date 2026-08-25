package chat

import (
	"context"
	"encoding/json"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	waE2E "wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"

	waclient "wa-api/pkg/infra/wa-noise/client"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"
	wasession "wa-api/pkg/infra/wa-noise/runtime/session"

	"google.golang.org/protobuf/proto"
)

// ForwardedMessageAdapter implements appport.ForwardedMessageSender.
type ForwardedMessageAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewForwardedMessageAdapter creates the adapter.
func NewForwardedMessageAdapter(getClient waclient.Getter) *ForwardedMessageAdapter {
	return &ForwardedMessageAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// SendForwardedMessage implements appport.ForwardedMessageSender.
//
// It deserializes dataJSON (written by eventhandler_message.go:322 as
// json.Marshal(events.Message)) into the wire proto, reads the existing
// forwarding score, increments by 1, sets IsForwarded=true on the appropriate
// ContextInfo, and sends. Media fields (URL, directPath, mediaKey,
// fileEncSHA256, fileSHA256) survive the JSON round-trip — measured against
// live data (CAP-55 round-trip measurement).
//
// The forwarding score logic matches Baileys generateForwardMessageContent:
// read existing score, increment by 1. A message that was never forwarded
// has score 0, so the result is 1 — first forward.
func (a *ForwardedMessageAdapter) SendForwardedMessage(ctx context.Context, txtID string, target domain.JID, dataJSON string, id string) (domain.MessageSendResult, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	recipient, err := wajid.ToJID(target)
	if err != nil {
		return domain.MessageSendResult{}, err
	}

	var evt events.Message
	if err := json.Unmarshal([]byte(dataJSON), &evt); err != nil {
		return domain.MessageSendResult{}, fmt.Errorf("failed to deserialize stored message: %w", err)
	}
	if evt.Message == nil {
		return domain.MessageSendResult{}, fmt.Errorf("stored message has no proto content")
	}

	msg := evt.Message
	applyForwardContext(msg)

	var extra []wanoise.SendRequestExtra
	if id != "" {
		extra = append(extra, wanoise.SendRequestExtra{ID: types.MessageID(id)})
	}

	resp, err := client.SendMessage(ctx, recipient, msg, extra...)
	if err != nil {
		return domain.MessageSendResult{}, err
	}
	return domain.MessageSendResult{Timestamp: resp.Timestamp, ID: string(resp.ID)}, nil
}

// applyForwardContext sets IsForwarded=true and increments ForwardingScore
// by 1 on the message's ContextInfo. If the message type doesn't have a
// ContextInfo yet, one is created.
//
// Baileys logic (generateForwardMessageContent): read forwardingScore from
// contextInfo, increment by 1, set isForwarded=true. Score 0 → 1 (first
// forward).
//
// For Conversation messages (plain text without ContextInfo), the message is
// converted to ExtendedTextMessage — Conversation has no ContextInfo field.
func applyForwardContext(m *waE2E.Message) {
	if m.GetConversation() != "" && m.GetExtendedTextMessage() == nil {
		text := m.GetConversation()
		m.Conversation = nil
		m.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
		}
	}

	ci := getOrCreateContextInfo(m)
	if ci == nil {
		return
	}

	existingScore := ci.GetForwardingScore()
	ci.IsForwarded = proto.Bool(true)
	ci.ForwardingScore = proto.Uint32(existingScore + 1)
}

// getOrCreateContextInfo returns the ContextInfo from the populated message
// type, creating one if it doesn't exist. Returns nil only if the message
// type is unknown/unsupported.
func getOrCreateContextInfo(m *waE2E.Message) *waE2E.ContextInfo {
	if v := m.GetExtendedTextMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetImageMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetDocumentMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetAudioMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetVideoMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetStickerMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetContactMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetLocationMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetTemplateMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	if v := m.GetListMessage(); v != nil {
		if v.ContextInfo == nil {
			v.ContextInfo = &waE2E.ContextInfo{}
		}
		return v.ContextInfo
	}
	return nil
}

var _ appport.ForwardedMessageSender = (*ForwardedMessageAdapter)(nil)
