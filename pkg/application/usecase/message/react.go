package message

import (
	"context"
	"fmt"
	"strings"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/whatsmeow"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ReactUseCase sends a reaction to a message
type ReactUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewReactUseCase creates a new instance
func NewReactUseCase(cp appport.ClientProvider, logger appport.Logger) *ReactUseCase {
	return &ReactUseCase{clientProvider: cp, logger: logger}
}

// Execute sends a reaction
func (uc *ReactUseCase) Execute(ctx context.Context, userID string, req domain.ReactRequest) (map[string]interface{}, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in Payload")
	}

	if req.Body == "" {
		return nil, fmt.Errorf("missing Body in Payload")
	}

	recipient, ok := whatsmeow.ParseJID(req.Phone)
	if !ok {
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

	var participantJID types.JID
	if !fromMe && req.Participant != "" {
		if pj, ok := whatsmeow.ParseJID(req.Participant); ok {
			participantJID = pj
		}
	}

	key := &waCommon.MessageKey{
		RemoteJID: proto.String(recipient.String()),
		FromMe:    proto.Bool(fromMe),
		ID:        proto.String(msgid),
	}
	if !fromMe && participantJID.String() != "" {
		key.Participant = proto.String(participantJID.String())
	}

	msg := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key:               key,
			Text:              proto.String(reaction),
			GroupingKey:       proto.String(reaction),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}

	resp, err := client.SendMessage(ctx, recipient, msg)
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
