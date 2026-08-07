// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"context"
	"errors"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// --- ApplyAck ---

func TestApplySendAckCopiesServerAttrs(t *testing.T) {
	tr := newFakeTransport()
	var resp Response
	err := ApplyAck(tr, ackNode(waBinary.Attrs{
		ackAttrServerID: "42",
		ackAttrTime:     "1700000000",
	}), sendTestUserJID, "", &resp)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if resp.ServerID != 42 {
		t.Errorf("ServerID = %d, want 42", resp.ServerID)
	}
	if resp.Timestamp.Unix() != 1700000000 {
		t.Errorf("Timestamp = %v", resp.Timestamp)
	}
}

func TestApplySendAckServerError(t *testing.T) {
	tr := newFakeTransport()
	var resp Response
	err := ApplyAck(tr, ackNode(waBinary.Attrs{ackAttrError: "403"}), sendTestUserJID, "", &resp)
	if !errors.Is(err, ErrServerReturnedError) {
		t.Fatalf("err = %v, want ErrServerReturnedError", err)
	}
}

// Peculiaridade preservada da Fase A: quando o servidor devolve error != 0, o
// erro e' guardado mas a checagem de phash (e a invalidacao de cache) ainda
// roda antes do retorno — nao ha return antecipado.
func TestApplySendAckErrorStillInvalidatesCache(t *testing.T) {
	tr := newFakeTransport()
	tr.groupCache[sendTestGroupJID] = true
	var resp Response
	err := ApplyAck(tr, ackNode(waBinary.Attrs{
		ackAttrError: "500",
		ackAttrPHash: "2:outro",
	}), sendTestGroupJID, "2:nosso", &resp)
	if !errors.Is(err, ErrServerReturnedError) {
		t.Fatalf("err = %v, want ErrServerReturnedError", err)
	}
	if tr.groupCache[sendTestGroupJID] {
		t.Error("groupCache deveria ter sido invalidado apesar do erro")
	}
}

func TestApplySendAckMatchingPHashKeepsCache(t *testing.T) {
	tr := newFakeTransport()
	tr.groupCache[sendTestGroupJID] = true
	var resp Response
	if err := ApplyAck(
		tr, ackNode(waBinary.Attrs{ackAttrPHash: "2:igual"}), sendTestGroupJID, "2:igual", &resp,
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !tr.groupCache[sendTestGroupJID] {
		t.Error("phash igual nao deve invalidar o cache")
	}
}

// phash ausente no ack e' o caso comum de DM: nao deve invalidar nada, mesmo
// que o phash local seja nao vazio.
func TestApplySendAckAbsentPHashKeepsCache(t *testing.T) {
	tr := newFakeTransport()
	tr.deviceCache[sendTestUserJID] = true
	var resp Response
	if err := ApplyAck(tr, ackNode(waBinary.Attrs{}), sendTestUserJID, "2:nosso", &resp); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !tr.deviceCache[sendTestUserJID] {
		t.Error("ack sem phash nao deve invalidar o cache")
	}
}

// --- InvalidateParticipantCache ---

func TestInvalidateParticipantCacheByServer(t *testing.T) {
	cases := map[string]struct {
		jid           types.JID
		wantGroupGone bool
		wantUserGone  bool
	}{
		"grupo":     {sendTestGroupJID, true, false},
		"pn":        {sendTestUserJID, false, true},
		"lid":       {sendTestLIDJID, false, true},
		"bot":       {types.JID{User: "9", Server: types.BotServer}, false, true},
		"hosted":    {types.JID{User: "9", Server: types.HostedServer}, false, true},
		"broadcast": {types.JID{User: "status", Server: types.BroadcastServer}, false, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.groupCache[tc.jid] = true
			tr.deviceCache[tc.jid] = true
			InvalidateParticipantCache(tr, tc.jid)
			if tr.groupCache[tc.jid] == tc.wantGroupGone {
				t.Errorf("groupCache presente=%v, queria removido=%v", tr.groupCache[tc.jid], tc.wantGroupGone)
			}
			if tr.deviceCache[tc.jid] == tc.wantUserGone {
				t.Errorf("deviceCache presente=%v, queria removido=%v", tr.deviceCache[tc.jid], tc.wantUserGone)
			}
		})
	}
}

// --- AwaitAck ---

func TestAwaitSendAckReturnsNode(t *testing.T) {
	tr := newFakeTransport()
	req := &RequestExtra{ID: "MSG1", Timeout: time.Second}
	respChan := tr.WaitResponse(req.ID)
	want := ackNode(waBinary.Attrs{ackAttrTime: "1700000000"})
	respChan <- want

	var resp Response
	got, err := AwaitAck(context.Background(), tr, req, &resp, respChan, nil, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != want {
		t.Errorf("no devolvido = %v", got)
	}
}

func TestAwaitSendAckTimeout(t *testing.T) {
	tr := newFakeTransport()
	req := &RequestExtra{ID: "MSG2", Timeout: time.Millisecond}
	respChan := tr.WaitResponse(req.ID)

	var resp Response
	_, err := AwaitAck(context.Background(), tr, req, &resp, respChan, nil, time.Now())
	if !errors.Is(err, ErrMessageTimedOut) {
		t.Fatalf("err = %v, want ErrMessageTimedOut", err)
	}
	if _, ok := tr.waiter(req.ID); ok {
		t.Error("o waiter deve ser cancelado no timeout")
	}
}

func TestAwaitSendAckContextCancel(t *testing.T) {
	tr := newFakeTransport()
	req := &RequestExtra{ID: "MSG3", Timeout: time.Minute}
	respChan := tr.WaitResponse(req.ID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var resp Response
	_, err := AwaitAck(ctx, tr, req, &resp, respChan, nil, time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, ok := tr.waiter(req.ID); ok {
		t.Error("o waiter deve ser cancelado quando o ctx morre")
	}
}

// Timeout <= 0 desliga o relogio (documentado em RequestExtra.Timeout): o
// select deve bloquear ate' o ack chegar, nunca disparar ErrMessageTimedOut.
func TestAwaitSendAckNonPositiveTimeoutNeverExpires(t *testing.T) {
	tr := newFakeTransport()
	req := &RequestExtra{ID: "MSG4", Timeout: -1}
	respChan := tr.WaitResponse(req.ID)
	want := ackNode(waBinary.Attrs{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		respChan <- want
	}()

	var resp Response
	got, err := AwaitAck(context.Background(), tr, req, &resp, respChan, nil, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != want {
		t.Error("no devolvido diferente do enviado")
	}
	if resp.DebugTimings.Resp <= 0 {
		t.Error("DebugTimings.Resp deveria ter sido preenchido")
	}
}

// Quando o "ack" e' na verdade um no de desconexao, AwaitAck reenvia o frame e
// devolve a resposta do reenvio, cronometrando o retry.
func TestAwaitSendAckRetriesOnDisconnect(t *testing.T) {
	tr := newFakeTransport()
	tr.disconnect = true
	tr.retryNode = ackNode(waBinary.Attrs{ackAttrTime: "1700000001"})
	req := &RequestExtra{ID: "MSG5", Timeout: time.Second}
	respChan := tr.WaitResponse(req.ID)
	respChan <- ackNode(waBinary.Attrs{})

	var resp Response
	got, err := AwaitAck(context.Background(), tr, req, &resp, respChan, nil, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != tr.retryNode {
		t.Error("deveria devolver o no do reenvio")
	}
	if tr.retryCalls != 1 {
		t.Errorf("RetryFrame chamado %d vezes, queria 1", tr.retryCalls)
	}
}

func TestAwaitSendAckPropagatesRetryError(t *testing.T) {
	tr := newFakeTransport()
	tr.disconnect = true
	sentinel := errors.New("boom")
	tr.retryErr = sentinel
	req := &RequestExtra{ID: "MSG6", Timeout: time.Second}
	respChan := tr.WaitResponse(req.ID)
	respChan <- ackNode(waBinary.Attrs{})

	var resp Response
	if _, err := AwaitAck(context.Background(), tr, req, &resp, respChan, nil, time.Now()); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
}
