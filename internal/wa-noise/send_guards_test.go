// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/msgattrs"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

// stubLIDStore devolve exatamente o que lhe mandarem para GetLIDForPN. Os
// demais metodos da interface nao sao exercitados pelos testes deste arquivo.
type stubLIDStore struct {
	lid types.JID
	err error
}

func (s stubLIDStore) PutManyLIDMappings(context.Context, []store.LIDMapping) error { return nil }
func (s stubLIDStore) PutLIDMapping(context.Context, types.JID, types.JID) error    { return nil }
func (s stubLIDStore) GetPNForLID(context.Context, types.JID) (types.JID, error) {
	return types.EmptyJID, nil
}
func (s stubLIDStore) GetLIDForPN(context.Context, types.JID) (types.JID, error) {
	return s.lid, s.err
}
func (s stubLIDStore) GetManyLIDsForPNs(context.Context, []types.JID) (map[types.JID]types.JID, error) {
	return nil, nil
}

// --- preparePeerMessageNode: LID vazio ---

// Bug do lote 8: GetLIDForPN devolve JID zerado *sem erro* quando nao conhece o
// mapeamento. Antes, esse zero seguia direto para encryptMessageForDevice, que
// montaria um Signal address de usuario vazio — endereco que nunca pode casar
// com uma sessao guardada. Agora falha explicitamente.
func TestPreparePeerMessageNodeRejectsEmptyLID(t *testing.T) {
	cli := sendTestClient()
	cli.Store = &store.Device{LIDs: stubLIDStore{lid: types.EmptyJID}}

	var timings MessageDebugTimings
	node, err := cli.preparePeerMessageNode(
		context.Background(), sendTestUserJID, "MSG1",
		&waE2E.Message{Conversation: proto.String("oi")}, &timings,
	)
	if node != nil {
		t.Errorf("no = %+v, queria nil", node)
	}
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("err = %v, want ErrNoSession", err)
	}
}

func TestPreparePeerMessageNodePropagatesLIDError(t *testing.T) {
	cli := sendTestClient()
	sentinel := errors.New("boom")
	cli.Store = &store.Device{LIDs: stubLIDStore{err: sentinel}}

	var timings MessageDebugTimings
	_, err := cli.preparePeerMessageNode(
		context.Background(), sendTestUserJID, "MSG1",
		&waE2E.Message{Conversation: proto.String("oi")}, &timings,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// --- grupo sem metadados em cache ---

// Bug do lote 8: getCachedGroupData devolve (nil, nil) quando o cache foi
// gravado sob outra chave (o `id` que o *servidor* devolveu difere do
// consultado). Os dois chamadores desreferenciavam esse nil.

func TestResolveGroupSendTargetNilCachedData(t *testing.T) {
	cli := sendTestClient()
	putGroupCache(cli, sendTestGroupJID, nil) // reproduz o (nil, nil)

	var resp SendResponse
	var extra nodeExtraParams
	participants, err := cli.resolveGroupSendTarget(
		context.Background(), sendTestGroupJID, &types.JID{}, &SendRequestExtra{}, &resp, &extra,
	)
	if participants != nil {
		t.Errorf("participantes = %v, queria nil", participants)
	}
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("err = %v, want ErrGroupNotFound", err)
	}
}

func TestSendGroupV3NilCachedData(t *testing.T) {
	cli := sendTestClient()
	putGroupCache(cli, sendTestGroupJID, nil)

	var timings MessageDebugTimings
	_, _, err := cli.sendGroupV3(
		context.Background(), sendTestGroupJID, sendTestLIDJID, "MSG1",
		nil, msgattrs.MessageAttrs{}, nil, &timings,
	)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("err = %v, want ErrGroupNotFound", err)
	}
}

// sendGroupV3 so' consulta o cache quando o destino e' grupo; para qualquer
// outro server groupMeta fica nil e o mesmo deref aconteceria.
func TestSendGroupV3NonGroupDestination(t *testing.T) {
	cli := sendTestClient()

	var timings MessageDebugTimings
	_, _, err := cli.sendGroupV3(
		context.Background(), sendTestUserJID, sendTestLIDJID, "MSG1",
		nil, msgattrs.MessageAttrs{}, nil, &timings,
	)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("err = %v, want ErrGroupNotFound", err)
	}
}
