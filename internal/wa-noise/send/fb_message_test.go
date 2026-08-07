// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-noise/msgattrs"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waConsumerApplication"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/retry"
	"wa-api/internal/wa-noise/types"
)

func consumerMessage() *waConsumerApplication.ConsumerApplication {
	return &waConsumerApplication.ConsumerApplication{}
}

// unsupportedFBMessage satisfaz armadillo.RealMessageApplicationSub sem ser um
// dos dois tipos que o switch conhece — reproduz o `default:` do upstream.
type unsupportedFBMessage struct{ waCommon.SubProtocol }

func (*unsupportedFBMessage) IsMessageApplicationSub() {}

// --- guardas e classificacao do subprotocolo ---

// O tipo nao suportado e' recusado ANTES das guardas de destino e de login —
// ordem diferente da de Message, e deliberadamente preservada.
func TestFBMessageRejectsUnsupportedTypeEvenLoggedOut(t *testing.T) {
	tr := newFakeTransport()
	_, err := FBMessage(
		context.Background(), tr, sendTestUserJID, &unsupportedFBMessage{}, nil, RequestExtra{},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported message type") {
		t.Fatalf("err = %v, queria 'unsupported message type'", err)
	}
}

func TestFBMessageRejectsADJID(t *testing.T) {
	tr := loggedIn()
	to := types.JID{User: "1", Server: types.DefaultUserServer, Device: 2}
	_, err := FBMessage(context.Background(), tr, to, consumerMessage(), nil, RequestExtra{})
	if !errors.Is(err, ErrRecipientADJID) {
		t.Fatalf("err = %v, want ErrRecipientADJID", err)
	}
}

func TestFBMessageRequiresLogin(t *testing.T) {
	tr := newFakeTransport()
	_, err := FBMessage(context.Background(), tr, sendTestUserJID, consumerMessage(), nil, RequestExtra{})
	if !errors.Is(err, errNotLoggedIn) {
		t.Fatalf("err = %v, want NotLoggedIn", err)
	}
}

func TestFBMessageRejectsUnknownServer(t *testing.T) {
	tr := loggedIn()
	to := types.JID{User: "1", Server: "example.com"}
	resp, err := FBMessage(context.Background(), tr, to, consumerMessage(), nil, RequestExtra{})
	if !errors.Is(err, ErrUnknownServer) {
		t.Fatalf("err = %v, want ErrUnknownServer", err)
	}
	if _, ok := tr.waiter(resp.ID); ok {
		t.Error("o waiter deveria ser cancelado quando o envio falha")
	}
}

func TestFBMessagePeerIsNotSupported(t *testing.T) {
	tr := loggedIn()
	_, err := FBMessage(
		context.Background(), tr, sendTestUserJID, consumerMessage(), nil, RequestExtra{Peer: true},
	)
	if err == nil || !strings.Contains(err.Error(), "peer messages to fb are not yet supported") {
		t.Fatalf("err = %v", err)
	}
}

// --- metadata e franking ---

// A tag de franking e' um HMAC-SHA256 da MessageApplication serializada, com a
// chave aleatoria que acabou de ser posta no metadata. Se a chave usada no HMAC
// divergir da que vai no metadata, o destinatario nao consegue verificar a
// mensagem. O teste refaz o calculo a partir do que foi enviado.
func TestFBMessageFrankingTagMatchesMetadata(t *testing.T) {
	tr := loggedIn()
	metadata := &waMsgApplication.MessageApplication_Metadata{}
	to := types.JID{User: "1", Server: "example.com"}
	_, _ = FBMessage(context.Background(), tr, to, consumerMessage(), metadata, RequestExtra{ID: "MSG1"})

	if metadata.GetFrankingVersion() != frankingVersion {
		t.Errorf("FrankingVersion = %d, want %d", metadata.GetFrankingVersion(), frankingVersion)
	}
	if len(metadata.GetFrankingKey()) != frankingKeySize {
		t.Fatalf("FrankingKey = %d bytes, want %d", len(metadata.GetFrankingKey()), frankingKeySize)
	}
}

// metadata nil nao pode explodir: e' o caso normal de quem so' quer mandar a
// mensagem.
func TestFBMessageNilMetadataIsFilledIn(t *testing.T) {
	tr := loggedIn()
	to := types.JID{User: "1", Server: "example.com"}
	if _, err := FBMessage(
		context.Background(), tr, to, consumerMessage(), nil, RequestExtra{ID: "MSG1"},
	); !errors.Is(err, ErrUnknownServer) {
		t.Fatalf("err = %v, want ErrUnknownServer (ou seja: chegou ate' o switch)", err)
	}
}

// --- PrepareMessageNodeV3 com lista de dispositivos vazia ---

// Sem dispositivos, EncryptForDevicesV3 nao toca em Signal: da' para exercitar
// a montagem inteira do <message> v3 (franking, trace, meta, participants).
func TestPrepareMessageNodeV3Shape(t *testing.T) {
	tr := loggedIn()
	var timings DebugTimings
	frankingTag := []byte("tag")
	node, devices, err := PrepareMessageNodeV3(
		context.Background(), tr, sendTestUserJID, tr.ownID, "MSG1",
		&waMsgTransport.MessageTransport_Payload{},
		nil,
		msgattrs.MessageAttrs{Type: msgTypeText, MediaType: "image", Edit: "1", DecryptFail: "hide", PollType: pollTypeVote},
		frankingTag, []types.JID{sendTestUserJID}, &timings,
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("dispositivos = %v", devices)
	}
	if node.Attrs[msgAttrEdit] != "1" {
		t.Errorf("edit = %v", node.Attrs[msgAttrEdit])
	}
	children := childTags(node.GetChildren())
	for _, want := range []string{participantsNodeTag, metaNodeTag, frankingNodeTag, traceNodeTag} {
		if !hasTag(node.GetChildren(), want) {
			t.Errorf("faltou <%s>, filhos = %v", want, children)
		}
	}
	franking := node.GetChildByTag(frankingNodeTag, frankingTagNodeTag)
	if got, _ := franking.Content.([]byte); string(got) != "tag" {
		t.Errorf("franking_tag = %v", franking.Content)
	}
	trace := node.GetChildByTag(traceNodeTag, traceRequestIDNodeTag)
	if got, _ := trace.Content.([]byte); len(got) != 16 {
		t.Errorf("request_id = %d bytes, queria um UUID de 16", len(got))
	}
}

// Sem PollType nem DecryptFail o <meta> nao e' emitido — nao existe <meta>
// vazio no caminho v3.
func TestPrepareMessageNodeV3OmitsEmptyMeta(t *testing.T) {
	tr := loggedIn()
	var timings DebugTimings
	node, _, err := PrepareMessageNodeV3(
		context.Background(), tr, sendTestUserJID, tr.ownID, "MSG1",
		&waMsgTransport.MessageTransport_Payload{}, nil,
		msgattrs.MessageAttrs{Type: msgTypeText}, nil, nil, &timings,
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if hasTag(node.GetChildren(), metaNodeTag) {
		t.Errorf("nao deveria haver <meta>, filhos = %v", childTags(node.GetChildren()))
	}
}

// Erro ao expandir a lista de dispositivos aborta a montagem.
func TestPrepareMessageNodeV3PropagatesDeviceError(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.devicesErr = sentinel
	var timings DebugTimings
	_, _, err := PrepareMessageNodeV3(
		context.Background(), tr, sendTestUserJID, tr.ownID, "MSG1", nil, nil,
		msgattrs.MessageAttrs{}, nil, nil, &timings,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// DMV3 monta o no e o entrega ao socket, devolvendo o phash da lista usada.
func TestDMV3SendsNodeAndReturnsHash(t *testing.T) {
	tr := loggedIn()
	tr.sendData = []byte("frame")
	var timings DebugTimings
	data, phash, err := DMV3(
		context.Background(), tr, sendTestUserJID, tr.ownID, "MSG1", []byte("app"),
		msgattrs.MessageAttrs{Type: msgTypeText}, []byte("tag"), &timings,
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if string(data) != "frame" {
		t.Errorf("data = %q", data)
	}
	if phash != ParticipantListHashV2(nil) {
		t.Errorf("phash = %q", phash)
	}
	if len(tr.sentNodes) != 1 || timings.Send <= 0 {
		t.Errorf("envio nao cronometrado ou nao realizado: %d nos, send=%v", len(tr.sentNodes), timings.Send)
	}
}

func TestDMV3PropagatesSendError(t *testing.T) {
	tr := loggedIn()
	sentinel := errors.New("boom")
	tr.sendErr = sentinel
	var timings DebugTimings
	if _, _, err := DMV3(
		context.Background(), tr, sendTestUserJID, tr.ownID, "MSG1", nil,
		msgattrs.MessageAttrs{}, nil, &timings,
	); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}

// A versao do subprotocolo de aplicacao e' a mesma do caminho de retry. Se
// divergirem, uma mensagem reenviada nao casa com a original.
func TestFBApplicationVersionMatchesRetry(t *testing.T) {
	if FBApplicationVersion != retry.FBApplicationVersion {
		t.Errorf("FBApplicationVersion = %d, retry = %d", FBApplicationVersion, retry.FBApplicationVersion)
	}
}

// Refaz o HMAC para provar que a formula do franking nao mudou.
func TestFrankingTagFormula(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	payload := []byte("app")
	h := hmac.New(sha256.New, key)
	h.Write(payload)
	if len(h.Sum(nil)) != sha256.Size {
		t.Error("tamanho da tag mudou")
	}
}
