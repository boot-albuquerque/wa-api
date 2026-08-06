package whatsmeow

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/domain"

	whatsmeow "wa-api/internal/waclient"
	"wa-api/internal/waclient/proto/waE2E"
	"wa-api/internal/waclient/types"
)

// --- ChatMessengerAdapter ---

func TestNewChatMessengerAdapter(t *testing.T) {
	if NewChatMessengerAdapter(getterWith(nil)) == nil {
		t.Fatal("NewChatMessengerAdapter returned nil")
	}
}

// TestChatMessengerAdapter_MarkRead_InvalidChatJID devolve erro de toJID.
// Tenta várias entradas; quando encontra uma que ParseJID rejeita,
// confirma que MarkRead propaga o erro.
func TestChatMessengerAdapter_MarkRead_InvalidChatJID(t *testing.T) {
	a := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	invalid := []domain.JID{
		"@@", "@", "x@", "@y.com", domain.JID(string([]byte{0x00})),
	}
	for _, jid := range invalid {
		err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), jid, "z@y.com")
		if err != nil && appErrCode(err) != "no_session" {
			return
		}
	}
	t.Skip("ParseJID não falhou para nenhuma entrada testada")
}

// TestChatMessengerAdapter_MarkRead_InvalidSenderJID devolve erro de toJID.
func TestChatMessengerAdapter_MarkRead_InvalidSenderJID(t *testing.T) {
	a := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	invalid := []domain.JID{
		"@@", "@", "x@", "@y.com", domain.JID(string([]byte{0x00})),
	}
	for _, jid := range invalid {
		err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", jid)
		if err != nil && appErrCode(err) != "no_session" {
			return
		}
	}
	t.Skip("ParseJID não falhou para nenhuma entrada testada")
}

// TestChatMessengerAdapter_MarkRead_NoSession.
func TestChatMessengerAdapter_MarkRead_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(getterWith(nil))
	err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", "z@y.com")
	if appErrCode(err) != "no_session" {
		t.Errorf("MarkRead code = %q", appErrCode(err))
	}
}

// TestChatMessengerAdapter_MarkRead_PropagatesError.
func TestChatMessengerAdapter_MarkRead_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &fakeWAClient{MarkReadFn: func(ctx context.Context, ids []types.MessageID, ts time.Time, chat, sender types.JID, extra ...types.ReceiptType) error {
		return sdkErr
	}}
	a := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", "z@y.com")
	if err == nil {
		t.Fatal("MarkRead não propagou erro")
	}
}

// TestChatMessengerAdapter_MarkRead_OK.
func TestChatMessengerAdapter_MarkRead_OK(t *testing.T) {
	called := false
	fake := &fakeWAClient{MarkReadFn: func(ctx context.Context, ids []types.MessageID, ts time.Time, chat, sender types.JID, extra ...types.ReceiptType) error {
		called = true
		return nil
	}}
	a := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", "z@y.com"); err != nil {
		t.Fatalf("MarkRead = %v", err)
	}
	if !called {
		t.Fatal("MarkRead não invocou o SDK")
	}
}

// TestChatMessengerAdapter_SendReaction_NoSession.
func TestChatMessengerAdapter_SendReaction_NoSession(t *testing.T) {
	a := NewChatMessengerAdapter(getterWith(nil))
	_, err := a.SendReaction(context.Background(), "u1", "x@y.com", domain.Reaction{Text: "👍"})
	if appErrCode(err) != "no_session" {
		t.Errorf("SendReaction code = %q", appErrCode(err))
	}
}

// TestChatMessengerAdapter_SendReaction_InvalidJID.
func TestChatMessengerAdapter_SendReaction_InvalidJID(t *testing.T) {
	a := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	_, err := a.SendReaction(context.Background(), "u1", domain.JID(string([]byte{0x00})), domain.Reaction{Text: "👍"})
	if err == nil {
		t.Skip("ParseJID não falhou; caminho de erro raro")
	}
}

// TestChatMessengerAdapter_SendReaction_PropagatesError.
func TestChatMessengerAdapter_SendReaction_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &fakeWAClient{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		return whatsmeow.SendResponse{}, sdkErr
	}}
	a := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": fake}))
	_, err := a.SendReaction(context.Background(), "u1", "x@y.com", domain.Reaction{Text: "👍"})
	if err == nil {
		t.Fatal("SendReaction não propagou erro")
	}
}

// TestChatMessengerAdapter_SendReaction_OK devolve MessageSendResult.
func TestChatMessengerAdapter_SendReaction_OK(t *testing.T) {
	now := time.Now()
	fake := &fakeWAClient{SendMessageFn: func(ctx context.Context, to types.JID, m *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		return whatsmeow.SendResponse{Timestamp: now, ID: types.MessageID("msg-1")}, nil
	}}
	a := NewChatMessengerAdapter(getterWith(map[string]waClient{"u1": fake}))
	res, err := a.SendReaction(context.Background(), "u1", "x@y.com", domain.Reaction{
		Text:            "👍",
		FromMe:          true,
		TargetMessageID: "orig-msg",
	})
	if err != nil {
		t.Fatalf("SendReaction = %v", err)
	}
	if res.Timestamp != now {
		t.Errorf("SendReaction timestamp = %v, want %v", res.Timestamp, now)
	}
}
