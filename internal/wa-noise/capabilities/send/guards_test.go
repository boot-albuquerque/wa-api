package send

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/capabilities/group"
	"wa-api/internal/wa-noise/protocol/msgattrs"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// Os tres bugs travados aqui foram encontrados e corrigidos na Fase E lote 8.
// Os testes vieram junto na extracao (Fase F/G lote 8), reescritos contra o
// duble de Transport em vez do *Client real.

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

// --- PreparePeerMessageNode: LID vazio ---

// Bug do lote 8: GetLIDForPN devolve JID zerado *sem erro* quando nao conhece o
// mapeamento. Antes, esse zero seguia direto para EncryptForDevice, que
// montaria um Signal address de usuario vazio — endereco que nunca pode casar
// com uma sessao guardada. Agora falha explicitamente.
func TestPreparePeerMessageNodeRejectsEmptyLID(t *testing.T) {
	tr := newFakeTransport()
	tr.store = &store.Device{LIDs: stubLIDStore{lid: types.EmptyJID}}

	var timings DebugTimings
	node, err := PreparePeerMessageNode(
		context.Background(), tr, sendTestUserJID, "MSG1",
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
	tr := newFakeTransport()
	sentinel := errors.New("boom")
	tr.store = &store.Device{LIDs: stubLIDStore{err: sentinel}}

	var timings DebugTimings
	_, err := PreparePeerMessageNode(
		context.Background(), tr, sendTestUserJID, "MSG1",
		&waE2E.Message{Conversation: proto.String("oi")}, &timings,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// --- grupo sem metadados em cache ---

// Bug do lote 8: CachedGroupData devolve (nil, nil) quando o cache foi gravado
// sob outra chave (o `id` que o *servidor* devolveu difere do consultado). Os
// dois chamadores desreferenciavam esse nil.

func TestResolveGroupSendTargetNilCachedData(t *testing.T) {
	tr := newFakeTransport() // groupMeta fica nil: reproduz o (nil, nil)

	var resp Response
	var extra NodeExtraParams
	participants, err := resolveGroupSendTarget(
		context.Background(), tr, sendTestGroupJID, &types.JID{}, &RequestExtra{}, &resp, &extra,
	)
	if participants != nil {
		t.Errorf("participantes = %v, queria nil", participants)
	}
	if !errors.Is(err, group.ErrNotFound) {
		t.Fatalf("err = %v, want group.ErrNotFound", err)
	}
}

func TestSendGroupV3NilCachedData(t *testing.T) {
	tr := newFakeTransport()

	var timings DebugTimings
	_, _, err := GroupV3(
		context.Background(), tr, sendTestGroupJID, sendTestLIDJID, "MSG1",
		nil, msgattrs.MessageAttrs{}, nil, &timings,
	)
	if !errors.Is(err, group.ErrNotFound) {
		t.Fatalf("err = %v, want group.ErrNotFound", err)
	}
}

// GroupV3 so' consulta o cache quando o destino e' grupo; para qualquer outro
// server groupMeta fica nil e o mesmo deref aconteceria.
func TestSendGroupV3NonGroupDestination(t *testing.T) {
	tr := newFakeTransport()
	// Mesmo com metadados disponiveis, um destino nao-grupo nunca chega a
	// consultar o cache — e por isso cai na mesma guarda.
	tr.groupMeta = &group.Meta{Members: []types.JID{sendTestUserJID}}

	var timings DebugTimings
	_, _, err := GroupV3(
		context.Background(), tr, sendTestUserJID, sendTestLIDJID, "MSG1",
		nil, msgattrs.MessageAttrs{}, nil, &timings,
	)
	if !errors.Is(err, group.ErrNotFound) {
		t.Fatalf("err = %v, want group.ErrNotFound", err)
	}
}

// O erro de consulta ao cache e' propagado (e nao confundido com o (nil, nil)).
func TestSendGroupV3PropagatesCacheError(t *testing.T) {
	tr := newFakeTransport()
	sentinel := errors.New("boom")
	tr.groupMetaErr = sentinel

	var timings DebugTimings
	_, _, err := GroupV3(
		context.Background(), tr, sendTestGroupJID, sendTestLIDJID, "MSG1",
		nil, msgattrs.MessageAttrs{}, nil, &timings,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

func TestResolveGroupSendTargetPropagatesCacheError(t *testing.T) {
	tr := newFakeTransport()
	sentinel := errors.New("boom")
	tr.groupMetaErr = sentinel

	var resp Response
	var extra NodeExtraParams
	_, err := resolveGroupSendTarget(
		context.Background(), tr, sendTestGroupJID, &types.JID{}, &RequestExtra{}, &resp, &extra,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}
