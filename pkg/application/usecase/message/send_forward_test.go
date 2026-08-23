package message_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

func TestSendForward_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendForwardRequest
	}{
		{"Phone", domain.SendForwardRequest{Body: "fwd text"}},
		{"Body", domain.SendForwardRequest{Phone: "5511987654321"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tm := &contractsfake.TextMessenger{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendForwardUseCase(tm, &contractsfake.JIDResolver{}, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request without %s was accepted", tc.name)
			}
			if n := len(tm.EnsureSessionCalls); n != 0 {
				t.Errorf("validation failed but session was checked %d time(s)", n)
			}
			if n := len(tm.SendTextCalls); n != 0 {
				t.Errorf("validation failed but SendText was called %d time(s)", n)
			}
		})
	}
}

func TestSendForward_SessionFailurePropagates(t *testing.T) {
	tm := &contractsfake.TextMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendForwardUseCase(tm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendForwardRequest{Phone: "5511987654321", Body: "fwd"})

	if !errors.Is(err, errSession) {
		t.Fatalf("port error did not propagate: got %#v", err)
	}
	if n := len(tm.SendTextCalls); n != 0 {
		t.Errorf("no session but SendText was called %d time(s)", n)
	}
}

func TestSendForward_InvalidPhoneNeverReachesSendText(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	logger := &contractsfake.Logger{}

	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	_, err := message.NewSendForwardUseCase(tm, jr, logger).
		Execute(context.Background(), userID, domain.SendForwardRequest{Phone: "garbage", Body: "fwd"})

	if err == nil {
		t.Fatal("invalid phone was accepted")
	}
	if n := len(tm.SendTextCalls); n != 0 {
		t.Fatalf("invalid JID but SendText was called %d time(s)", n)
	}
}

func TestSendForward_Success_DefaultScore(t *testing.T) {
	sentAt := time.Date(2026, 8, 23, 14, 0, 0, 0, time.UTC)
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, fwd *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			if fwd == nil {
				t.Fatal("ForwardContext is nil — forwarding was not applied")
			}
			if fwd.ForwardingScore != 1 {
				t.Errorf("ForwardingScore: got %d, want 1 (default)", fwd.ForwardingScore)
			}
			return domain.MessageSendResult{Timestamp: sentAt, ID: "wire-fwd-1"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendForwardUseCase(tm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendForwardRequest{Phone: "5511987654321", Body: "forwarded text"})

	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText called %d time(s), want 1", n)
	}
	call := tm.SendTextCalls[0]
	if call.Target != domain.JID("5511987654321@s.whatsapp.net") {
		t.Errorf("target: got %q, want %q", call.Target, "5511987654321@s.whatsapp.net")
	}
	if call.Text != "forwarded text" {
		t.Errorf("text: got %q, want %q", call.Text, "forwarded text")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-fwd-1" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "wire-fwd-1")
	}
	if result.Timestamp != sentAt.Unix() {
		t.Errorf("Timestamp: got %d, want %d", result.Timestamp, sentAt.Unix())
	}
}

func TestSendForward_Success_CustomScore(t *testing.T) {
	score := uint32(5)
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, fwd *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			if fwd == nil {
				t.Fatal("ForwardContext is nil")
			}
			if fwd.ForwardingScore != 5 {
				t.Errorf("ForwardingScore: got %d, want 5", fwd.ForwardingScore)
			}
			return domain.MessageSendResult{ID: "wire-fwd-5"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendForwardUseCase(tm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendForwardRequest{Phone: "5511987654321", Body: "fwd", ForwardingScore: &score})

	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
	if result.MessageID != "wire-fwd-5" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "wire-fwd-5")
	}
}

func TestSendForward_ZeroScoreDefaultsToOne(t *testing.T) {
	score := uint32(0)
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, fwd *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			if fwd == nil {
				t.Fatal("ForwardContext is nil")
			}
			if fwd.ForwardingScore != 1 {
				t.Errorf("ForwardingScore: got %d, want 1 (zero should default to 1)", fwd.ForwardingScore)
			}
			return domain.MessageSendResult{ID: "wire-fwd-z"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendForwardUseCase(tm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendForwardRequest{Phone: "5511987654321", Body: "fwd", ForwardingScore: &score})

	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
}

func TestSendForward_DownstreamFailure(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(context.Context, string, domain.JID, string, *domain.LinkPreviewData, *domain.ReplyContext, []string, *domain.ForwardContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendForwardUseCase(tm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendForwardRequest{Phone: "5511987654321", Body: "fwd"})

	if !errors.Is(err, errDownstream) {
		t.Fatalf("downstream error not propagated: got %#v", err)
	}
	if result != nil {
		t.Errorf("result should be nil on error, got %+v", result)
	}
}

func TestSendForward_NilPreviewPassedToSendText(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, preview *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, _ *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			if preview != nil {
				t.Errorf("forward use case should pass nil preview, got %+v", preview)
			}
			return domain.MessageSendResult{ID: "wire-fwd-np"}, nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendForwardUseCase(tm, &contractsfake.JIDResolver{}, logger).
		Execute(context.Background(), userID, domain.SendForwardRequest{Phone: "5511987654321", Body: "fwd"})

	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
}
