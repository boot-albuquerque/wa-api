// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package group

import (
	"context"
	"errors"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// --- UpdateParticipants ---

func TestUpdateParticipantsBuildsNodeAndParsesResponse(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{
		Tag: string(ChangePromote),
		Content: []waBinary.Node{
			participantNode(groupTestPNJID, waBinary.Attrs{"type": participantTypeAdmin}),
		},
	}}}}
	got, err := UpdateParticipants(context.Background(), tr, groupTestJID,
		[]types.JID{groupTestPNJID}, ChangePromote)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 || !got[0].IsAdmin {
		t.Errorf("participantes = %+v", got)
	}
	node := sentContent(t, tr, 0)
	if node.Tag != string(ChangePromote) {
		t.Errorf("tag = %s", node.Tag)
	}
	if node.GetChildByTag(participantTag).Attrs["jid"] != groupTestPNJID {
		t.Errorf("no = %s", node.XMLString())
	}
}

// Ao ADICIONAR um LID, o telefone conhecido viaja junto em phone_number. E' o
// unico ramo em que o dominio consulta o LIDStore.
func TestUpdateParticipantsAttachesPhoneForLIDAdd(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.pnForLID = groupTestPNJID
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{Tag: string(ChangeAdd)}}}}
	if _, err := UpdateParticipants(context.Background(), tr, groupTestJID,
		[]types.JID{groupTestLIDJID}, ChangeAdd); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	sent := sentContent(t, tr, 0)
	part := sent.GetChildByTag(participantTag)
	if part.Attrs["phone_number"] != groupTestPNJID {
		t.Errorf("participante = %+v", part.Attrs)
	}
}

// LID sem telefone conhecido: nenhum atributo extra.
func TestUpdateParticipantsSkipsEmptyPhone(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.pnForLID = types.EmptyJID
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{Tag: string(ChangeAdd)}}}}
	if _, err := UpdateParticipants(context.Background(), tr, groupTestJID,
		[]types.JID{groupTestLIDJID}, ChangeAdd); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	sent := sentContent(t, tr, 0)
	part := sent.GetChildByTag(participantTag)
	if _, ok := part.Attrs["phone_number"]; ok {
		t.Errorf("participante = %+v", part.Attrs)
	}
}

// Remocao de LID nao consulta o store (so' o ramo de add faz isso).
func TestUpdateParticipantsRemoveDoesNotLookUpPhone(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.pnForLIDErr = errors.New("nao deveria ser chamado")
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{Tag: string(ChangeRemove)}}}}
	if _, err := UpdateParticipants(context.Background(), tr, groupTestJID,
		[]types.JID{groupTestLIDJID}, ChangeRemove); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestUpdateParticipantsPhoneLookupError(t *testing.T) {
	tr := newFakeTransport()
	st := tr.withStores()
	st.pnForLIDErr = errors.New("db off")
	_, err := UpdateParticipants(context.Background(), tr, groupTestJID,
		[]types.JID{groupTestLIDJID}, ChangeAdd)
	if err == nil {
		t.Fatal("esperado erro de lookup")
	}
	if len(tr.sent) != 0 {
		t.Error("nao deveria ter enviado nada")
	}
}

func TestUpdateParticipantsErrors(t *testing.T) {
	tr := newFakeTransport()
	tr.withStores()
	sentinel := errors.New("nope")
	tr.err = []error{sentinel}
	if _, err := UpdateParticipants(context.Background(), tr, groupTestJID, nil, ChangeAdd); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	tr2.withStores()
	tr2.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := UpdateParticipants(context.Background(), tr2, groupTestJID, nil, ChangeAdd)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != string(ChangeAdd) {
		t.Fatalf("err = %v", err)
	}
}

// --- GetRequestParticipants ---

func TestGetRequestParticipants(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{
		Tag: "membership_approval_requests",
		Content: []waBinary.Node{{
			Tag: "membership_approval_request",
			Attrs: waBinary.Attrs{
				"jid":          groupTestPNJID,
				"request_time": "1700000000",
			},
		}},
	}}}}
	got, err := GetRequestParticipants(context.Background(), tr, groupTestJID)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 || got[0].JID != groupTestPNJID || got[0].RequestedAt.Unix() != 1700000000 {
		t.Errorf("pedidos = %+v", got)
	}
	if tr.sent[0].Type != IQGet {
		t.Errorf("type = %q", tr.sent[0].Type)
	}
}

func TestGetRequestParticipantsErrors(t *testing.T) {
	tr := newFakeTransport()
	sentinel := errors.New("nope")
	tr.err = []error{sentinel}
	if _, err := GetRequestParticipants(context.Background(), tr, groupTestJID); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}

	tr2 := newFakeTransport()
	tr2.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetRequestParticipants(context.Background(), tr2, groupTestJID)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != "membership_approval_requests" {
		t.Fatalf("err = %v", err)
	}
}

// --- UpdateRequestParticipants ---

func TestUpdateRequestParticipants(t *testing.T) {
	tr := newFakeTransport()
	tr.resp = []*waBinary.Node{{Content: []waBinary.Node{{
		Tag: "membership_requests_action",
		Content: []waBinary.Node{{
			Tag:     string(RequestApprove),
			Content: []waBinary.Node{participantNode(groupTestPNJID, nil)},
		}},
	}}}}
	got, err := UpdateRequestParticipants(context.Background(), tr, groupTestJID,
		[]types.JID{groupTestPNJID}, RequestApprove)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != 1 || got[0].JID != groupTestPNJID {
		t.Errorf("participantes = %+v", got)
	}
	node := sentContent(t, tr, 0)
	if node.Tag != "membership_requests_action" {
		t.Fatalf("tag = %s", node.Tag)
	}
	action := node.GetChildByTag(string(RequestApprove))
	if action.GetChildByTag(participantTag).Attrs["jid"] != groupTestPNJID {
		t.Errorf("no = %s", node.XMLString())
	}
}

func TestUpdateRequestParticipantsErrors(t *testing.T) {
	tr := newFakeTransport()
	sentinel := errors.New("nope")
	tr.err = []error{sentinel}
	if _, err := UpdateRequestParticipants(context.Background(), tr, groupTestJID, nil, RequestReject); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}

	// Falta o envelope externo.
	tr2 := newFakeTransport()
	tr2.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := UpdateRequestParticipants(context.Background(), tr2, groupTestJID, nil, RequestReject)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != "membership_requests_action" {
		t.Fatalf("err = %v", err)
	}

	// Envelope presente, acao ausente.
	tr3 := newFakeTransport()
	tr3.resp = []*waBinary.Node{{Content: []waBinary.Node{{Tag: "membership_requests_action"}}}}
	_, err = UpdateRequestParticipants(context.Background(), tr3, groupTestJID, nil, RequestReject)
	if !errors.As(err, &missing) || missing.Tag != string(RequestReject) {
		t.Fatalf("err = %v", err)
	}
}
