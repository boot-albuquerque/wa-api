package group

import (
	"context"
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// sentContent devolve os filhos do no' de conteudo do i-esimo <iq> enviado.
func sentContent(t *testing.T, tr *fakeTransport, i int) waBinary.Node {
	t.Helper()
	nodes, ok := tr.sent[i].Content.([]waBinary.Node)
	if !ok || len(nodes) != 1 {
		t.Fatalf("Content do iq %d = %+v", i, tr.sent[i].Content)
	}
	return nodes[0]
}

func hasChild(node waBinary.Node, tag string) bool {
	_, ok := node.GetOptionalChildByTag(tag)
	return ok
}

// --- Create ---

func TestCreateSimpleGroup(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	info, err := Create(context.Background(), tr, ReqCreate{
		Name:         "Meu grupo",
		Participants: []types.JID{groupTestPNJID},
		CreateKey:    "3EB0ABC",
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if info.JID != groupTestJID {
		t.Errorf("JID = %s", info.JID)
	}
	node := sentContent(t, tr, 0)
	if node.Attrs["subject"] != "Meu grupo" {
		t.Errorf("subject = %v", node.Attrs["subject"])
	}
	// O prefixo estatico de ID web e' removido da chave.
	if node.Attrs["key"] != "ABC" {
		t.Errorf("key = %v, esperado sem o prefixo web", node.Attrs["key"])
	}
	parts := node.GetChildrenByTag(participantTag)
	if len(parts) != 1 || parts[0].Attrs["jid"] != groupTestPNJID {
		t.Errorf("participantes = %+v", parts)
	}
}

// Sem CreateKey, o dominio gera uma com GenerateMessageID.
func TestCreateGeneratesCreateKey(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.msgID = "3EB0GERADA"
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	if _, err := Create(context.Background(), tr, ReqCreate{Name: "X"}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	created := sentContent(t, tr, 0)
	if got := created.Attrs["key"]; got != "GERADA" {
		t.Errorf("key = %v", got)
	}
}

// O privacy token de cada participante, quando existe, vira um <privacy> filho.
func TestCreateAttachesPrivacyToken(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.token = &store.PrivacyToken{Token: []byte("TOK")}
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	if _, err := Create(context.Background(), tr, ReqCreate{
		Name: "X", Participants: []types.JID{groupTestPNJID}, CreateKey: "K",
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	created := sentContent(t, tr, 0)
	part := created.GetChildrenByTag(participantTag)[0]
	privacy, ok := part.GetOptionalChildByTag("privacy")
	if !ok {
		t.Fatalf("participante sem <privacy>: %s", part.XMLString())
	}
	if string(privacy.Content.([]byte)) != "TOK" {
		t.Errorf("token = %v", privacy.Content)
	}
}

func TestCreateFailsOnPrivacyTokenError(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.tokenErr = errors.New("db off")
	_, err := Create(context.Background(), tr, ReqCreate{
		Name: "X", Participants: []types.JID{groupTestPNJID},
	})
	if err == nil {
		t.Fatal("esperado erro de privacy token")
	}
	if len(tr.sent) != 0 {
		t.Error("nao deveria ter enviado nada")
	}
}

func TestCreateCommunityDefaultsApprovalMode(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	if _, err := Create(context.Background(), tr, ReqCreate{
		Name:        "Comunidade",
		CreateKey:   "K",
		GroupParent: types.GroupParent{IsParent: true},
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	created := sentContent(t, tr, 0)
	parent, ok := created.GetOptionalChildByTag(parentTag)
	if !ok {
		t.Fatal("faltou <parent>")
	}
	if parent.Attrs["default_membership_approval_mode"] != defaultMembershipApprovalMode {
		t.Errorf("<parent> = %+v", parent.Attrs)
	}
}

func TestCreateCommunityKeepsExplicitApprovalMode(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	if _, err := Create(context.Background(), tr, ReqCreate{
		Name:      "Comunidade",
		CreateKey: "K",
		GroupParent: types.GroupParent{
			IsParent:                      true,
			DefaultMembershipApprovalMode: "outro",
		},
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	created := sentContent(t, tr, 0)
	parent, _ := created.GetOptionalChildByTag(parentTag)
	if parent.Attrs["default_membership_approval_mode"] != "outro" {
		t.Errorf("<parent> = %+v", parent.Attrs)
	}
}

// LinkedParentJID so' e' considerado quando IsParent e' falso — sao ramos
// exclusivos do mesmo if/else.
func TestCreateLinkedParentIgnoredWhenParent(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}, {Content: []waBinary.Node{groupNodeWith()}}}

	if _, err := Create(context.Background(), tr, ReqCreate{
		Name: "X", CreateKey: "K",
		GroupLinkedParent: types.GroupLinkedParent{LinkedParentJID: groupTestJID},
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	created := sentContent(t, tr, 0)
	linked, ok := created.GetOptionalChildByTag(linkedParentTag)
	if !ok || linked.Attrs["jid"] != groupTestJID {
		t.Errorf("<linked_parent> = %+v (ok=%v)", linked.Attrs, ok)
	}

	if _, err := Create(context.Background(), tr, ReqCreate{
		Name: "X", CreateKey: "K",
		GroupParent:       types.GroupParent{IsParent: true},
		GroupLinkedParent: types.GroupLinkedParent{LinkedParentJID: groupTestJID},
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	second := sentContent(t, tr, 1)
	if hasChild(second, linkedParentTag) {
		t.Error("<linked_parent> nao deveria aparecer junto com <parent>")
	}
}

func TestCreateOptionalFlags(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{groupNodeWith()}}}
	if _, err := Create(context.Background(), tr, ReqCreate{
		Name: "X", CreateKey: "K",
		GroupLocked:                 types.GroupLocked{IsLocked: true},
		GroupAnnounce:               types.GroupAnnounce{IsAnnounce: true},
		GroupEphemeral:              types.GroupEphemeral{IsEphemeral: true, DisappearingTimer: 86400},
		GroupMembershipApprovalMode: types.GroupMembershipApprovalMode{IsJoinApprovalRequired: true},
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := sentContent(t, tr, 0)
	for _, tag := range []string{lockedTag, announcementTag, ephemeralTag, membershipApprovalModeTag} {
		if !hasChild(node, tag) {
			t.Errorf("faltou <%s>: %s", tag, node.XMLString())
		}
	}
	eph, _ := node.GetOptionalChildByTag(ephemeralTag)
	if eph.Attrs["expiration"] != uint32(86400) || eph.Attrs["trigger"] != "1" {
		t.Errorf("<ephemeral> = %+v", eph.Attrs)
	}
	approval, _ := node.GetOptionalChildByTag(membershipApprovalModeTag)
	join := approval.GetChildByTag(joinTag)
	if join.Attrs["state"] != joinStateOn {
		t.Errorf("<group_join> = %+v", join.Attrs)
	}
}

func TestCreatePropagatesIQErrorAndMissingGroupNode(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	sentinel := errors.New("406")
	tr.err = []error{sentinel}
	if _, err := Create(context.Background(), tr, ReqCreate{Name: "X", CreateKey: "K"}); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	tr2.withStores()
	tr2.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := Create(context.Background(), tr2, ReqCreate{Name: "X", CreateKey: "K"})
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != nodeTag {
		t.Fatalf("err = %v", err)
	}
}

// --- Link / Unlink / Leave ---

func TestLinkBuildsNestedNode(t *testing.T) {
	tr := newFakeTransport()
	if err := Link(context.Background(), tr, groupTestJID, groupTestJID); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	links := sentContent(t, tr, 0)

	if links.Tag != "links" {
		t.Fatalf("tag = %s", links.Tag)
	}
	link := links.GetChildByTag(linkTag)
	if link.Attrs["link_type"] != string(types.GroupLinkChangeTypeSub) {
		t.Errorf("<link> = %+v", link.Attrs)
	}
	if link.GetChildByTag(nodeTag).Attrs["jid"] != groupTestJID {
		t.Errorf("<group> = %s", link.XMLString())
	}
	if tr.sent[0].To != groupTestJID || tr.sent[0].Type != IQSet {
		t.Errorf("iq = %+v", tr.sent[0])
	}
}

func TestUnlinkBuildsNode(t *testing.T) {
	tr := newFakeTransport()
	if err := Unlink(context.Background(), tr, groupTestJID, groupTestJID); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := sentContent(t, tr, 0)
	if node.Tag != unlinkTag || node.Attrs["unlink_type"] != string(types.GroupLinkChangeTypeSub) {
		t.Errorf("no = %s", node.XMLString())
	}
	if node.GetChildByTag(nodeTag).Attrs["jid"] != groupTestJID {
		t.Errorf("no = %s", node.XMLString())
	}
}

func TestLeaveBuildsNodeAndPropagatesError(t *testing.T) {
	tr := newFakeTransport()
	if err := Leave(context.Background(), tr, groupTestJID); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	node := sentContent(t, tr, 0)
	if node.Tag != "leave" || node.GetChildByTag(nodeTag).Attrs["id"] != groupTestJID {
		t.Errorf("no = %s", node.XMLString())
	}
	// leave vai sempre para o servidor de grupos, nao para o grupo.
	if tr.sent[0].To != types.GroupServerJID {
		t.Errorf("To = %s", tr.sent[0].To)
	}

	tr2 := newFakeTransport()
	sentinel := errors.New("nope")
	tr2.err = []error{sentinel}
	if err := Leave(context.Background(), tr2, groupTestJID); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
	if err := Link(context.Background(), tr2, groupTestJID, groupTestJID); err != nil {
		t.Fatalf("erro inesperado no segundo envio: %v", err)
	}
}
