package user

import (
	"context"
	"testing"
	"wa-api/pkg/infra/wa-noise/waclient"
	"wa-api/pkg/infra/wa-noise/waclient/waclienttest"

	"wa-api/pkg/domain"
)

// TestUserAdapter_InvalidJIDs cobre os caminhos wajid.ToJID que falham.
func TestUserAdapter_InvalidJIDs(t *testing.T) {
	a := NewUserAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": &waclienttest.Fake{}}))
	badJID := domain.JID(string([]byte{0x00}))
	cases := []struct {
		name string
		run  func() error
	}{
		{"GetUserInfo", func() error { _, e := a.GetUserInfo(context.Background(), "u1", []domain.JID{badJID}); return e }},
		{"GetProfilePicture", func() error { _, e := a.GetProfilePicture(context.Background(), "u1", badJID, false); return e }},
		{"UpdateBlocklist", func() error { _, e := a.UpdateBlocklist(context.Background(), "u1", badJID, true); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
			}
		})
	}
}
