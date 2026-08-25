package message_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// newForwardUC is a helper that builds the use case with only the by-content
// dependencies. Tests for the by-key path supply their own fakes.
func newForwardUC(tm *contractsfake.TextMessenger, jr *contractsfake.JIDResolver) *message.SendForwardUseCase {
	return message.NewSendForwardUseCase(
		tm,
		&contractsfake.ForwardedMessageSender{},
		&contractsfake.StoredMessageReader{},
		jr,
		&contractsfake.Logger{},
	)
}

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

			_, err := newForwardUC(tm, &contractsfake.JIDResolver{}).
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

	_, err := newForwardUC(tm, &contractsfake.JIDResolver{}).
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

	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errJID },
	}

	_, err := newForwardUC(tm, jr).
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

	result, err := newForwardUC(tm, &contractsfake.JIDResolver{}).
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

	result, err := newForwardUC(tm, &contractsfake.JIDResolver{}).
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

	_, err := newForwardUC(tm, &contractsfake.JIDResolver{}).
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

	result, err := newForwardUC(tm, &contractsfake.JIDResolver{}).
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

	_, err := newForwardUC(tm, &contractsfake.JIDResolver{}).
		Execute(context.Background(), userID, domain.SendForwardRequest{Phone: "5511987654321", Body: "fwd"})

	if err != nil {
		t.Fatalf("happy path failed: %v", err)
	}
}

// --- CAP-55: Forward by key ---

// fakeDataJSON builds a minimal datajson blob with an ExtendedTextMessage
// carrying the given forwarding score, mimicking the real structure written by
// eventhandler_message.go:322 (json.Marshal(events.Message)).
//
// Production path: pkg/infra/db/message_history.go:23 (SaveMessageToHistory).
func fakeDataJSON(text string, forwardingScore uint32) string {
	type contextInfo struct {
		IsForwarded     bool   `json:"isForwarded,omitempty"`
		ForwardingScore uint32 `json:"forwardingScore,omitempty"`
	}
	type extendedText struct {
		Text        string       `json:"text"`
		ContextInfo *contextInfo `json:"contextInfo,omitempty"`
	}
	type protoMsg struct {
		ExtendedTextMessage *extendedText `json:"extendedTextMessage,omitempty"`
	}
	type evtMsg struct {
		Message *protoMsg `json:"Message"`
	}

	evt := evtMsg{Message: &protoMsg{ExtendedTextMessage: &extendedText{Text: text}}}
	if forwardingScore > 0 {
		evt.Message.ExtendedTextMessage.ContextInfo = &contextInfo{
			IsForwarded:     true,
			ForwardingScore: forwardingScore,
		}
	}
	b, _ := json.Marshal(evt)
	return string(b)
}

func TestSendForwardByKey_Success_ScoreDerived(t *testing.T) {
	sentAt := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	dataJSON := fakeDataJSON("hello from history", 3)

	fwd := &contractsfake.ForwardedMessageSender{
		SendForwardedMessageFunc: func(_ context.Context, _ string, target domain.JID, gotJSON string, _ string) (domain.MessageSendResult, error) {
			if target != "5511999999999@s.whatsapp.net" {
				t.Errorf("target: got %q, want 5511999999999@s.whatsapp.net", target)
			}
			if gotJSON != dataJSON {
				t.Error("dataJSON not forwarded to sender")
			}
			return domain.MessageSendResult{ID: "wire-fwd-key-1", Timestamp: sentAt}, nil
		},
	}
	smr := &contractsfake.StoredMessageReader{
		GetStoredMessageFunc: func(_ context.Context, userID, messageID string) (*domain.StoredMessageData, error) {
			if messageID != "ABC123" {
				t.Errorf("messageID: got %q, want ABC123", messageID)
			}
			return &domain.StoredMessageData{DataJSON: dataJSON, ChatJID: "120363041454003546@g.us"}, nil
		},
	}

	uc := message.NewSendForwardUseCase(
		&contractsfake.TextMessenger{},
		fwd,
		smr,
		&contractsfake.JIDResolver{},
		&contractsfake.Logger{},
	)

	result, err := uc.Execute(context.Background(), userID,
		domain.SendForwardRequest{Phone: "5511999999999", MessageID: "ABC123"})

	if err != nil {
		t.Fatalf("by-key forward failed: %v", err)
	}
	if result.MessageID != "wire-fwd-key-1" {
		t.Errorf("MessageID: got %q, want wire-fwd-key-1", result.MessageID)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}

	if n := len(fwd.SendForwardedMessageCalls); n != 1 {
		t.Fatalf("SendForwardedMessage called %d time(s), want 1", n)
	}
}

// Test 2: Score is DERIVED, not caller-supplied. Even if the payload sends
// ForwardingScore: 99, the by-key path ignores it — the score comes from
// the stored message's ContextInfo (incremented by the adapter).
func TestSendForwardByKey_ScoreIgnoresPayload(t *testing.T) {
	dataJSON := fakeDataJSON("hello", 3)
	score99 := uint32(99)

	fwd := &contractsfake.ForwardedMessageSender{
		SendForwardedMessageFunc: func(_ context.Context, _ string, _ domain.JID, gotJSON string, _ string) (domain.MessageSendResult, error) {
			if gotJSON != dataJSON {
				t.Error("dataJSON should be forwarded as-is; score derivation is adapter's job")
			}
			return domain.MessageSendResult{ID: "wire-fwd-ignore-score"}, nil
		},
	}
	smr := &contractsfake.StoredMessageReader{
		GetStoredMessageFunc: func(_ context.Context, _, _ string) (*domain.StoredMessageData, error) {
			return &domain.StoredMessageData{DataJSON: dataJSON, ChatJID: "chat@g.us"}, nil
		},
	}

	uc := message.NewSendForwardUseCase(
		&contractsfake.TextMessenger{},
		fwd,
		smr,
		&contractsfake.JIDResolver{},
		&contractsfake.Logger{},
	)

	result, err := uc.Execute(context.Background(), userID,
		domain.SendForwardRequest{
			Phone:           "5511999999999",
			MessageID:       "ABC123",
			ForwardingScore: &score99,
		})

	if err != nil {
		t.Fatalf("by-key forward failed: %v", err)
	}
	if result.MessageID != "wire-fwd-ignore-score" {
		t.Errorf("MessageID: got %q, want wire-fwd-ignore-score", result.MessageID)
	}
}

func TestSendForwardByKey_MessageNotFound(t *testing.T) {
	fwd := &contractsfake.ForwardedMessageSender{}
	smr := &contractsfake.StoredMessageReader{
		GetStoredMessageFunc: func(_ context.Context, _, _ string) (*domain.StoredMessageData, error) {
			return nil, apperr.New("message_not_found", apperr.CategoryNotFound, "message not found in history", false, nil)
		},
	}

	uc := message.NewSendForwardUseCase(
		&contractsfake.TextMessenger{},
		fwd,
		smr,
		&contractsfake.JIDResolver{},
		&contractsfake.Logger{},
	)

	_, err := uc.Execute(context.Background(), userID,
		domain.SendForwardRequest{Phone: "5511999999999", MessageID: "NONEXISTENT"})

	if err == nil {
		t.Fatal("expected error for non-existent message")
	}
	var ae *apperr.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if ae.Category != apperr.CategoryNotFound {
		t.Errorf("category: got %q, want %q", ae.Category, apperr.CategoryNotFound)
	}
	if ae.Code != "message_not_found" {
		t.Errorf("code: got %q, want message_not_found", ae.Code)
	}
	if n := len(fwd.SendForwardedMessageCalls); n != 0 {
		t.Errorf("message not found but SendForwardedMessage was called %d time(s)", n)
	}
}

// Test 4: Backward compat — by-content (Phone+Body) still works.
func TestSendForwardByContent_StillWorks(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, text string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, fwd *domain.ForwardContext, _ string) (domain.MessageSendResult, error) {
			if text != "old-style forward" {
				t.Errorf("text: got %q", text)
			}
			if fwd == nil || fwd.ForwardingScore != 1 {
				t.Errorf("expected default score 1, got %+v", fwd)
			}
			return domain.MessageSendResult{ID: "wire-compat"}, nil
		},
	}

	uc := message.NewSendForwardUseCase(
		tm,
		&contractsfake.ForwardedMessageSender{},
		&contractsfake.StoredMessageReader{},
		&contractsfake.JIDResolver{},
		&contractsfake.Logger{},
	)

	result, err := uc.Execute(context.Background(), userID,
		domain.SendForwardRequest{Phone: "5511999999999", Body: "old-style forward"})

	if err != nil {
		t.Fatalf("by-content forward failed: %v", err)
	}
	if result.MessageID != "wire-compat" {
		t.Errorf("MessageID: got %q, want wire-compat", result.MessageID)
	}
}
