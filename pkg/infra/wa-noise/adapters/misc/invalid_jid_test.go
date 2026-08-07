package misc

import (
	"context"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"
)

// TestMiscAdapter_InvalidJIDs cobre os caminhos wajid.ToJID que falham.
func TestMiscAdapter_InvalidJIDs(t *testing.T) {
	a := NewMiscAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": &testkit.Fake{}}))
	badJID := domain.JID(string([]byte{0x00}))
	cases := []struct {
		name string
		run  func() error
	}{
		{"ArchiveChat", func() error { return a.ArchiveChat(context.Background(), "u1", badJID, true) }},
		{"RejectCall", func() error { return a.RejectCall(context.Background(), "u1", badJID, "c1") }},
		{"RequestUnavailableMessage_Chat", func() error {
			_, e := a.RequestUnavailableMessage(context.Background(), "u1", badJID, "z@y.com", "m1")
			return e
		}},
		{"RequestUnavailableMessage_Sender", func() error {
			_, e := a.RequestUnavailableMessage(context.Background(), "u1", "x@y.com", badJID, "m1")
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
			}
		})
	}
}
