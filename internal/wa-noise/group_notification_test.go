// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

// groupChangeNode monta o envelope <notification type="w:gp2"> com os
// atributos obrigatorios que parseGroupChange exige, mais os filhos dados.
func groupChangeNode(children ...waBinary.Node) *waBinary.Node {
	return &waBinary.Node{
		Tag: "notification",
		Attrs: waBinary.Attrs{
			"from":        groupTestJID,
			"t":           "1700000000",
			"participant": groupTestPNJID,
			"notify":      "Fulano",
		},
		Content: children,
	}
}

// --- parseGroupChange: envelope ---

func TestParseGroupChangeEnvelope(t *testing.T) {
	cli := groupTestClient()
	evt, _, err := cli.parseGroupChange(groupChangeNode())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.JID != groupTestJID {
		t.Errorf("JID = %s", evt.JID)
	}
	if evt.Sender == nil || *evt.Sender != groupTestPNJID {
		t.Errorf("Sender = %v", evt.Sender)
	}
	if evt.Notify != "Fulano" {
		t.Errorf("Notify = %q", evt.Notify)
	}
	if evt.Timestamp.Unix() != 1700000000 {
		t.Errorf("Timestamp = %v", evt.Timestamp)
	}
}

func TestParseGroupChangeMissingEnvelopeAttrs(t *testing.T) {
	cli := groupTestClient()
	_, _, err := cli.parseGroupChange(&waBinary.Node{Tag: "notification"})
	if err == nil {
		t.Fatal("esperado erro sem `from`/`t`")
	}
}

// --- parseGroupChange: listas de participantes ---

func TestParseGroupChangeParticipantLists(t *testing.T) {
	cli := groupTestClient()
	evt, _, err := cli.parseGroupChange(groupChangeNode(
		waBinary.Node{
			Tag:   string(ParticipantChangeAdd),
			Attrs: waBinary.Attrs{"reason": "invite", "v_id": "2", "prev_v_id": "1"},
			Content: []waBinary.Node{
				participantNode(groupTestPNJID, nil),
			},
		},
		waBinary.Node{
			Tag:     string(ParticipantChangePromote),
			Content: []waBinary.Node{participantNode(groupTestPN2JID, nil)},
		},
	))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(evt.Join) != 1 || evt.Join[0] != groupTestPNJID {
		t.Errorf("Join = %v", evt.Join)
	}
	if evt.JoinReason != "invite" {
		t.Errorf("JoinReason = %q", evt.JoinReason)
	}
	if len(evt.Promote) != 1 || evt.Promote[0] != groupTestPN2JID {
		t.Errorf("Promote = %v", evt.Promote)
	}
}

func TestParseGroupChangeParticipantVersionIDs(t *testing.T) {
	cli := groupTestClient()
	evt, _, err := cli.parseGroupChange(groupChangeNode(waBinary.Node{
		Tag:     string(ParticipantChangeAdd),
		Attrs:   waBinary.Attrs{"v_id": "2", "prev_v_id": "1"},
		Content: []waBinary.Node{participantNode(groupTestPNJID, nil)},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.ParticipantVersionID != "2" || evt.PrevParticipantVersionID != "1" {
		t.Errorf("version IDs = %q/%q", evt.ParticipantVersionID, evt.PrevParticipantVersionID)
	}
}

// Contrato do upstream, travado aqui de proposito: os IDs de versao de
// participante sao campos **unicos** no evento, entao com mais de um elemento
// de participante na mesma notificacao vence o ultimo — inclusive quando o
// ultimo nao traz os atributos, caso em que os IDs voltam a ser vazios. Ao
// contrario dos pares LID/PN (que sao lista e passaram a acumular no lote 6),
// nao ha como acumular um campo escalar sem decidir uma semantica nova.
func TestParseGroupChangeParticipantVersionIDsLastElementWins(t *testing.T) {
	cli := groupTestClient()
	evt, _, err := cli.parseGroupChange(groupChangeNode(
		waBinary.Node{
			Tag:     string(ParticipantChangeAdd),
			Attrs:   waBinary.Attrs{"v_id": "2", "prev_v_id": "1"},
			Content: []waBinary.Node{participantNode(groupTestPNJID, nil)},
		},
		waBinary.Node{
			Tag:     string(ParticipantChangePromote),
			Content: []waBinary.Node{participantNode(groupTestPN2JID, nil)},
		},
	))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.ParticipantVersionID != "" || evt.PrevParticipantVersionID != "" {
		t.Errorf("version IDs = %q/%q, esperado os do ultimo elemento (vazios)",
			evt.ParticipantVersionID, evt.PrevParticipantVersionID)
	}
}

// Regressao: antes do lote 6, cada elemento de participante **sobrescrevia**
// lidPairs, entao uma notificacao com <add> e <remove> juntos perdia os
// mapeamentos LID/PN do primeiro — e eles nunca chegavam ao
// PutManyLIDMappings do chamador (notification.go).
func TestParseGroupChangeAccumulatesLIDPairsAcrossElements(t *testing.T) {
	cli := groupTestClient()
	evt, lidPairs, err := cli.parseGroupChange(groupChangeNode(
		waBinary.Node{
			Tag:     string(ParticipantChangeAdd),
			Content: []waBinary.Node{participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPNJID})},
		},
		waBinary.Node{
			Tag:     string(ParticipantChangeRemove),
			Content: []waBinary.Node{participantNode(groupTestPN2JID, waBinary.Attrs{"lid": groupTestLID2})},
		},
	))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(evt.Join) != 1 || len(evt.Leave) != 1 {
		t.Fatalf("Join = %v, Leave = %v", evt.Join, evt.Leave)
	}
	if len(lidPairs) != 2 {
		t.Fatalf("lidPairs = %+v, esperado os 2 pares (add + remove)", lidPairs)
	}
	if lidPairs[0].LID != groupTestLIDJID || lidPairs[0].PN != groupTestPNJID {
		t.Errorf("par do <add> = %+v", lidPairs[0])
	}
	if lidPairs[1].LID != groupTestLID2 || lidPairs[1].PN != groupTestPN2JID {
		t.Errorf("par do <remove> = %+v", lidPairs[1])
	}
}

// --- parseGroupChange: mudancas de configuracao ---

func TestParseGroupChangeLockedAndAnnounce(t *testing.T) {
	cli := groupTestClient()
	cases := []struct {
		tag        string
		attrs      waBinary.Attrs
		checkEvent func(*testing.T, *events.GroupInfo)
	}{
		{groupLockedTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if e.Locked == nil || !e.Locked.IsLocked {
				t.Errorf("Locked = %+v", e.Locked)
			}
		}},
		{groupUnlockedTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if e.Locked == nil || e.Locked.IsLocked {
				t.Errorf("Locked = %+v", e.Locked)
			}
		}},
		{groupAnnouncementTag, waBinary.Attrs{"v_id": "3"}, func(t *testing.T, e *events.GroupInfo) {
			if e.Announce == nil || !e.Announce.IsAnnounce || e.Announce.AnnounceVersionID != "3" {
				t.Errorf("Announce = %+v", e.Announce)
			}
		}},
		{groupNotAnnouncementTag, waBinary.Attrs{"v_id": "4"}, func(t *testing.T, e *events.GroupInfo) {
			if e.Announce == nil || e.Announce.IsAnnounce {
				t.Errorf("Announce = %+v", e.Announce)
			}
		}},
		{groupEphemeralTag, waBinary.Attrs{"expiration": "604800"}, func(t *testing.T, e *events.GroupInfo) {
			if e.Ephemeral == nil || !e.Ephemeral.IsEphemeral || e.Ephemeral.DisappearingTimer != 604800 {
				t.Errorf("Ephemeral = %+v", e.Ephemeral)
			}
		}},
		{groupNotEphemeralTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if e.Ephemeral == nil || e.Ephemeral.IsEphemeral {
				t.Errorf("Ephemeral = %+v", e.Ephemeral)
			}
		}},
		{groupMembershipApprovalModeTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if e.MembershipApprovalMode == nil || !e.MembershipApprovalMode.IsJoinApprovalRequired {
				t.Errorf("MembershipApprovalMode = %+v", e.MembershipApprovalMode)
			}
		}},
		{groupSuspendedTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if !e.Suspended {
				t.Error("Suspended = false")
			}
		}},
		{groupUnsuspendedTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if !e.Unsuspended {
				t.Error("Unsuspended = false")
			}
		}},
		{groupDeleteTag, waBinary.Attrs{"reason": "spam"}, func(t *testing.T, e *events.GroupInfo) {
			if e.Delete == nil || !e.Delete.Deleted || e.Delete.DeleteReason != "spam" {
				t.Errorf("Delete = %+v", e.Delete)
			}
		}},
		{groupInviteTag, waBinary.Attrs{"code": "XYZ"}, func(t *testing.T, e *events.GroupInfo) {
			if e.NewInviteLink == nil || *e.NewInviteLink != InviteLinkPrefix+"XYZ" {
				t.Errorf("NewInviteLink = %v", e.NewInviteLink)
			}
		}},
		{groupSubjectTag, waBinary.Attrs{"subject": "Novo nome", "s_t": "1700000005", "s_o": groupTestPNJID}, func(t *testing.T, e *events.GroupInfo) {
			if e.Name == nil || e.Name.Name != "Novo nome" || e.Name.NameSetAt.Unix() != 1700000005 {
				t.Errorf("Name = %+v", e.Name)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			evt, _, err := cli.parseGroupChange(groupChangeNode(waBinary.Node{Tag: tc.tag, Attrs: tc.attrs}))
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			tc.checkEvent(t, evt)
		})
	}
}

func TestParseGroupChangeTopicSetAndDeleted(t *testing.T) {
	cli := groupTestClient()

	evt, _, err := cli.parseGroupChange(groupChangeNode(waBinary.Node{
		Tag:   groupDescriptionTag,
		Attrs: waBinary.Attrs{"id": "TOPIC2"},
		Content: []waBinary.Node{{
			Tag:     groupDescriptionBodyTag,
			Content: []byte("novo assunto"),
		}},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.Topic == nil || evt.Topic.Topic != "novo assunto" || evt.Topic.TopicID != "TOPIC2" {
		t.Fatalf("Topic = %+v", evt.Topic)
	}
	if evt.Topic.TopicDeleted {
		t.Error("TopicDeleted = true para um set")
	}
	if evt.Topic.TopicSetBy != groupTestPNJID {
		t.Errorf("TopicSetBy = %s", evt.Topic.TopicSetBy)
	}
	if !evt.Topic.TopicSetAt.Equal(evt.Timestamp) {
		t.Errorf("TopicSetAt = %v, esperado o timestamp da notificacao", evt.Topic.TopicSetAt)
	}

	evt, _, err = cli.parseGroupChange(groupChangeNode(waBinary.Node{
		Tag:     groupDescriptionTag,
		Attrs:   waBinary.Attrs{"id": "TOPIC3"},
		Content: []waBinary.Node{{Tag: groupDeleteTag}},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.Topic == nil || !evt.Topic.TopicDeleted || evt.Topic.Topic != "" {
		t.Errorf("Topic apagado = %+v", evt.Topic)
	}
}

func TestParseGroupChangeTopicWithBadBodyFails(t *testing.T) {
	cli := groupTestClient()
	_, _, err := cli.parseGroupChange(groupChangeNode(waBinary.Node{
		Tag:   groupDescriptionTag,
		Attrs: waBinary.Attrs{"id": "TOPIC4"},
		Content: []waBinary.Node{{
			Tag:     groupDescriptionBodyTag,
			Content: []waBinary.Node{{Tag: "inesperado"}},
		}},
	}))
	if err == nil {
		t.Fatal("esperado erro com <body> de conteudo nao-binario")
	}
}

func TestParseGroupChangeLinkAndUnlink(t *testing.T) {
	cli := groupTestClient()
	linked := waBinary.Node{
		Tag:   groupNodeTag,
		Attrs: waBinary.Attrs{"jid": groupTestJID, "subject": "Subgrupo"},
	}

	evt, _, err := cli.parseGroupChange(groupChangeNode(waBinary.Node{
		Tag:     groupLinkTag,
		Attrs:   waBinary.Attrs{"link_type": string(types.GroupLinkChangeTypeSub)},
		Content: []waBinary.Node{linked},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.Link == nil || evt.Link.Type != types.GroupLinkChangeTypeSub || evt.Link.Group.JID != groupTestJID {
		t.Errorf("Link = %+v", evt.Link)
	}

	evt, _, err = cli.parseGroupChange(groupChangeNode(waBinary.Node{
		Tag: groupUnlinkTag,
		Attrs: waBinary.Attrs{
			"unlink_type":   string(types.GroupLinkChangeTypeSub),
			"unlink_reason": "delete",
		},
		Content: []waBinary.Node{linked},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.Unlink == nil || evt.Unlink.UnlinkReason != types.GroupUnlinkReason("delete") {
		t.Errorf("Unlink = %+v", evt.Unlink)
	}
}

func TestParseGroupChangeLinkWithoutGroupNode(t *testing.T) {
	cli := groupTestClient()
	for _, tag := range []string{groupLinkTag, groupUnlinkTag} {
		_, _, err := cli.parseGroupChange(groupChangeNode(waBinary.Node{
			Tag:   tag,
			Attrs: waBinary.Attrs{"link_type": "sub", "unlink_type": "sub", "unlink_reason": "delete"},
		}))
		var missing *ElementMissingError
		if !errors.As(err, &missing) || missing.Tag != groupNodeTag {
			t.Errorf("%s: erro = %v, esperado ElementMissingError de <group>", tag, err)
		}
	}
}

func TestParseGroupChangeUnknownChildIsCollected(t *testing.T) {
	cli := groupTestClient()
	evt, _, err := cli.parseGroupChange(groupChangeNode(waBinary.Node{Tag: "tag_do_futuro"}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(evt.UnknownChanges) != 1 || evt.UnknownChanges[0].Tag != "tag_do_futuro" {
		t.Errorf("UnknownChanges = %+v", evt.UnknownChanges)
	}
}

func TestParseGroupChangeChildMissingAttrsFails(t *testing.T) {
	cli := groupTestClient()
	// <delete> exige `reason`.
	_, _, err := cli.parseGroupChange(groupChangeNode(waBinary.Node{Tag: groupDeleteTag}))
	if err == nil {
		t.Fatal("esperado erro por atributo obrigatorio ausente no filho")
	}
}

// --- updateGroupParticipantCache ---

func TestUpdateGroupParticipantCacheAddsAndRemoves(t *testing.T) {
	cli := groupTestClient()
	cli.groupCache[groupTestJID] = &groupMetaCache{
		Members: []types.JID{groupTestPNJID, groupTestPN2JID},
	}
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID:   groupTestJID,
		Join:  []types.JID{groupTestLIDJID, groupTestPNJID}, // o segundo ja' esta la'
		Leave: []types.JID{groupTestPN2JID},
	})
	members := cli.groupCache[groupTestJID].Members
	if len(members) != 2 {
		t.Fatalf("membros = %v, esperado 2", members)
	}
	has := func(jid types.JID) bool {
		for _, m := range members {
			if m == jid {
				return true
			}
		}
		return false
	}
	if !has(groupTestPNJID) || !has(groupTestLIDJID) || has(groupTestPN2JID) {
		t.Errorf("membros = %v", members)
	}
}

func TestUpdateGroupParticipantCacheNoopWithoutJoinOrLeave(t *testing.T) {
	cli := groupTestClient()
	cli.groupCache[groupTestJID] = &groupMetaCache{Members: []types.JID{groupTestPNJID}}
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID:     groupTestJID,
		Promote: []types.JID{groupTestPN2JID},
	})
	if len(cli.groupCache[groupTestJID].Members) != 1 {
		t.Errorf("membros = %v, promote nao deve mexer no cache", cli.groupCache[groupTestJID].Members)
	}
}

func TestUpdateGroupParticipantCacheIgnoresUncachedGroup(t *testing.T) {
	cli := groupTestClient()
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID:  groupTestJID,
		Join: []types.JID{groupTestPNJID},
	})
	if _, ok := cli.groupCache[groupTestJID]; ok {
		t.Error("grupo nao cacheado nao deve ser criado pela notificacao")
	}
}

// Remover um membro que nao esta no cache nao pode estourar indice nem
// corromper a lista.
func TestUpdateGroupParticipantCacheLeaveOfUnknownMember(t *testing.T) {
	cli := groupTestClient()
	cli.groupCache[groupTestJID] = &groupMetaCache{Members: []types.JID{groupTestPNJID}}
	cli.updateGroupParticipantCache(&events.GroupInfo{
		JID:   groupTestJID,
		Leave: []types.JID{groupTestPN2JID},
	})
	if len(cli.groupCache[groupTestJID].Members) != 1 {
		t.Errorf("membros = %v", cli.groupCache[groupTestJID].Members)
	}
}

// --- parseGroupCreate / parseGroupNotification ---

func createNotificationNode() *waBinary.Node {
	return &waBinary.Node{
		Tag: "notification",
		Attrs: waBinary.Attrs{
			"participant": groupTestPNJID,
			"notify":      "Fulano",
		},
		Content: []waBinary.Node{{
			Tag:   groupCreateTag,
			Attrs: waBinary.Attrs{"reason": "create", "key": "KEY1", "type": "new"},
			Content: []waBinary.Node{{
				Tag:   groupNodeTag,
				Attrs: waBinary.Attrs{"id": groupTestJID.User, "creation": "1699999999"},
				Content: []waBinary.Node{
					participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPNJID, "display_name": "5511XXXX999"}),
				},
			}},
		}},
	}
}

func TestParseGroupNotificationRoutesCreate(t *testing.T) {
	cli := groupTestClient()
	evt, lidPairs, redacted, err := cli.parseGroupNotification(createNotificationNode())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	joined, ok := evt.(*events.JoinedGroup)
	if !ok {
		t.Fatalf("evento = %T, esperado *events.JoinedGroup", evt)
	}
	if joined.CreateKey != "KEY1" || joined.Reason != "create" || joined.Type != "new" {
		t.Errorf("evento = %+v", joined)
	}
	if joined.Sender == nil || *joined.Sender != groupTestPNJID {
		t.Errorf("Sender = %v", joined.Sender)
	}
	if joined.GroupInfo.JID != groupTestJID {
		t.Errorf("GroupInfo.JID = %s", joined.GroupInfo.JID)
	}
	if len(lidPairs) != 1 || lidPairs[0].LID != groupTestLIDJID {
		t.Errorf("lidPairs = %+v", lidPairs)
	}
	if len(redacted) != 1 {
		t.Errorf("redacted = %+v", redacted)
	}
	if _, cached := cli.groupCache[groupTestJID]; !cached {
		t.Error("parseGroupCreate deveria ter cacheado o grupo")
	}
}

func TestParseGroupCreateWithoutGroupNode(t *testing.T) {
	cli := groupTestClient()
	_, _, _, err := cli.parseGroupNotification(&waBinary.Node{
		Tag:     "notification",
		Content: []waBinary.Node{{Tag: groupCreateTag}},
	})
	if err == nil {
		t.Fatal("esperado erro sem <group> dentro de <create>")
	}
}

func TestParseGroupCreateWithUnparseableGroupNode(t *testing.T) {
	cli := groupTestClient()
	_, _, _, err := cli.parseGroupNotification(&waBinary.Node{
		Tag: "notification",
		Content: []waBinary.Node{{
			Tag:     groupCreateTag,
			Content: []waBinary.Node{{Tag: groupNodeTag}}, // sem `id`/`creation`
		}},
	})
	if err == nil {
		t.Fatal("esperado erro de parse do <group>")
	}
}

// Um <create> acompanhado de outro filho **nao** e' tratado como criacao: cai
// no ramo de mudanca de grupo, que exige o envelope completo.
func TestParseGroupNotificationRoutesChangeWhenNotSoleCreate(t *testing.T) {
	cli := groupTestClient()
	node := createNotificationNode()
	node.Attrs["from"] = groupTestJID
	node.Attrs["t"] = "1700000000"
	node.Content = append(node.Content.([]waBinary.Node), waBinary.Node{Tag: groupSuspendedTag})

	evt, _, redacted, err := cli.parseGroupNotification(node)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	change, ok := evt.(*events.GroupInfo)
	if !ok {
		t.Fatalf("evento = %T, esperado *events.GroupInfo", evt)
	}
	if !change.Suspended {
		t.Error("Suspended = false")
	}
	if len(change.UnknownChanges) != 1 || change.UnknownChanges[0].Tag != groupCreateTag {
		t.Errorf("UnknownChanges = %+v", change.UnknownChanges)
	}
	if redacted != nil {
		t.Errorf("redactedPhones = %+v, o ramo de mudanca nao devolve nenhum", redacted)
	}
}

func TestParseGroupNotificationChangeUpdatesCache(t *testing.T) {
	cli := groupTestClient()
	cli.groupCache[groupTestJID] = &groupMetaCache{Members: []types.JID{groupTestPNJID}}
	_, _, _, err := cli.parseGroupNotification(groupChangeNode(waBinary.Node{
		Tag:     string(ParticipantChangeAdd),
		Content: []waBinary.Node{participantNode(groupTestPN2JID, nil)},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(cli.groupCache[groupTestJID].Members) != 2 {
		t.Errorf("membros = %v", cli.groupCache[groupTestJID].Members)
	}
}

func TestParseGroupNotificationPropagatesChangeError(t *testing.T) {
	cli := groupTestClient()
	_, _, _, err := cli.parseGroupNotification(&waBinary.Node{Tag: "notification"})
	if err == nil {
		t.Fatal("esperado erro de envelope")
	}
}
