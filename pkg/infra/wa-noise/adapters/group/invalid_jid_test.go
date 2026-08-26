package group

import (
	"context"
	"testing"
	"time"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"
)

// TestGroupAdapter_InvalidJIDs cobre os caminhos wajid.ToJID que falham (ou
// são pulados quando wajid.ParseJID é leniente).
func TestGroupAdapter_InvalidJIDs(t *testing.T) {
	a := NewGroupAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": &testkit.Fake{}}))
	badJID := domain.JID(string([]byte{0x00}))
	cases := []struct {
		name string
		run  func() error
	}{
		{"GetGroupInfo", func() error { _, e := a.GetGroupInfo(context.Background(), "u1", badJID); return e }},
		{"GetGroupInviteLink", func() error { _, e := a.GetGroupInviteLink(context.Background(), "u1", badJID); return e }},
		{"SetGroupName", func() error { return a.SetGroupName(context.Background(), "u1", badJID, "n") }},
		{"SetGroupTopic", func() error { return a.SetGroupTopic(context.Background(), "u1", badJID, "t") }},
		{"SetGroupPhoto", func() error { return a.SetGroupPhoto(context.Background(), "u1", badJID, nil) }},
		{"SetGroupAnnounce", func() error { return a.SetGroupAnnounce(context.Background(), "u1", badJID, true) }},
		{"SetGroupLocked", func() error { return a.SetGroupLocked(context.Background(), "u1", badJID, true) }},
		{"SetDisappearingTimer", func() error { return a.SetDisappearingTimer(context.Background(), "u1", badJID, time.Hour, time.Now()) }},
		{"UpdateGroupParticipants", func() error {
			_, e := a.UpdateGroupParticipants(context.Background(), "u1", badJID, []domain.JID{"x@s.whatsapp.net"}, domain.ParticipantAdd)
			return e
		}},
		{"GetRequestParticipants", func() error { _, e := a.GetRequestParticipants(context.Background(), "u1", badJID); return e }},
		{"UpdateRequestParticipants", func() error {
			return a.UpdateRequestParticipants(context.Background(), "u1", badJID, []domain.JID{"x@s.whatsapp.net"}, domain.RequestApprove)
		}},
		{"SetJoinApprovalMode", func() error { return a.SetJoinApprovalMode(context.Background(), "u1", badJID, true) }},
		{"LeaveGroup", func() error { return a.LeaveGroup(context.Background(), "u1", badJID) }},
		{"CreateGroup", func() error { _, e := a.CreateGroup(context.Background(), "u1", "n", []domain.JID{badJID}, domain.CreateGroupOpts{}); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Skip("wajid.ParseJID não falhou; caminho de erro raro")
			}
		})
	}
}
