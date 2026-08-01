package message

import (
	"context"
	"fmt"
	"strings"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ReactUseCase sends a reaction to a message
type ReactUseCase struct {
	chats  appport.ChatMessenger
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewReactUseCase creates a new instance
func NewReactUseCase(cm appport.ChatMessenger, jr appport.JIDResolver, logger appport.Logger) *ReactUseCase {
	return &ReactUseCase{chats: cm, jids: jr, logger: logger}
}

// Execute sends a reaction
func (uc *ReactUseCase) Execute(ctx context.Context, userID string, req domain.ReactRequest) (map[string]interface{}, error) {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		return nil, fmt.Errorf("no session")
	}

	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in Payload")
	}

	if req.Body == "" {
		return nil, fmt.Errorf("missing Body in Payload")
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		return nil, fmt.Errorf("could not parse Phone")
	}

	msgid := req.Id
	if msgid == "" {
		return nil, fmt.Errorf("missing Id in Payload")
	}

	fromMe := false
	if strings.HasPrefix(msgid, "me:") {
		fromMe = true
		msgid = msgid[len("me:"):]
	}

	reaction := req.Body
	if reaction == "remove" {
		reaction = ""
	}

	// Um Participant que não resolve é ignorado, e não vira erro —
	// comportamento preservado do upstream.
	var participant domain.JID
	if !fromMe && req.Participant != "" {
		if pj, err := uc.jids.ResolveJID(ctx, req.Participant); err == nil {
			participant = pj
		}
	}

	resp, err := uc.chats.SendReaction(ctx, userID, recipient, domain.Reaction{
		TargetMessageID: msgid,
		FromMe:          fromMe,
		Participant:     participant,
		Text:            reaction,
		SentAt:          time.Now(),
	})
	if err != nil {
		uc.logger.Error(ctx, "Error sending reaction", "error", err, "user_id", userID)
		return nil, fmt.Errorf("error sending message: %v", err)
	}

	uc.logger.Info(ctx, "Reaction sent", "timestamp", fmt.Sprintf("%v", resp.Timestamp), "id", msgid, "user_id", userID)

	return map[string]interface{}{
		"Details":   "Sent",
		"Timestamp": resp.Timestamp.Unix(),
		"Id":        msgid,
	}, nil
}
