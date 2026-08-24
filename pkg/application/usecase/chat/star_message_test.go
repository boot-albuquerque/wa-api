package chat_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

const starTestUserID = "u1"

func validStarRequest() domain.StarMessageRequest {
	return domain.StarMessageRequest{
		Chat:      "120363111111111111@g.us",
		Sender:    "5511999999999@s.whatsapp.net",
		MessageID: "ABCDE12345",
		FromMe:    false,
		Star:      true,
	}
}

func TestStarMessage_NoSession(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	ms.EnsureSessionFunc = func(context.Context, string) error {
		return apperr.New("no_session", apperr.CategoryValidation, "no session", false, nil)
	}
	uc := chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, &contractsfake.Logger{})
	_, err := uc.Execute(context.Background(), starTestUserID, validStarRequest())
	if err == nil {
		t.Fatal("expected error for no session")
	}
	if len(ms.StarMessageCalls) != 0 {
		t.Error("StarMessage should not be called without session")
	}
}

func TestStarMessage_MissingChat(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	uc := chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, &contractsfake.Logger{})
	req := validStarRequest()
	req.Chat = ""
	_, err := uc.Execute(context.Background(), starTestUserID, req)
	if err == nil {
		t.Fatal("expected validation error for missing chat")
	}
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != "missing_chat" {
		t.Errorf("error code = %v, want missing_chat", err)
	}
}

func TestStarMessage_MissingMessageID(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	uc := chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, &contractsfake.Logger{})
	req := validStarRequest()
	req.MessageID = ""
	_, err := uc.Execute(context.Background(), starTestUserID, req)
	if err == nil {
		t.Fatal("expected validation error for missing message_id")
	}
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != "missing_message_id" {
		t.Errorf("error code = %v, want missing_message_id", err)
	}
}

func TestStarMessage_MissingSender(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	uc := chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, &contractsfake.Logger{})
	req := validStarRequest()
	req.Sender = ""
	_, err := uc.Execute(context.Background(), starTestUserID, req)
	if err == nil {
		t.Fatal("expected validation error for missing sender")
	}
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != "missing_sender" {
		t.Errorf("error code = %v, want missing_sender", err)
	}
}

func TestStarMessage_InvalidChatJID(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	jr := &contractsfake.JIDResolver{
		ResolveQualifiedJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if raw == "invalid" {
				return "", errors.New("bad jid")
			}
			return domain.JID(raw), nil
		},
	}
	uc := chat.NewStarMessageUseCase(ms, jr, &contractsfake.Logger{})
	req := validStarRequest()
	req.Chat = "invalid"
	_, err := uc.Execute(context.Background(), starTestUserID, req)
	if err == nil {
		t.Fatal("expected validation error for invalid chat JID")
	}
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != "invalid_chat_jid" {
		t.Errorf("error code = %v, want invalid_chat_jid", err)
	}
}

func TestStarMessage_InvalidSenderJID(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	jr := &contractsfake.JIDResolver{
		ResolveQualifiedJIDFunc: func(_ context.Context, raw string) (domain.JID, error) {
			if raw == "invalid-sender" {
				return "", errors.New("bad jid")
			}
			return domain.JID(raw), nil
		},
	}
	uc := chat.NewStarMessageUseCase(ms, jr, &contractsfake.Logger{})
	req := validStarRequest()
	req.Sender = "invalid-sender"
	_, err := uc.Execute(context.Background(), starTestUserID, req)
	if err == nil {
		t.Fatal("expected validation error for invalid sender JID")
	}
	var ae *apperr.AppError
	if !errors.As(err, &ae) || ae.Code != "invalid_sender_jid" {
		t.Errorf("error code = %v, want invalid_sender_jid", err)
	}
}

func TestStarMessage_PortError(t *testing.T) {
	boom := errors.New("sdk down")
	ms := &contractsfake.MessageStarrer{
		StarMessageFunc: func(context.Context, string, domain.JID, domain.JID, string, bool, bool) error {
			return boom
		},
	}
	log := &contractsfake.Logger{}
	uc := chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, log)
	_, err := uc.Execute(context.Background(), starTestUserID, validStarRequest())
	if err == nil {
		t.Fatal("expected error from port")
	}
	if !log.Logged("failed to star message") {
		t.Error("expected error log for port failure")
	}
}

func TestStarMessage_Star_OK(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	uc := chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, &contractsfake.Logger{})
	req := validStarRequest()
	req.Star = true
	rsp, err := uc.Execute(context.Background(), starTestUserID, req)
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if !rsp.Success {
		t.Error("expected success=true")
	}
	if rsp.Message != "Message starred" {
		t.Errorf("message = %q, want %q", rsp.Message, "Message starred")
	}
	if len(ms.StarMessageCalls) != 1 {
		t.Fatalf("StarMessage calls = %d, want 1", len(ms.StarMessageCalls))
	}
	c := ms.StarMessageCalls[0]
	if c.Chat != domain.JID(req.Chat) {
		t.Errorf("chat = %q, want %q", c.Chat, req.Chat)
	}
	if c.Sender != domain.JID(req.Sender) {
		t.Errorf("sender = %q, want %q", c.Sender, req.Sender)
	}
	if c.MessageID != req.MessageID {
		t.Errorf("messageID = %q, want %q", c.MessageID, req.MessageID)
	}
	if c.FromMe != false {
		t.Error("fromMe should be false")
	}
	if c.Star != true {
		t.Error("star should be true")
	}
}

func TestStarMessage_Unstar_OK(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	uc := chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, &contractsfake.Logger{})
	req := validStarRequest()
	req.Star = false
	rsp, err := uc.Execute(context.Background(), starTestUserID, req)
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if rsp.Message != "Message unstarred" {
		t.Errorf("message = %q, want %q", rsp.Message, "Message unstarred")
	}
	if len(ms.StarMessageCalls) != 1 {
		t.Fatalf("StarMessage calls = %d, want 1", len(ms.StarMessageCalls))
	}
	if ms.StarMessageCalls[0].Star != false {
		t.Error("star should be false for unstar")
	}
}

func TestStarMessage_FromMe_ForwardedToPort(t *testing.T) {
	ms := &contractsfake.MessageStarrer{}
	uc := chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, &contractsfake.Logger{})
	req := validStarRequest()
	req.FromMe = true
	_, err := uc.Execute(context.Background(), starTestUserID, req)
	if err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if len(ms.StarMessageCalls) != 1 {
		t.Fatalf("StarMessage calls = %d, want 1", len(ms.StarMessageCalls))
	}
	if !ms.StarMessageCalls[0].FromMe {
		t.Error("fromMe should be true when request has FromMe=true")
	}
}
