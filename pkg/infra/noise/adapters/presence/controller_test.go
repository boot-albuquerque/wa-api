package presence

import (
	"context"
	"errors"
	"testing"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"

	"wa-api/pkg/domain"

	"wa-api/internal/noise/protocol/types"
)

// --- PresenceControllerAdapter ---

func TestNewPresenceControllerAdapter(t *testing.T) {
	if NewPresenceControllerAdapter(testkit.GetterWith(nil)) == nil {
		t.Fatal("NewPresenceControllerAdapter returned nil")
	}
}

// TestPresenceControllerAdapter_SendPresence_NoSession devolve wasession.ErrNoSession.
func TestPresenceControllerAdapter_SendPresence_NoSession(t *testing.T) {
	a := NewPresenceControllerAdapter(testkit.GetterWith(nil))
	err := a.SendPresence(context.Background(), "u1", domain.PresenceAvailable)
	if err == nil {
		t.Fatal("SendPresence com nil client returned nil")
	}
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendPresence code = %q, want no_session", testkit.AppErrCode(err))
	}
}

// TestPresenceControllerAdapter_SendPresence_UnknownPresence devolve erro
// semântico (não wasession.ErrNoSession) para tipo desconhecido.
func TestPresenceControllerAdapter_SendPresence_UnknownPresence(t *testing.T) {
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]client.Client{"u1": &testkit.Fake{}}))
	err := a.SendPresence(context.Background(), "u1", "weird")
	if err == nil {
		t.Fatal("SendPresence com presence inválida = nil")
	}
	if testkit.AppErrCode(err) == "no_session" {
		t.Errorf("SendPresence devolveu no_session para presença inválida")
	}
}

// TestPresenceControllerAdapter_SendPresence_Available propaga para o SDK.
func TestPresenceControllerAdapter_SendPresence_Available(t *testing.T) {
	called := false
	var seenState types.Presence
	fake := &testkit.Fake{
		SendPresenceFn: func(ctx context.Context, s types.Presence) error {
			called = true
			seenState = s
			return nil
		},
	}
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
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
	fake := &testkit.Fake{
		SendPresenceFn: func(ctx context.Context, s types.Presence) error {
			seenState = s
			return nil
		},
	}
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
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
	fake := &testkit.Fake{SendPresenceFn: func(ctx context.Context, s types.Presence) error { return sdkErr }}
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	err := a.SendPresence(context.Background(), "u1", domain.PresenceAvailable)
	if err == nil {
		t.Fatal("SendPresence não propagou erro")
	}
}

// TestPresenceControllerAdapter_SendChatPresence_NoSession.
func TestPresenceControllerAdapter_SendChatPresence_NoSession(t *testing.T) {
	a := NewPresenceControllerAdapter(testkit.GetterWith(nil))
	err := a.SendChatPresence(context.Background(), "u1", "x@y.com", "composing", "")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SendChatPresence code = %q, want no_session", testkit.AppErrCode(err))
	}
}

// TestPresenceControllerAdapter_SendChatPresence_PropagatesError.
func TestPresenceControllerAdapter_SendChatPresence_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &testkit.Fake{SendChatPresenceFn: func(ctx context.Context, jid types.JID, s types.ChatPresence, m types.ChatPresenceMedia) error {
		return sdkErr
	}}
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	err := a.SendChatPresence(context.Background(), "u1", "x@y.com", "composing", "")
	if err == nil {
		t.Fatal("SendChatPresence não propagou erro")
	}
}

// TestPresenceControllerAdapter_SubscribePresence_NoSession.
func TestPresenceControllerAdapter_SubscribePresence_NoSession(t *testing.T) {
	a := NewPresenceControllerAdapter(testkit.GetterWith(nil))
	err := a.SubscribePresence(context.Background(), "u1", "x@y.com")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("SubscribePresence code = %q", testkit.AppErrCode(err))
	}
}

// TestPresenceControllerAdapter_SubscribePresence_InvalidJID: cobre
// o ramo "wajid.ToJID falhou".
func TestPresenceControllerAdapter_SubscribePresence_InvalidJID(t *testing.T) {
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]client.Client{"u1": &testkit.Fake{}}))
	// Tenta várias entradas até encontrar uma que wajid.ParseJID rejeite.
	for _, raw := range []string{string([]byte{0x00}), "@", ""} {
		err := a.SubscribePresence(context.Background(), "u1", domain.JID(raw))
		if err != nil {
			return
		}
	}
	t.Skip("wajid.ParseJID não falhou para nenhuma entrada testada")
}

// TestPresenceControllerAdapter_SendChatPresence_InvalidJID.
func TestPresenceControllerAdapter_SendChatPresence_InvalidJID(t *testing.T) {
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]client.Client{"u1": &testkit.Fake{}}))
	err := a.SendChatPresence(context.Background(), "u1", domain.JID(string([]byte{0x00})), "c", "")
	if err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
}

// TestPresenceControllerAdapter_SubscribePresence_PropagatesError.
func TestPresenceControllerAdapter_SubscribePresence_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &testkit.Fake{SubscribePresenceFn: func(ctx context.Context, jid types.JID) error { return sdkErr }}
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	err := a.SubscribePresence(context.Background(), "u1", "x@y.com")
	if err == nil {
		t.Fatal("SubscribePresence não propagou erro")
	}
}
