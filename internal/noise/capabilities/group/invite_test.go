package group

import (
	"context"
	"errors"
	"testing"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// --- GetInviteLink ---

func TestGetInviteLinkUsesGetOrSetByReset(t *testing.T) {
	for _, tc := range []struct {
		reset bool
		want  IQType
	}{{false, IQGet}, {true, IQSet}} {
		tr := newFakeTransport()
		tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{
			Tag:   inviteTag,
			Attrs: waBinary.Attrs{"code": "XYZ"},
		}}}}
		link, err := GetInviteLink(context.Background(), tr, groupTestJID, tc.reset)
		if err != nil {
			t.Fatalf("reset=%v: erro inesperado: %v", tc.reset, err)
		}
		if link != InviteLinkPrefix+"XYZ" {
			t.Errorf("link = %q", link)
		}
		if tr.sent[0].Type != tc.want {
			t.Errorf("reset=%v: type = %q, esperado %q", tc.reset, tr.sent[0].Type, tc.want)
		}
	}
}

func TestGetInviteLinkMapsIQErrors(t *testing.T) {
	cases := []struct {
		name string
		iq   error
		want error
	}{
		{"401", testIQErrors.NotAuthorized, ErrInviteLinkUnauthorized},
		{"404", testIQErrors.NotFound, ErrNotFound},
		{"403", testIQErrors.Forbidden, ErrNotInGroup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.err = []error{tc.iq}
			_, err := GetInviteLink(context.Background(), tr, groupTestJID, false)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, esperado %v", err, tc.want)
			}
		})
	}

	tr := newFakeTransport()
	sentinel := errors.New("500")
	tr.err = []error{sentinel}
	if _, err := GetInviteLink(context.Background(), tr, groupTestJID, false); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetInviteLinkWithoutCode(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{Tag: inviteTag}}}}
	if _, err := GetInviteLink(context.Background(), tr, groupTestJID, false); err == nil {
		t.Fatal("esperado erro sem atributo code")
	}
}

// --- GetInfoFromInvite / JoinWithInvite ---

func TestGetInfoFromInviteBuildsAddRequest(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	info, err := GetInfoFromInvite(context.Background(), tr, groupTestJID, groupTestPNJID, "CODE", 42)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if info.JID != groupTestJID {
		t.Errorf("JID = %s", info.JID)
	}
	query := sentContent(t, tr, 0)
	add := query.GetChildByTag(addRequestTag)
	if add.Attrs["code"] != "CODE" || add.Attrs["expiration"] != int64(42) || add.Attrs["admin"] != groupTestPNJID {
		t.Errorf("<add_request> = %+v", add.Attrs)
	}
}

func TestGetInfoFromInviteErrors(t *testing.T) {
	tr := newFakeTransport()
	sentinel := errors.New("nope")
	tr.err = []error{sentinel}
	if _, err := GetInfoFromInvite(context.Background(), tr, groupTestJID, groupTestPNJID, "C", 1); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	tr2.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetInfoFromInvite(context.Background(), tr2, groupTestJID, groupTestPNJID, "C", 1)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != nodeTag {
		t.Fatalf("err = %v", err)
	}
}

func TestJoinWithInvite(t *testing.T) {
	tr := newFakeTransport()
	if err := JoinWithInvite(context.Background(), tr, groupTestJID, groupTestPNJID, "CODE", 42); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := sentContent(t, tr, 0)
	if node.Tag != "accept" || node.Attrs["code"] != "CODE" || node.Attrs["admin"] != groupTestPNJID {
		t.Errorf("no = %s", node.XMLString())
	}

	tr2 := newFakeTransport()
	sentinel := errors.New("nope")
	tr2.err = []error{sentinel}
	if err := JoinWithInvite(context.Background(), tr2, groupTestJID, groupTestPNJID, "C", 1); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

// --- GetInfoFromLink ---

func TestGetInfoFromLinkTrimsPrefix(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	if _, err := GetInfoFromLink(context.Background(), tr, InviteLinkPrefix+"CODE"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got := sentContent(t, tr, 0).Attrs["code"]; got != "CODE" {
		t.Errorf("code = %v, esperado sem o prefixo", got)
	}
}

func TestGetInfoFromLinkMapsIQErrors(t *testing.T) {
	cases := []struct {
		name string
		iq   error
		want error
	}{
		{"410", testIQErrors.Gone, ErrInviteLinkRevoked},
		{"406", testIQErrors.NotAcceptable, ErrInviteLinkInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.err = []error{tc.iq}
			if _, err := GetInfoFromLink(context.Background(), tr, "C"); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, esperado %v", err, tc.want)
			}
		})
	}

	tr := newFakeTransport()
	sentinel := errors.New("500")
	tr.err = []error{sentinel}
	if _, err := GetInfoFromLink(context.Background(), tr, "C"); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	tr2.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetInfoFromLink(context.Background(), tr2, "C")
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != nodeTag {
		t.Fatalf("err = %v", err)
	}
}

// --- JoinWithLink ---

func TestJoinWithLinkReturnsGroupJID(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{
		Tag:   nodeTag,
		Attrs: waBinary.Attrs{"jid": groupTestJID},
	}}}}
	got, err := JoinWithLink(context.Background(), tr, InviteLinkPrefix+"CODE")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != groupTestJID {
		t.Errorf("jid = %s", got)
	}
	if tr.sent[0].Type != IQSet {
		t.Errorf("type = %q", tr.sent[0].Type)
	}
}

// Quando o grupo exige aprovacao, o servidor devolve
// <membership_approval_request> em vez de <group>, e e' dele que sai o JID.
func TestJoinWithLinkReturnsApprovalRequestJID(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{
		Tag:   "membership_approval_request",
		Attrs: waBinary.Attrs{"jid": groupTestJID},
	}}}}
	got, err := JoinWithLink(context.Background(), tr, "CODE")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != groupTestJID {
		t.Errorf("jid = %s", got)
	}
}

func TestJoinWithLinkErrors(t *testing.T) {
	cases := []struct {
		name string
		iq   error
		want error
	}{
		{"410", testIQErrors.Gone, ErrInviteLinkRevoked},
		{"406", testIQErrors.NotAcceptable, ErrInviteLinkInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.err = []error{tc.iq}
			got, err := JoinWithLink(context.Background(), tr, "C")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, esperado %v", err, tc.want)
			}
			if got != types.EmptyJID {
				t.Errorf("jid = %s, esperado vazio", got)
			}
		})
	}

	tr := newFakeTransport()
	sentinel := errors.New("500")
	tr.err = []error{sentinel}
	if _, err := JoinWithLink(context.Background(), tr, "C"); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	tr2.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := JoinWithLink(context.Background(), tr2, "C")
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != nodeTag {
		t.Fatalf("err = %v", err)
	}
}
