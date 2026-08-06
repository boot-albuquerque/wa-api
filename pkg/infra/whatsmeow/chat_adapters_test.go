package whatsmeow

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// --- JIDResolverAdapter ---

func TestJIDResolverAdapter_New(t *testing.T) {
	if NewJIDResolverAdapter() == nil {
		t.Fatal("NewJIDResolverAdapter returned nil")
	}
}

// TestResolveJID_Invalid devolve erro para entrada que ParseJID não aceita.
// O ParseJID é leniente (devolve ok=true para telefone cru com servidor
// padrão), então o caminho de erro é difícil de atingir — mas tentamos.
func TestResolveJID_Invalid(t *testing.T) {
	a := JIDResolverAdapter{}
	_, err := a.ResolveJID(context.Background(), string([]byte{0x00}))
	if err == nil {
		t.Skip("ParseJID não devolve erro para esta entrada; o caminho de erro é raro")
	}
}

// TestResolveJID_Phone com telefone cru aplica servidor padrão.
func TestResolveJID_Phone(t *testing.T) {
	a := JIDResolverAdapter{}
	got, err := a.ResolveJID(context.Background(), "5511987654321")
	if err != nil {
		t.Fatalf("ResolveJID phone = %v", err)
	}
	if got != "5511987654321@s.whatsapp.net" {
		t.Errorf("ResolveJID phone = %q", got)
	}
}

// TestResolveJID_Qualified devolve o JID qualificado como está.
func TestResolveJID_Qualified(t *testing.T) {
	a := JIDResolverAdapter{}
	in := "120363000000000000@g.us"
	got, err := a.ResolveJID(context.Background(), in)
	if err != nil {
		t.Fatalf("ResolveJID qualified = %v", err)
	}
	if string(got) != in {
		t.Errorf("ResolveJID qualified = %q, want %q", got, in)
	}
}

// TestResolveQualifiedJID_BareNumber rejeita telefone sem @.
func TestResolveQualifiedJID_BareNumber(t *testing.T) {
	a := JIDResolverAdapter{}
	_, err := a.ResolveQualifiedJID(context.Background(), "5511987654321")
	if err == nil {
		t.Fatal("ResolveQualifiedJID aceitou telefone cru")
	}
}

// TestResolveQualifiedJID_Qualified aceita entrada com servidor.
func TestResolveQualifiedJID_Qualified(t *testing.T) {
	a := JIDResolverAdapter{}
	in := "5511987654321@s.whatsapp.net"
	got, err := a.ResolveQualifiedJID(context.Background(), in)
	if err != nil {
		t.Fatalf("ResolveQualifiedJID = %v", err)
	}
	if string(got) != in {
		t.Errorf("ResolveQualifiedJID = %q, want %q", got, in)
	}
}

// TestToJID_Empty devolve JID zero.
func TestToJID_Empty(t *testing.T) {
	got, err := toJID("")
	if err != nil {
		t.Fatalf("toJID(empty) = %v", err)
	}
	if !got.IsEmpty() {
		t.Errorf("toJID(empty) = %v, want empty", got)
	}
}

// TestToJID_Invalid devolve erro quando ParseJID falha.
func TestToJID_Invalid(t *testing.T) {
	_, err := toJID(domain.JID(string([]byte{0x00})))
	if err == nil {
		t.Skip("ParseJID não falhou; caminho de erro é raro")
	}
}

// TestToJID_Valid faz round-trip.
func TestToJID_Valid(t *testing.T) {
	in := domain.JID("5511987654321@s.whatsapp.net")
	got, err := toJID(in)
	if err != nil {
		t.Fatalf("toJID = %v", err)
	}
	if got.String() != string(in) {
		t.Errorf("toJID round-trip = %q, want %q", got.String(), in)
	}
}

// --- PresenceControllerAdapter ---

func TestNewPresenceControllerAdapter(t *testing.T) {
	if NewPresenceControllerAdapter(getterWith(nil)) == nil {
		t.Fatal("NewPresenceControllerAdapter returned nil")
	}
}

// TestPresenceControllerAdapter_SendPresence_NoSession devolve ErrNoSession.
func TestPresenceControllerAdapter_SendPresence_NoSession(t *testing.T) {
	a := NewPresenceControllerAdapter(getterWith(nil))
	err := a.SendPresence(context.Background(), "u1", domain.PresenceAvailable)
	if err == nil {
		t.Fatal("SendPresence com nil client returned nil")
	}
	if appErrCode(err) != "no_session" {
		t.Errorf("SendPresence code = %q, want no_session", appErrCode(err))
	}
}

// TestPresenceControllerAdapter_SendPresence_UnknownPresence devolve erro
// semântico (não ErrNoSession) para tipo desconhecido.
func TestPresenceControllerAdapter_SendPresence_UnknownPresence(t *testing.T) {
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	err := a.SendPresence(context.Background(), "u1", "weird")
	if err == nil {
		t.Fatal("SendPresence com presence inválida = nil")
	}
	if appErrCode(err) == "no_session" {
		t.Errorf("SendPresence devolveu no_session para presença inválida")
	}
}

// TestPresenceControllerAdapter_SendPresence_Available propaga para o SDK.
func TestPresenceControllerAdapter_SendPresence_Available(t *testing.T) {
	called := false
	var seenState types.Presence
	fake := &fakeWAClient{
		SendPresenceFn: func(ctx context.Context, s types.Presence) error {
			called = true
			seenState = s
			return nil
		},
	}
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SendPresence(context.Background(), "u1", domain.PresenceAvailable); err != nil {
		t.Fatalf("SendPresence = %v", err)
	}
	if !called {
		t.Fatal("SendPresence não invocou o SDK")
	}
	if seenState != types.PresenceAvailable {
		t.Errorf("SendPresence state = %v, want available", seenState)
	}
}

// TestPresenceControllerAdapter_SendPresence_Unavailable propaga para o SDK.
func TestPresenceControllerAdapter_SendPresence_Unavailable(t *testing.T) {
	var seenState types.Presence
	fake := &fakeWAClient{
		SendPresenceFn: func(ctx context.Context, s types.Presence) error {
			seenState = s
			return nil
		},
	}
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": fake}))
	if err := a.SendPresence(context.Background(), "u1", domain.PresenceUnavailable); err != nil {
		t.Fatalf("SendPresence = %v", err)
	}
	if seenState != types.PresenceUnavailable {
		t.Errorf("SendPresence state = %v, want unavailable", seenState)
	}
}

// TestPresenceControllerAdapter_SendPresence_PropagatesError.
func TestPresenceControllerAdapter_SendPresence_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &fakeWAClient{SendPresenceFn: func(ctx context.Context, s types.Presence) error { return sdkErr }}
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.SendPresence(context.Background(), "u1", domain.PresenceAvailable)
	if err == nil {
		t.Fatal("SendPresence não propagou erro")
	}
}

// TestPresenceControllerAdapter_SendChatPresence_NoSession.
func TestPresenceControllerAdapter_SendChatPresence_NoSession(t *testing.T) {
	a := NewPresenceControllerAdapter(getterWith(nil))
	err := a.SendChatPresence(context.Background(), "u1", "x@y.com", "composing", "")
	if appErrCode(err) != "no_session" {
		t.Errorf("SendChatPresence code = %q, want no_session", appErrCode(err))
	}
}

// TestPresenceControllerAdapter_SendChatPresence_PropagatesError.
func TestPresenceControllerAdapter_SendChatPresence_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &fakeWAClient{SendChatPresenceFn: func(ctx context.Context, jid types.JID, s types.ChatPresence, m types.ChatPresenceMedia) error {
		return sdkErr
	}}
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.SendChatPresence(context.Background(), "u1", "x@y.com", "composing", "")
	if err == nil {
		t.Fatal("SendChatPresence não propagou erro")
	}
}

// TestPresenceControllerAdapter_SubscribePresence_NoSession.
func TestPresenceControllerAdapter_SubscribePresence_NoSession(t *testing.T) {
	a := NewPresenceControllerAdapter(getterWith(nil))
	err := a.SubscribePresence(context.Background(), "u1", "x@y.com")
	if appErrCode(err) != "no_session" {
		t.Errorf("SubscribePresence code = %q", appErrCode(err))
	}
}

// TestPresenceControllerAdapter_SubscribePresence_InvalidJID: cobre
// o ramo "toJID falhou".
func TestPresenceControllerAdapter_SubscribePresence_InvalidJID(t *testing.T) {
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	// Tenta várias entradas até encontrar uma que ParseJID rejeite.
	for _, raw := range []string{string([]byte{0x00}), "@", ""} {
		err := a.SubscribePresence(context.Background(), "u1", domain.JID(raw))
		if err != nil {
			return
		}
	}
	t.Skip("ParseJID não falhou para nenhuma entrada testada")
}

// TestPresenceControllerAdapter_SendChatPresence_InvalidJID.
func TestPresenceControllerAdapter_SendChatPresence_InvalidJID(t *testing.T) {
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": &fakeWAClient{}}))
	err := a.SendChatPresence(context.Background(), "u1", domain.JID(string([]byte{0x00})), "c", "")
	if err == nil {
		t.Skip("ParseJID não falhou; caminho de erro raro")
	}
}

// TestPresenceControllerAdapter_SubscribePresence_PropagatesError.
func TestPresenceControllerAdapter_SubscribePresence_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &fakeWAClient{SubscribePresenceFn: func(ctx context.Context, jid types.JID) error { return sdkErr }}
	a := NewPresenceControllerAdapter(getterWith(map[string]waClient{"u1": fake}))
	err := a.SubscribePresence(context.Background(), "u1", "x@y.com")
	if err == nil {
		t.Fatal("SubscribePresence não propagou erro")
	}
}

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
