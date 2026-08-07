// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

// --- infraestrutura compartilhada dos testes do lote 6 ---

var (
	groupTestJID    = types.NewJID("55511", types.GroupServer)
	groupTestPNJID  = types.NewJID("5511999", types.DefaultUserServer)
	groupTestLIDJID = types.NewJID("8877", types.HiddenUserServer)
	groupTestPN2JID = types.NewJID("5511888", types.DefaultUserServer)
	groupTestLID2   = types.NewJID("7766", types.HiddenUserServer)
)

// groupTestClient monta o minimo de Client que o parsing de grupo precisa:
// logger no-op e o cache de grupo inicializado (cacheGroupInfo escreve nele, e
// escrita em mapa nil e' panic).
func groupTestClient() *Client {
	return &Client{
		Log:        waLog.Noop,
		groupCache: make(map[types.JID]*groupMetaCache),
	}
}

func participantNode(jid types.JID, extra waBinary.Attrs) waBinary.Node {
	attrs := waBinary.Attrs{"jid": jid}
	for k, v := range extra {
		attrs[k] = v
	}
	return waBinary.Node{Tag: groupParticipantTag, Attrs: attrs}
}

// --- parseParticipant ---

func TestParseParticipantAdminFlags(t *testing.T) {
	cases := []struct {
		pcpType    string
		admin      bool
		superAdmin bool
	}{
		{"", false, false},
		{participantTypeAdmin, true, false},
		{participantTypeSuperAdmin, true, true},
		{"member", false, false},
	}
	for _, tc := range cases {
		node := participantNode(groupTestPNJID, waBinary.Attrs{"type": tc.pcpType})
		got := parseParticipant(node.AttrGetter(), &node)
		if got.IsAdmin != tc.admin || got.IsSuperAdmin != tc.superAdmin {
			t.Errorf("type=%q: admin=%v super=%v, esperado %v/%v",
				tc.pcpType, got.IsAdmin, got.IsSuperAdmin, tc.admin, tc.superAdmin)
		}
	}
}

func TestParseParticipantPNFillsLIDFromAttr(t *testing.T) {
	node := participantNode(groupTestPNJID, waBinary.Attrs{"lid": groupTestLIDJID})
	got := parseParticipant(node.AttrGetter(), &node)
	if got.PhoneNumber != groupTestPNJID {
		t.Errorf("PhoneNumber = %s, esperado %s", got.PhoneNumber, groupTestPNJID)
	}
	if got.LID != groupTestLIDJID {
		t.Errorf("LID = %s, esperado %s", got.LID, groupTestLIDJID)
	}
}

func TestParseParticipantLIDFillsPhoneFromAttr(t *testing.T) {
	node := participantNode(groupTestLIDJID, waBinary.Attrs{
		"phone_number": groupTestPNJID,
		"display_name": "5511XXXX999",
	})
	got := parseParticipant(node.AttrGetter(), &node)
	if got.LID != groupTestLIDJID {
		t.Errorf("LID = %s, esperado %s", got.LID, groupTestLIDJID)
	}
	if got.PhoneNumber != groupTestPNJID {
		t.Errorf("PhoneNumber = %s, esperado %s", got.PhoneNumber, groupTestPNJID)
	}
	if got.DisplayName != "5511XXXX999" {
		t.Errorf("DisplayName = %q", got.DisplayName)
	}
}

func TestParseParticipantErrorWithoutAddRequest(t *testing.T) {
	node := participantNode(groupTestPNJID, waBinary.Attrs{"error": "403"})
	got := parseParticipant(node.AttrGetter(), &node)
	if got.Error != 403 {
		t.Errorf("Error = %d, esperado 403", got.Error)
	}
	if got.AddRequest != nil {
		t.Errorf("AddRequest = %+v, esperado nil sem <add_request>", got.AddRequest)
	}
}

func TestParseParticipantErrorWithAddRequest(t *testing.T) {
	node := participantNode(groupTestPNJID, waBinary.Attrs{"error": "403"})
	node.Content = []waBinary.Node{{
		Tag: groupAddRequestTag,
		Attrs: waBinary.Attrs{
			"code":       "ABC123",
			"expiration": "1700000000",
		},
	}}
	got := parseParticipant(node.AttrGetter(), &node)
	if got.AddRequest == nil {
		t.Fatal("AddRequest nil, esperado preenchido")
	}
	if got.AddRequest.Code != "ABC123" {
		t.Errorf("Code = %q", got.AddRequest.Code)
	}
	if got.AddRequest.Expiration.Unix() != 1700000000 {
		t.Errorf("Expiration = %v", got.AddRequest.Expiration)
	}
}

// Sem `error`, o <add_request> presente e' ignorado — o upstream so' o le no
// ramo de erro, e este teste trava esse contrato.
func TestParseParticipantIgnoresAddRequestWithoutError(t *testing.T) {
	node := participantNode(groupTestPNJID, nil)
	node.Content = []waBinary.Node{{
		Tag:   groupAddRequestTag,
		Attrs: waBinary.Attrs{"code": "ABC123", "expiration": "1700000000"},
	}}
	got := parseParticipant(node.AttrGetter(), &node)
	if got.AddRequest != nil {
		t.Errorf("AddRequest = %+v, esperado nil sem atributo error", got.AddRequest)
	}
}

// --- parseGroupNode ---

func fullGroupNode() *waBinary.Node {
	return &waBinary.Node{
		Tag: groupNodeTag,
		Attrs: waBinary.Attrs{
			"id":              groupTestJID.User,
			"creator":         groupTestPNJID,
			"subject":         "Grupo de teste",
			"s_t":             "1700000000",
			"s_o":             groupTestPNJID,
			"creation":        "1699999999",
			"size":            "2",
			"addressing_mode": "lid",
		},
		Content: []waBinary.Node{
			participantNode(groupTestPNJID, waBinary.Attrs{"type": participantTypeSuperAdmin, "lid": groupTestLIDJID}),
			participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPN2JID}),
			{
				Tag:   groupDescriptionTag,
				Attrs: waBinary.Attrs{"id": "TOPIC1", "t": "1700000001", "participant": groupTestPNJID},
				Content: []waBinary.Node{{
					Tag:     groupDescriptionBodyTag,
					Content: []byte("assunto do grupo"),
				}},
			},
			{Tag: groupAnnouncementTag},
			{Tag: groupLockedTag},
			{Tag: groupEphemeralTag, Attrs: waBinary.Attrs{"expiration": "86400"}},
			{Tag: groupMemberAddModeTag, Content: []byte(types.GroupMemberAddModeAdmin)},
			{Tag: groupDefaultSubGroupTag},
			{Tag: groupIncognitoTag},
			{Tag: groupMembershipApprovalModeTag},
			{Tag: groupSuspendedTag},
		},
	}
}

func TestParseGroupNodeFull(t *testing.T) {
	cli := groupTestClient()
	info, err := cli.parseGroupNode(fullGroupNode())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if info.JID != groupTestJID {
		t.Errorf("JID = %s, esperado %s", info.JID, groupTestJID)
	}
	if info.Name != "Grupo de teste" || info.NameSetAt.Unix() != 1700000000 {
		t.Errorf("nome = %q em %v", info.Name, info.NameSetAt)
	}
	if info.GroupCreated.Unix() != 1699999999 {
		t.Errorf("GroupCreated = %v", info.GroupCreated)
	}
	if info.ParticipantCount != 2 {
		t.Errorf("ParticipantCount = %d", info.ParticipantCount)
	}
	if info.AddressingMode != types.AddressingMode("lid") {
		t.Errorf("AddressingMode = %q", info.AddressingMode)
	}
	if len(info.Participants) != 2 {
		t.Fatalf("participantes = %d, esperado 2", len(info.Participants))
	}
	if !info.Participants[0].IsSuperAdmin || info.Participants[0].LID != groupTestLIDJID {
		t.Errorf("participante 0 = %+v", info.Participants[0])
	}
	if info.Participants[1].PhoneNumber != groupTestPN2JID {
		t.Errorf("participante 1 = %+v", info.Participants[1])
	}
	if info.Topic != "assunto do grupo" || info.TopicID != "TOPIC1" {
		t.Errorf("topico = %q/%q", info.Topic, info.TopicID)
	}
	if info.TopicSetAt.Unix() != 1700000001 || info.TopicSetBy != groupTestPNJID {
		t.Errorf("topico setado em %v por %s", info.TopicSetAt, info.TopicSetBy)
	}
	if !info.IsAnnounce || !info.IsLocked || !info.IsEphemeral ||
		!info.IsDefaultSubGroup || !info.IsIncognito || !info.IsJoinApprovalRequired || !info.Suspended {
		t.Errorf("flags booleanas: %+v", info)
	}
	if info.DisappearingTimer != 86400 {
		t.Errorf("DisappearingTimer = %d", info.DisappearingTimer)
	}
	if info.MemberAddMode != types.GroupMemberAddModeAdmin {
		t.Errorf("MemberAddMode = %q", info.MemberAddMode)
	}
}

func TestParseGroupNodeParentAndLinkedParent(t *testing.T) {
	cli := groupTestClient()
	parent, err := cli.parseGroupNode(&waBinary.Node{
		Tag:   groupNodeTag,
		Attrs: waBinary.Attrs{"id": groupTestJID.User, "creation": "1"},
		Content: []waBinary.Node{{
			Tag:   groupParentTag,
			Attrs: waBinary.Attrs{"default_membership_approval_mode": defaultMembershipApprovalMode},
		}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !parent.IsParent || parent.DefaultMembershipApprovalMode != defaultMembershipApprovalMode {
		t.Errorf("parent = %+v", parent)
	}

	child, err := cli.parseGroupNode(&waBinary.Node{
		Tag:   groupNodeTag,
		Attrs: waBinary.Attrs{"id": groupTestJID.User, "creation": "1"},
		Content: []waBinary.Node{{
			Tag:   groupLinkedParentTag,
			Attrs: waBinary.Attrs{"jid": groupTestJID},
		}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if child.LinkedParentJID != groupTestJID {
		t.Errorf("LinkedParentJID = %s", child.LinkedParentJID)
	}
}

// <description> sem <body> nao preenche nada — nem topico, nem ID, nem autor.
func TestParseGroupNodeDescriptionWithoutBody(t *testing.T) {
	cli := groupTestClient()
	info, err := cli.parseGroupNode(&waBinary.Node{
		Tag:   groupNodeTag,
		Attrs: waBinary.Attrs{"id": groupTestJID.User, "creation": "1"},
		Content: []waBinary.Node{{
			Tag:   groupDescriptionTag,
			Attrs: waBinary.Attrs{"id": "TOPIC1"},
		}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if info.Topic != "" || info.TopicID != "" {
		t.Errorf("topico preenchido sem <body>: %q/%q", info.Topic, info.TopicID)
	}
}

// <description> com <body> de conteudo nao-binario nao derruba o parse: o
// topico fica vazio (type assertion com comma-ok descartado no upstream).
func TestParseGroupNodeDescriptionNonByteBody(t *testing.T) {
	cli := groupTestClient()
	info, err := cli.parseGroupNode(&waBinary.Node{
		Tag:   groupNodeTag,
		Attrs: waBinary.Attrs{"id": groupTestJID.User, "creation": "1"},
		Content: []waBinary.Node{{
			Tag:   groupDescriptionTag,
			Attrs: waBinary.Attrs{"id": "TOPIC1", "t": "1"},
			Content: []waBinary.Node{{
				Tag:     groupDescriptionBodyTag,
				Content: []waBinary.Node{{Tag: "inesperado"}},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if info.Topic != "" {
		t.Errorf("Topic = %q, esperado vazio", info.Topic)
	}
	if info.TopicID != "TOPIC1" {
		t.Errorf("TopicID = %q", info.TopicID)
	}
}

func TestParseGroupNodeUnknownChildIsIgnored(t *testing.T) {
	cli := groupTestClient()
	info, err := cli.parseGroupNode(&waBinary.Node{
		Tag:     groupNodeTag,
		Attrs:   waBinary.Attrs{"id": groupTestJID.User, "creation": "1"},
		Content: []waBinary.Node{{Tag: "tag_do_futuro"}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if info.JID != groupTestJID {
		t.Errorf("JID = %s", info.JID)
	}
}

func TestParseGroupNodeMissingRequiredAttrs(t *testing.T) {
	cli := groupTestClient()
	info, err := cli.parseGroupNode(&waBinary.Node{Tag: groupNodeTag})
	if err == nil {
		t.Fatal("esperado erro por `id`/`creation` ausentes")
	}
	if info == nil {
		t.Fatal("parseGroupNode devolveu info nil junto com erro; chamadores logam info.JID")
	}
}

// --- parseGroupLinkTargetNode ---

func TestParseGroupLinkTargetNodePrefersJIDAttr(t *testing.T) {
	target, err := parseGroupLinkTargetNode(&waBinary.Node{
		Tag: groupNodeTag,
		Attrs: waBinary.Attrs{
			"jid":     groupTestJID,
			"id":      "outro",
			"subject": "Subgrupo",
			"s_t":     "1700000000",
		},
		Content: []waBinary.Node{{Tag: groupDefaultSubGroupTag}},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if target.JID != groupTestJID {
		t.Errorf("JID = %s, esperado o atributo jid", target.JID)
	}
	if target.Name != "Subgrupo" || target.NameSetAt.Unix() != 1700000000 {
		t.Errorf("nome = %q em %v", target.Name, target.NameSetAt)
	}
	if !target.IsDefaultSubGroup {
		t.Error("IsDefaultSubGroup = false com <default_sub_group> presente")
	}
}

func TestParseGroupLinkTargetNodeFallsBackToIDAttr(t *testing.T) {
	target, err := parseGroupLinkTargetNode(&waBinary.Node{
		Tag:   groupNodeTag,
		Attrs: waBinary.Attrs{"id": groupTestJID.User},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if target.JID != groupTestJID {
		t.Errorf("JID = %s, esperado montado a partir de `id`", target.JID)
	}
	if target.IsDefaultSubGroup {
		t.Error("IsDefaultSubGroup = true sem <default_sub_group>")
	}
}

func TestParseGroupLinkTargetNodeMissingBothIDs(t *testing.T) {
	_, err := parseGroupLinkTargetNode(&waBinary.Node{Tag: groupNodeTag})
	if err == nil {
		t.Fatal("esperado erro sem `jid` nem `id`")
	}
}

// --- parseParticipantList ---

func TestParseParticipantListCollectsBothMappingDirections(t *testing.T) {
	participants, lidPairs := parseParticipantList(&waBinary.Node{
		Tag: groupAddRequestTag,
		Content: []waBinary.Node{
			participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPNJID}),
			participantNode(groupTestPN2JID, waBinary.Attrs{"lid": groupTestLID2}),
		},
	})
	if len(participants) != 2 {
		t.Fatalf("participantes = %d, esperado 2", len(participants))
	}
	if len(lidPairs) != 2 {
		t.Fatalf("lidPairs = %d, esperado 2", len(lidPairs))
	}
	if lidPairs[0].LID != groupTestLIDJID || lidPairs[0].PN != groupTestPNJID {
		t.Errorf("par 0 = %+v", lidPairs[0])
	}
	if lidPairs[1].LID != groupTestLID2 || lidPairs[1].PN != groupTestPN2JID {
		t.Errorf("par 1 = %+v", lidPairs[1])
	}
}

func TestParseParticipantListSkipsInvalidChildren(t *testing.T) {
	participants, lidPairs := parseParticipantList(&waBinary.Node{
		Tag: groupAddRequestTag,
		Content: []waBinary.Node{
			{Tag: "outra_tag", Attrs: waBinary.Attrs{"jid": groupTestPNJID}},
			{Tag: groupParticipantTag, Attrs: waBinary.Attrs{"jid": "nao e' JID"}},
			{Tag: groupParticipantTag},
			// JID valido, mas com par vazio/de tipo errado: entra na lista de
			// participantes e nao gera mapeamento.
			participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": types.EmptyJID}),
			participantNode(groupTestPN2JID, waBinary.Attrs{"lid": "tambem nao e' JID"}),
		},
	})
	if len(participants) != 2 {
		t.Fatalf("participantes = %d (%v), esperado 2", len(participants), participants)
	}
	if len(lidPairs) != 0 {
		t.Errorf("lidPairs = %+v, esperado vazio", lidPairs)
	}
}

func TestParseParticipantListEmptyNode(t *testing.T) {
	participants, lidPairs := parseParticipantList(&waBinary.Node{Tag: groupAddRequestTag})
	if len(participants) != 0 {
		t.Errorf("participantes = %v", participants)
	}
	if lidPairs != nil {
		t.Errorf("lidPairs = %v, esperado nil", lidPairs)
	}
}

// --- cacheGroupInfo ---

// Regressao: antes do lote 6, lidPairs era alocado com `make(..., len(parts))`
// e so' preenchido por indice nos participantes que tinham LID **e** PN, o que
// deixava entradas zeradas no slice entregue a PutManyLIDMappings.
func TestCacheGroupInfoSkipsParticipantsWithoutMapping(t *testing.T) {
	cli := groupTestClient()
	info := &types.GroupInfo{
		JID: groupTestJID,
		Participants: []types.GroupParticipant{
			{JID: groupTestPNJID},
			{JID: groupTestLIDJID, LID: groupTestLIDJID, PhoneNumber: groupTestPNJID, DisplayName: "5511XXXX999"},
			{JID: groupTestPN2JID},
		},
	}
	lidPairs, redacted := cli.cacheGroupInfo(info, true)
	if len(lidPairs) != 1 {
		t.Fatalf("lidPairs = %+v, esperado exatamente 1 par", lidPairs)
	}
	if lidPairs[0].LID != groupTestLIDJID || lidPairs[0].PN != groupTestPNJID {
		t.Errorf("par = %+v", lidPairs[0])
	}
	if len(redacted) != 1 || redacted[0].JID != groupTestLIDJID {
		t.Errorf("redacted = %+v", redacted)
	}
	cached, ok := cli.groupCache[groupTestJID]
	if !ok {
		t.Fatal("grupo nao entrou no cache")
	}
	if len(cached.Members) != 3 {
		t.Errorf("membros em cache = %v", cached.Members)
	}
}

func TestCacheGroupInfoCommunityAnnouncementFlag(t *testing.T) {
	cli := groupTestClient()
	cli.cacheGroupInfo(&types.GroupInfo{
		JID:               groupTestJID,
		GroupAnnounce:     types.GroupAnnounce{IsAnnounce: true},
		GroupIsDefaultSub: types.GroupIsDefaultSub{IsDefaultSubGroup: true},
		AddressingMode:    types.AddressingModeLID,
	}, true)
	cached := cli.groupCache[groupTestJID]
	if !cached.CommunityAnnouncementGroup {
		t.Error("CommunityAnnouncementGroup = false para grupo de anuncio de comunidade")
	}
	if cached.AddressingMode != types.AddressingModeLID {
		t.Errorf("AddressingMode = %q", cached.AddressingMode)
	}
}
