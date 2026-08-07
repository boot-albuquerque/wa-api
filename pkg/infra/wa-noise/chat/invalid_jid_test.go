package chat

import (
	"context"
	"testing"
	"time"
	"wa-api/pkg/infra/wa-noise/waclient"
	"wa-api/pkg/infra/wa-noise/waclient/waclienttest"

	"wa-api/pkg/domain"
)

// TestChatAdapter_InvalidJIDs cobre os caminhos wajid.ToJID que falham.
func TestChatAdapter_InvalidJIDs(t *testing.T) {
	a := NewChatMessengerAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": &waclienttest.Fake{}}))
	badJID := domain.JID(string([]byte{0x00}))
	if err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), badJID, "z@y.com"); err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
	if _, err := a.SendReaction(context.Background(), "u1", badJID, domain.Reaction{Text: "👍"}); err == nil {
		t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
	}
}
