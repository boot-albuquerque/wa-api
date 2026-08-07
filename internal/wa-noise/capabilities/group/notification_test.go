package group

import (
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// groupChangeNode monta o envelope <notification type="w:gp2"> com os
// atributos obrigatorios que ParseChange exige, mais os filhos dados.
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

// --- ParseChange: envelope ---

func TestParseGroupChangeEnvelope(t *testing.T) {
	tr := newFakeTransport()
	evt, _, err := ParseChange(tr, groupChangeNode())
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
	tr := newFakeTransport()
	_, _, err := ParseChange(tr, &waBinary.Node{Tag: "notification"})
	if err == nil {
		t.Fatal("esperado erro sem `from`/`t`")
	}
}

// --- ParseChange: listas de participantes ---

func TestParseGroupChangeParticipantLists(t *testing.T) {
	tr := newFakeTransport()
	evt, _, err := ParseChange(tr, groupChangeNode(
		waBinary.Node{
			Tag:   string(ChangeAdd),
			Attrs: waBinary.Attrs{"reason": "invite", "v_id": "2", "prev_v_id": "1"},
			Content: []waBinary.Node{
				participantNode(groupTestPNJID, nil),
			},
		},
		waBinary.Node{
			Tag:     string(ChangePromote),
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
	tr := newFakeTransport()
	evt, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{
		Tag:     string(ChangeAdd),
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
	tr := newFakeTransport()
	evt, _, err := ParseChange(tr, groupChangeNode(
		waBinary.Node{
			Tag:     string(ChangeAdd),
			Attrs:   waBinary.Attrs{"v_id": "2", "prev_v_id": "1"},
			Content: []waBinary.Node{participantNode(groupTestPNJID, nil)},
		},
		waBinary.Node{
			Tag:     string(ChangePromote),
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
	tr := newFakeTransport()
	evt, lidPairs, err := ParseChange(tr, groupChangeNode(
		waBinary.Node{
			Tag:     string(ChangeAdd),
			Content: []waBinary.Node{participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPNJID})},
		},
		waBinary.Node{
			Tag:     string(ChangeRemove),
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

// --- ParseChange: mudancas de configuracao ---

func TestParseGroupChangeLockedAndAnnounce(t *testing.T) {
	tr := newFakeTransport()
	cases := []struct {
		tag        string
		attrs      waBinary.Attrs
		checkEvent func(*testing.T, *events.GroupInfo)
	}{
		{lockedTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if e.Locked == nil || !e.Locked.IsLocked {
				t.Errorf("Locked = %+v", e.Locked)
			}
		}},
		{unlockedTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if e.Locked == nil || e.Locked.IsLocked {
				t.Errorf("Locked = %+v", e.Locked)
			}
		}},
		{announcementTag, waBinary.Attrs{"v_id": "3"}, func(t *testing.T, e *events.GroupInfo) {
			if e.Announce == nil || !e.Announce.IsAnnounce || e.Announce.AnnounceVersionID != "3" {
				t.Errorf("Announce = %+v", e.Announce)
			}
		}},
		{notAnnouncementTag, waBinary.Attrs{"v_id": "4"}, func(t *testing.T, e *events.GroupInfo) {
			if e.Announce == nil || e.Announce.IsAnnounce {
				t.Errorf("Announce = %+v", e.Announce)
			}
		}},
		{ephemeralTag, waBinary.Attrs{"expiration": "604800"}, func(t *testing.T, e *events.GroupInfo) {
			if e.Ephemeral == nil || !e.Ephemeral.IsEphemeral || e.Ephemeral.DisappearingTimer != 604800 {
				t.Errorf("Ephemeral = %+v", e.Ephemeral)
			}
		}},
		{notEphemeralTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if e.Ephemeral == nil || e.Ephemeral.IsEphemeral {
				t.Errorf("Ephemeral = %+v", e.Ephemeral)
			}
		}},
		{membershipApprovalModeTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if e.MembershipApprovalMode == nil || !e.MembershipApprovalMode.IsJoinApprovalRequired {
				t.Errorf("MembershipApprovalMode = %+v", e.MembershipApprovalMode)
			}
		}},
		{suspendedTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if !e.Suspended {
				t.Error("Suspended = false")
			}
		}},
		{unsuspendedTag, nil, func(t *testing.T, e *events.GroupInfo) {
			if !e.Unsuspended {
				t.Error("Unsuspended = false")
			}
		}},
		{deleteTag, waBinary.Attrs{"reason": "spam"}, func(t *testing.T, e *events.GroupInfo) {
			if e.Delete == nil || !e.Delete.Deleted || e.Delete.DeleteReason != "spam" {
				t.Errorf("Delete = %+v", e.Delete)
			}
		}},
		{inviteTag, waBinary.Attrs{"code": "XYZ"}, func(t *testing.T, e *events.GroupInfo) {
			if e.NewInviteLink == nil || *e.NewInviteLink != InviteLinkPrefix+"XYZ" {
				t.Errorf("NewInviteLink = %v", e.NewInviteLink)
			}
		}},
		{subjectTag, waBinary.Attrs{"subject": "Novo nome", "s_t": "1700000005", "s_o": groupTestPNJID}, func(t *testing.T, e *events.GroupInfo) {
			if e.Name == nil || e.Name.Name != "Novo nome" || e.Name.NameSetAt.Unix() != 1700000005 {
				t.Errorf("Name = %+v", e.Name)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			evt, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{Tag: tc.tag, Attrs: tc.attrs}))
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			tc.checkEvent(t, evt)
		})
	}
}

func TestParseGroupChangeTopicSetAndDeleted(t *testing.T) {
	tr := newFakeTransport()

	evt, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{
		Tag:   descriptionTag,
		Attrs: waBinary.Attrs{"id": "TOPIC2"},
		Content: []waBinary.Node{{
			Tag:     descriptionBodyTag,
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

	evt, _, err = ParseChange(tr, groupChangeNode(waBinary.Node{
		Tag:     descriptionTag,
		Attrs:   waBinary.Attrs{"id": "TOPIC3"},
		Content: []waBinary.Node{{Tag: deleteTag}},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.Topic == nil || !evt.Topic.TopicDeleted || evt.Topic.Topic != "" {
		t.Errorf("Topic apagado = %+v", evt.Topic)
	}
}

func TestParseGroupChangeTopicWithBadBodyFails(t *testing.T) {
	tr := newFakeTransport()
	_, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{
		Tag:   descriptionTag,
		Attrs: waBinary.Attrs{"id": "TOPIC4"},
		Content: []waBinary.Node{{
			Tag:     descriptionBodyTag,
			Content: []waBinary.Node{{Tag: "inesperado"}},
		}},
	}))
	if err == nil {
		t.Fatal("esperado erro com <body> de conteudo nao-binario")
	}
}

func TestParseGroupChangeLinkAndUnlink(t *testing.T) {
	tr := newFakeTransport()
	linked := waBinary.Node{
		Tag:   nodeTag,
		Attrs: waBinary.Attrs{"jid": groupTestJID, "subject": "Subgrupo"},
	}

	evt, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{
		Tag:     linkTag,
		Attrs:   waBinary.Attrs{"link_type": string(types.GroupLinkChangeTypeSub)},
		Content: []waBinary.Node{linked},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if evt.Link == nil || evt.Link.Type != types.GroupLinkChangeTypeSub || evt.Link.Group.JID != groupTestJID {
		t.Errorf("Link = %+v", evt.Link)
	}

	evt, _, err = ParseChange(tr, groupChangeNode(waBinary.Node{
		Tag: unlinkTag,
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
	tr := newFakeTransport()
	for _, tag := range []string{linkTag, unlinkTag} {
		_, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{
			Tag:   tag,
			Attrs: waBinary.Attrs{"link_type": "sub", "unlink_type": "sub", "unlink_reason": "delete"},
		}))
		// testElementMissing e' o duble de *wa-noise.ElementMissingError; o
		// contrato de que a raiz entrega o tipo historico esta' em
		// group_transport.go.
		var missing *testElementMissing
		if !errors.As(err, &missing) || missing.Tag != nodeTag {
			t.Errorf("%s: erro = %v, esperado ElementMissingError de <group>", tag, err)
		}
	}
}

func TestParseGroupChangeUnknownChildIsCollected(t *testing.T) {
	tr := newFakeTransport()
	evt, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{Tag: "tag_do_futuro"}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(evt.UnknownChanges) != 1 || evt.UnknownChanges[0].Tag != "tag_do_futuro" {
		t.Errorf("UnknownChanges = %+v", evt.UnknownChanges)
	}
}

func TestParseGroupChangeChildMissingAttrsFails(t *testing.T) {
	tr := newFakeTransport()
	// <delete> exige `reason`.
	_, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{Tag: deleteTag}))
	if err == nil {
		t.Fatal("esperado erro por atributo obrigatorio ausente no filho")
	}
}

// --- UpdateParticipantCache ---

func TestUpdateGroupParticipantCacheAddsAndRemoves(t *testing.T) {
	tr := newFakeTransport()
	tr.putCached(groupTestJID, &Meta{
		Members: []types.JID{groupTestPNJID, groupTestPN2JID},
	})
	UpdateParticipantCache(tr, &events.GroupInfo{
		JID:   groupTestJID,
		Join:  []types.JID{groupTestLIDJID, groupTestPNJID}, // o segundo ja' esta la'
		Leave: []types.JID{groupTestPN2JID},
	})
	members := mustCached(t, tr, groupTestJID).Members
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
	tr := newFakeTransport()
	tr.putCached(groupTestJID, &Meta{Members: []types.JID{groupTestPNJID}})
	UpdateParticipantCache(tr, &events.GroupInfo{
		JID:     groupTestJID,
		Promote: []types.JID{groupTestPN2JID},
	})
	if len(mustCached(t, tr, groupTestJID).Members) != 1 {
		t.Errorf("membros = %v, promote nao deve mexer no cache", mustCached(t, tr, groupTestJID).Members)
	}
}

func TestUpdateGroupParticipantCacheIgnoresUncachedGroup(t *testing.T) {
	tr := newFakeTransport()
	UpdateParticipantCache(tr, &events.GroupInfo{
		JID:  groupTestJID,
		Join: []types.JID{groupTestPNJID},
	})
	if _, ok := tr.cached(groupTestJID); ok {
		t.Error("grupo nao cacheado nao deve ser criado pela notificacao")
	}
}

// Remover um membro que nao esta no cache nao pode estourar indice nem
// corromper a lista.
func TestUpdateGroupParticipantCacheLeaveOfUnknownMember(t *testing.T) {
	tr := newFakeTransport()
	tr.putCached(groupTestJID, &Meta{Members: []types.JID{groupTestPNJID}})
	UpdateParticipantCache(tr, &events.GroupInfo{
		JID:   groupTestJID,
		Leave: []types.JID{groupTestPN2JID},
	})
	if len(mustCached(t, tr, groupTestJID).Members) != 1 {
		t.Errorf("membros = %v", mustCached(t, tr, groupTestJID).Members)
	}
}

// --- ParseCreate / ParseNotification ---

func createNotificationNode() *waBinary.Node {
	return &waBinary.Node{
		Tag: "notification",
		Attrs: waBinary.Attrs{
			"participant": groupTestPNJID,
			"notify":      "Fulano",
		},
		Content: []waBinary.Node{{
			Tag:   createTag,
			Attrs: waBinary.Attrs{"reason": "create", "key": "KEY1", "type": "new"},
			Content: []waBinary.Node{{
				Tag:   nodeTag,
				Attrs: waBinary.Attrs{"id": groupTestJID.User, "creation": "1699999999"},
				Content: []waBinary.Node{
					participantNode(groupTestLIDJID, waBinary.Attrs{"phone_number": groupTestPNJID, "display_name": "5511XXXX999"}),
				},
			}},
		}},
	}
}

func TestParseGroupNotificationRoutesCreate(t *testing.T) {
	tr := newFakeTransport()
	evt, lidPairs, redacted, err := ParseNotification(tr, createNotificationNode())
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
	if _, cached := tr.cached(groupTestJID); !cached {
		t.Error("ParseCreate deveria ter cacheado o grupo")
	}
}

func TestParseGroupCreateWithoutGroupNode(t *testing.T) {
	tr := newFakeTransport()
	_, _, _, err := ParseNotification(tr, &waBinary.Node{
		Tag:     "notification",
		Content: []waBinary.Node{{Tag: createTag}},
	})
	if err == nil {
		t.Fatal("esperado erro sem <group> dentro de <create>")
	}
}

func TestParseGroupCreateWithUnparseableGroupNode(t *testing.T) {
	tr := newFakeTransport()
	_, _, _, err := ParseNotification(tr, &waBinary.Node{
		Tag: "notification",
		Content: []waBinary.Node{{
			Tag:     createTag,
			Content: []waBinary.Node{{Tag: nodeTag}}, // sem `id`/`creation`
		}},
	})
	if err == nil {
		t.Fatal("esperado erro de parse do <group>")
	}
}

// Um <create> acompanhado de outro filho **nao** e' tratado como criacao: cai
// no ramo de mudanca de grupo, que exige o envelope completo.
func TestParseGroupNotificationRoutesChangeWhenNotSoleCreate(t *testing.T) {
	tr := newFakeTransport()
	node := createNotificationNode()
	node.Attrs["from"] = groupTestJID
	node.Attrs["t"] = "1700000000"
	node.Content = append(node.Content.([]waBinary.Node), waBinary.Node{Tag: suspendedTag})

	evt, _, redacted, err := ParseNotification(tr, node)
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
	if len(change.UnknownChanges) != 1 || change.UnknownChanges[0].Tag != createTag {
		t.Errorf("UnknownChanges = %+v", change.UnknownChanges)
	}
	if redacted != nil {
		t.Errorf("redactedPhones = %+v, o ramo de mudanca nao devolve nenhum", redacted)
	}
}

func TestParseGroupNotificationChangeUpdatesCache(t *testing.T) {
	tr := newFakeTransport()
	tr.putCached(groupTestJID, &Meta{Members: []types.JID{groupTestPNJID}})
	_, _, _, err := ParseNotification(tr, groupChangeNode(waBinary.Node{
		Tag:     string(ChangeAdd),
		Content: []waBinary.Node{participantNode(groupTestPN2JID, nil)},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(mustCached(t, tr, groupTestJID).Members) != 2 {
		t.Errorf("membros = %v", mustCached(t, tr, groupTestJID).Members)
	}
}

func TestParseGroupNotificationPropagatesChangeError(t *testing.T) {
	tr := newFakeTransport()
	_, _, _, err := ParseNotification(tr, &waBinary.Node{Tag: "notification"})
	if err == nil {
		t.Fatal("esperado erro de envelope")
	}
}

// <demote> na notificacao vira evt.Demote — e' o quarto ramo de mudanca de
// participante; os outros tres ja' tem teste acima.
func TestParseGroupChangeDemote(t *testing.T) {
	tr := newFakeTransport()
	evt, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{
		Tag:     string(ChangeDemote),
		Attrs:   waBinary.Attrs{"v_id": "V2", "prev_v_id": "V1"},
		Content: []waBinary.Node{participantNode(groupTestPNJID, nil)},
	}))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(evt.Demote) != 1 || evt.Demote[0] != groupTestPNJID {
		t.Errorf("Demote = %v", evt.Demote)
	}
	if evt.ParticipantVersionID != "V2" || evt.PrevParticipantVersionID != "V1" {
		t.Errorf("versoes = %q/%q", evt.ParticipantVersionID, evt.PrevParticipantVersionID)
	}
}

// <link>/<unlink> com um <group> filho que nao parseia devolve erro de parse,
// nao ElementMissingError.
func TestParseGroupChangeLinkWithUnparseableGroupNode(t *testing.T) {
	for _, tag := range []string{linkTag, unlinkTag} {
		tr := newFakeTransport()
		_, _, err := ParseChange(tr, groupChangeNode(waBinary.Node{
			Tag:   tag,
			Attrs: waBinary.Attrs{"link_type": "sub", "unlink_type": "sub", "unlink_reason": "delete"},
			// <group> sem `jid` nem `id`: ParseLinkTargetNode falha.
			Content: []waBinary.Node{{Tag: nodeTag}},
		}))
		if err == nil {
			t.Fatalf("%s: esperado erro de parse do <group>", tag)
		}
		var missing *testElementMissing
		if errors.As(err, &missing) {
			t.Errorf("%s: erro = %v, esperado erro de parse e nao de elemento ausente", tag, err)
		}
	}
}
