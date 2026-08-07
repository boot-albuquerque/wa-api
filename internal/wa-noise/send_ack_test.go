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
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/util/log"
)

var (
	sendTestGroupJID = types.JID{User: "12345", Server: types.GroupServer}
	sendTestUserJID  = types.JID{User: "5511999999999", Server: types.DefaultUserServer}
	sendTestLIDJID   = types.JID{User: "77777777", Server: types.HiddenUserServer}
)

func sendTestClient() *Client {
	return &Client{
		Log:              waLog.Noop,
		groupCache:       make(map[types.JID]*groupMetaCache),
		userDevicesCache: make(map[types.JID]deviceCache),
		responseWaiters:  make(map[string]chan<- *waBinary.Node),
	}
}

func ackNode(attrs waBinary.Attrs) *waBinary.Node {
	return &waBinary.Node{Tag: "ack", Attrs: attrs}
}

// --- applySendAck ---

func TestApplySendAckCopiesServerAttrs(t *testing.T) {
	cli := sendTestClient()
	var resp SendResponse
	err := cli.applySendAck(ackNode(waBinary.Attrs{
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
	cli := sendTestClient()
	var resp SendResponse
	err := cli.applySendAck(
		ackNode(waBinary.Attrs{ackAttrError: "403"}),
		sendTestUserJID, "", &resp,
	)
	if !errors.Is(err, ErrServerReturnedError) {
		t.Fatalf("err = %v, want ErrServerReturnedError", err)
	}
}

// Peculiaridade preservada da Fase A: quando o servidor devolve error != 0, o
// erro e' guardado mas a checagem de phash (e a invalidacao de cache) ainda
// roda antes do retorno — nao ha return antecipado.
func TestApplySendAckErrorStillInvalidatesCache(t *testing.T) {
	cli := sendTestClient()
	cli.groupCache[sendTestGroupJID] = &groupMetaCache{}
	var resp SendResponse
	err := cli.applySendAck(ackNode(waBinary.Attrs{
		ackAttrError: "500",
		ackAttrPHash: "2:outro",
	}), sendTestGroupJID, "2:nosso", &resp)
	if !errors.Is(err, ErrServerReturnedError) {
		t.Fatalf("err = %v, want ErrServerReturnedError", err)
	}
	if _, ok := cli.groupCache[sendTestGroupJID]; ok {
		t.Error("groupCache deveria ter sido invalidado apesar do erro")
	}
}

func TestApplySendAckMatchingPHashKeepsCache(t *testing.T) {
	cli := sendTestClient()
	cli.groupCache[sendTestGroupJID] = &groupMetaCache{}
	var resp SendResponse
	if err := cli.applySendAck(
		ackNode(waBinary.Attrs{ackAttrPHash: "2:igual"}),
		sendTestGroupJID, "2:igual", &resp,
	); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := cli.groupCache[sendTestGroupJID]; !ok {
		t.Error("phash igual nao deve invalidar o cache")
	}
}

// phash ausente no ack e' o caso comum de DM: nao deve invalidar nada, mesmo
// que o phash local seja nao vazio.
func TestApplySendAckAbsentPHashKeepsCache(t *testing.T) {
	cli := sendTestClient()
	cli.userDevicesCache[sendTestUserJID] = deviceCache{}
	var resp SendResponse
	if err := cli.applySendAck(ackNode(waBinary.Attrs{}), sendTestUserJID, "2:nosso", &resp); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := cli.userDevicesCache[sendTestUserJID]; !ok {
		t.Error("ack sem phash nao deve invalidar o cache")
	}
}

// --- invalidateParticipantCache ---

func TestInvalidateParticipantCacheByServer(t *testing.T) {
	cases := map[string]struct {
		jid           types.JID
		wantGroupGone bool
		wantUserGone  bool
	}{
		"grupo":     {sendTestGroupJID, true, false},
		"pn":        {sendTestUserJID, false, true},
		"lid":       {sendTestLIDJID, false, true},
		"broadcast": {types.JID{User: "status", Server: types.BroadcastServer}, false, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cli := sendTestClient()
			cli.groupCache[tc.jid] = &groupMetaCache{}
			cli.userDevicesCache[tc.jid] = deviceCache{}
			cli.invalidateParticipantCache(tc.jid)
			if _, ok := cli.groupCache[tc.jid]; ok == tc.wantGroupGone {
				t.Errorf("groupCache presente=%v, queria removido=%v", ok, tc.wantGroupGone)
			}
			if _, ok := cli.userDevicesCache[tc.jid]; ok == tc.wantUserGone {
				t.Errorf("userDevicesCache presente=%v, queria removido=%v", ok, tc.wantUserGone)
			}
		})
	}
}

// --- awaitSendAck ---

func TestAwaitSendAckReturnsNode(t *testing.T) {
	cli := sendTestClient()
	req := &SendRequestExtra{ID: "MSG1", Timeout: time.Second}
	respChan := cli.waitResponse(req.ID)
	want := ackNode(waBinary.Attrs{ackAttrTime: "1700000000"})
	respChan <- want

	var resp SendResponse
	got, err := cli.awaitSendAck(context.Background(), req, &resp, respChan, nil, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != want {
		t.Errorf("no devolvido = %v", got)
	}
}

func TestAwaitSendAckTimeout(t *testing.T) {
	cli := sendTestClient()
	req := &SendRequestExtra{ID: "MSG2", Timeout: time.Millisecond}
	respChan := cli.waitResponse(req.ID)

	var resp SendResponse
	_, err := cli.awaitSendAck(context.Background(), req, &resp, respChan, nil, time.Now())
	if !errors.Is(err, ErrMessageTimedOut) {
		t.Fatalf("err = %v, want ErrMessageTimedOut", err)
	}
	if _, ok := cli.responseWaiters[req.ID]; ok {
		t.Error("o waiter deve ser cancelado no timeout")
	}
}

func TestAwaitSendAckContextCancel(t *testing.T) {
	cli := sendTestClient()
	req := &SendRequestExtra{ID: "MSG3", Timeout: time.Minute}
	respChan := cli.waitResponse(req.ID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var resp SendResponse
	_, err := cli.awaitSendAck(ctx, req, &resp, respChan, nil, time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, ok := cli.responseWaiters[req.ID]; ok {
		t.Error("o waiter deve ser cancelado quando o ctx morre")
	}
}

// Timeout <= 0 desliga o relogio (documentado em SendRequestExtra.Timeout): o
// select deve bloquear ate' o ack chegar, nunca disparar ErrMessageTimedOut.
func TestAwaitSendAckNonPositiveTimeoutNeverExpires(t *testing.T) {
	cli := sendTestClient()
	req := &SendRequestExtra{ID: "MSG4", Timeout: -1}
	respChan := cli.waitResponse(req.ID)
	want := ackNode(waBinary.Attrs{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		respChan <- want
	}()

	var resp SendResponse
	got, err := cli.awaitSendAck(context.Background(), req, &resp, respChan, nil, time.Now())
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
