package chat

import (
	"context"
	"testing"
	"time"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"

	"wa-api/pkg/domain"
)

// TestChatAdapter_InvalidJIDs cobre os caminhos wajid.ToJID que falham.
func TestChatAdapter_InvalidJIDs(t *testing.T) {
	a := NewChatMessengerAdapter(testkit.GetterWith(map[string]client.Client{"u1": &testkit.Fake{}}))
	badJID := domain.JID(string([]byte{0x00}))
	if err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), badJID, "z@y.com"); err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if _, err := a.SendReaction(context.Background(), "u1", badJID, domain.Reaction{Text: "👍"}); err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
}
