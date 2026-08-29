package prekeys

import (
	"errors"
	"strings"
	"testing"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

func testJID(t *testing.T, user string, device uint16) types.JID {
	t.Helper()
	jid := types.NewJID(user, types.DefaultUserServer)
	jid.Device = device
	return jid
}

// listNode monta a resposta de <iq> de busca de prekeys com um <user> por JID.
func listNode(children ...waBinary.Node) *waBinary.Node {
	return &waBinary.Node{Tag: "iq", Content: []waBinary.Node{
		{Tag: "list", Content: children},
	}}
}

func userNode(t *testing.T, jid types.JID, withPreKey bool) waBinary.Node {
	t.Helper()
	node := preKeyBundleNode(t, 555, withPreKey)
	node.Attrs = waBinary.Attrs{"jid": jid}
	return node
}

func TestFetch(t *testing.T) {
	tr := newFakeTransport(t)
	jid := testJID(t, "111", 2)
	tr.enqueueIQ(listNode(userNode(t, jid, true)), nil)

	resp, err := Fetch(t.Context(), tr, []types.JID{jid})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got, ok := resp[jid]
	if !ok {
		t.Fatalf("resposta = %+v, sem entrada para %s", resp, jid)
	}
	if got.Err != nil {
		t.Fatalf("Err = %v", got.Err)
	}
	if got.Bundle.DeviceID() != uint32(jid.Device) {
		t.Errorf("DeviceID = %d, esperado %d", got.Bundle.DeviceID(), jid.Device)
	}

	call := tr.calls()[0]
	if call.Namespace != "encrypt" || call.Type != IQGet {
		t.Errorf("IQ = %+v, esperado get/encrypt", call)
	}
	key, ok := call.Content.([]waBinary.Node)
	if !ok || len(key) != 1 || key[0].Tag != "key" {
		t.Fatalf("conteudo = %+v, esperado um unico <key>", call.Content)
	}
	users, ok := key[0].Content.([]waBinary.Node)
	if !ok || len(users) != 1 || users[0].Attrs["reason"] != "identity" {
		t.Errorf("<key> = %+v, esperado um <user reason=\"identity\">", key[0].Content)
	}
}

// Filhos que nao sao <user> sao ignorados; um <user> malformado entra no mapa
// com Err preenchido, sem derrubar a resposta inteira.
func TestFetchIgnoresNonUserChildrenAndKeepsPerUserErrors(t *testing.T) {
	tr := newFakeTransport(t)
	ok := testJID(t, "111", 1)
	bad := testJID(t, "222", 1)
	tr.enqueueIQ(listNode(
		waBinary.Node{Tag: "ruido"},
		userNode(t, ok, true),
		waBinary.Node{Tag: "user", Attrs: waBinary.Attrs{"jid": bad}},
	), nil)

	resp, err := Fetch(t.Context(), tr, []types.JID{ok, bad})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("%d entradas, esperado 2 (o <ruido> nao entra)", len(resp))
	}
	if resp[ok].Err != nil {
		t.Errorf("Err do JID valido = %v", resp[ok].Err)
	}
	if resp[bad].Err == nil {
		t.Error("JID malformado deveria ter Err preenchido")
	}
}

func TestFetchErrors(t *testing.T) {
	jid := testJID(t, "111", 1)

	t.Run("erro de transporte", func(t *testing.T) {
		tr := newFakeTransport(t)
		sentinel := errors.New("socket fechado")
		tr.enqueueIQ(nil, sentinel)

		_, err := Fetch(t.Context(), tr, []types.JID{jid})
		if !errors.Is(err, sentinel) {
			t.Fatalf("erro = %v, esperado embrulhar %v", err, sentinel)
		}
		if !strings.Contains(err.Error(), "failed to send prekey request") {
			t.Errorf("erro = %q, sem o contexto da consulta", err)
		}
	})

	t.Run("resposta vazia", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.enqueueIQ(&waBinary.Node{Tag: "iq"}, nil)

		_, err := Fetch(t.Context(), tr, []types.JID{jid})
		if err == nil || !strings.Contains(err.Error(), "empty response") {
			t.Fatalf("erro = %v, esperado reclamar de resposta vazia", err)
		}
	})
}

func TestFetchNoError(t *testing.T) {
	tr := newFakeTransport(t)
	ok := testJID(t, "111", 1)
	bad := testJID(t, "222", 1)
	tr.enqueueIQ(listNode(
		userNode(t, ok, true),
		waBinary.Node{Tag: "user", Attrs: waBinary.Attrs{"jid": bad}},
	), nil)

	bundles := FetchNoError(t.Context(), tr, []types.JID{ok, bad})

	if len(bundles) != 1 {
		t.Fatalf("%d bundles, esperado 1 (o malformado e' descartado)", len(bundles))
	}
	if bundles[ok] == nil {
		t.Errorf("bundle de %s ausente", ok)
	}
}

// Uma falha da requisicao inteira faz FetchNoError devolver nil, sem propagar
// erro: o chamador (envio de mensagem) segue sem os bundles.
func TestFetchNoErrorSwallowsRequestFailure(t *testing.T) {
	tr := newFakeTransport(t)
	tr.enqueueIQ(nil, errors.New("socket fechado"))

	if got := FetchNoError(t.Context(), tr, []types.JID{testJID(t, "111", 1)}); got != nil {
		t.Errorf("= %v, esperado nil", got)
	}
}
