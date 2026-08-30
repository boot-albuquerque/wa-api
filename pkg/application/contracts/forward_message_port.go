package port

import (
	"context"

	"wa-api/pkg/domain"
)

// StoredMessageReader retrieves a stored message from history for forwarding
// (CAP-55). The implementation reads the datajson column from message_history.
type StoredMessageReader interface {
	// GetStoredMessage returns the stored message data for (userID, messageID).
	// Returns an apperr with CategoryNotFound when the message does not exist
	// or has no datajson.
	GetStoredMessage(ctx context.Context, userID, messageID string) (*domain.StoredMessageData, error)
}

// ForwardedMessageSender sends a stored message as forwarded (CAP-55).
//
// The implementation deserializes the datajson into the wire proto, reads the
// existing forwarding score from the proto's ContextInfo, INCREMENTS it by 1,
// sets IsForwarded=true, and sends via the noise client — no re-upload of
// media.
type ForwardedMessageSender interface {
	SessionGuard

	// SendForwardedMessage deserializes dataJSON into the wire proto, derives
	// the forwarding score (existing + 1), sets IsForwarded=true, and sends
	// to target. id, when non-empty, is the caller's chosen message ID.
	SendForwardedMessage(ctx context.Context, txtID string, target domain.JID, dataJSON string, id string) (domain.MessageSendResult, error)
}
