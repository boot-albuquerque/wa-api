// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package pairing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/libsignal/ecc"
	waBinary "wa-api/internal/wa-noise/binary"

	"wa-api/internal/wa-noise/security/paircrypto"
	"wa-api/internal/wa-noise/protocol/proto/waAdv"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/security/keys"
)

// ---------------------------------------------------------------- pair-device

func pairDeviceNode(refs ...waBinary.Node) *waBinary.Node {
	return &waBinary.Node{
		Tag:   "iq",
		Attrs: waBinary.Attrs{"from": types.ServerJID, "id": "abc123"},
		Content: []waBinary.Node{
			{Tag: "pair-device", Content: refs},
		},
	}
}

func TestHandleDeviceNodeEmiteQRCodes(t *testing.T) {
	tr := newFakeTransport(t)
	tr.configuredClientType = ClientChrome

	HandleDeviceNode(t.Context(), tr, pairDeviceNode(
		waBinary.Node{Tag: "ref", Content: []byte("ref-um")},
		waBinary.Node{Tag: "ref", Content: []byte("ref-dois")},
	))

	// Primeiro sai o ack do <iq>.
	nodes := tr.snapshotNodes()
	if len(nodes) != 1 || nodes[0].Tag != "iq" || nodes[0].Attrs["type"] != "result" {
		t.Fatalf("nos enviados = %+v, esperado um unico ack de <iq>", nodes)
	}
	if nodes[0].Attrs["id"] != "abc123" || nodes[0].Attrs["to"] != types.ServerJID {
		t.Errorf("ack = %+v, deveria ecoar id e from do pedido", nodes[0].Attrs)
	}

	evts := tr.snapshotEvents()
	if len(evts) != 1 {
		t.Fatalf("%d eventos, esperado 1", len(evts))
	}
	qr, ok := evts[0].(*events.QR)
	if !ok {
		t.Fatalf("evento = %T, esperado *events.QR", evts[0])
	}
	if len(qr.Codes) != 2 {
		t.Fatalf("%d codigos, esperado 2", len(qr.Codes))
	}
	for i, code := range qr.Codes {
		if !strings.HasPrefix(code, "https://wa.me/settings/linked_devices#") {
			t.Errorf("codes[%d] = %q, sem o prefixo do formato de QR", i, code)
		}
		// O ultimo campo do payload e' o tipo de cliente configurado.
		if !strings.HasSuffix(code, ","+string(ClientChrome)) {
			t.Errorf("codes[%d] = %q, deveria terminar no tipo de cliente %q", i, code, ClientChrome)
		}
	}
}

// Filhos que nao sao <ref>, ou <ref> com conteudo que nao e' []byte, sao
// logados e pulados — o evento sai com os que sobraram.
func TestHandleDeviceNodePulaFilhosInesperados(t *testing.T) {
	tr := newFakeTransport(t)

	HandleDeviceNode(t.Context(), tr, pairDeviceNode(
		waBinary.Node{Tag: "ruido", Content: []byte("x")},
		waBinary.Node{Tag: "ref", Content: "nao e bytes"},
		waBinary.Node{Tag: "ref", Content: []byte("bom")},
	))

	qr := tr.snapshotEvents()[0].(*events.QR)
	if len(qr.Codes) != 1 {
		t.Errorf("%d codigos, esperado 1 (os outros dois sao invalidos)", len(qr.Codes))
	}
}

// Uma falha ao enviar o ack e' logada, mas nao impede o evento de QR.
func TestHandleDeviceNodeAckFalhandoAindaEmiteQR(t *testing.T) {
	tr := newFakeTransport(t)
	tr.enqueueSendErr(errors.New("socket fechado"))

	HandleDeviceNode(t.Context(), tr, pairDeviceNode(
		waBinary.Node{Tag: "ref", Content: []byte("ref")},
	))

	if len(tr.snapshotEvents()) != 1 {
		t.Error("o evento de QR deveria sair mesmo com o ack falhando")
	}
}

// --------------------------------------------------------------- pair-success

// signedIdentity monta um device identity valido, assinado pela chave de
// identidade do "aparelho principal" simulado, e o container HMAC que o
// acompanha.
func signedIdentity(t *testing.T, tr *fakeTransport, hosted bool) []byte {
	t.Helper()
	advSecret := tr.Store().AdvSecretKey
	companionIdentity := tr.Store().IdentityKey
	mainKey := keys.NewKeyPair()

	// O prefixo da assinatura de conta e' escolhido a partir do DeviceType
	// DENTRO de details (e nao do AccountType do container) — sao dois campos
	// distintos, e Confirm consulta cada um no seu lugar.
	deviceType := waAdv.ADVEncryptionType_E2EE
	if hosted {
		deviceType = waAdv.ADVEncryptionType_HOSTED
	}
	details, err := proto.Marshal(&waAdv.ADVDeviceIdentity{
		RawID:      proto.Uint32(1),
		KeyIndex:   proto.Uint32(7),
		DeviceType: deviceType.Enum(),
	})
	if err != nil {
		t.Fatalf("marshal details: %v", err)
	}
	identity := &waAdv.ADVSignedDeviceIdentity{
		Details:             details,
		AccountSignatureKey: mainKey.Pub[:],
	}
	// Espelha o que VerifyAccountSignature confere: prefixo || details ||
	// chave de identidade publica DO COMPANION, assinado pela chave da conta.
	accountPrefix := paircrypto.AdvAccountSignaturePrefix
	if hosted {
		accountPrefix = paircrypto.AdvHostedAccountSignaturePrefix
	}
	accountSig := ecc.CalculateSignature(
		ecc.NewDjbECPrivateKey(*mainKey.Priv),
		paircrypto.ConcatBytes(accountPrefix, details, companionIdentity.Pub[:]),
	)
	identity.AccountSignature = accountSig[:]

	signedDetails, err := proto.Marshal(identity)
	if err != nil {
		t.Fatalf("marshal identity: %v", err)
	}

	accountType := waAdv.ADVEncryptionType_E2EE
	if hosted {
		accountType = waAdv.ADVEncryptionType_HOSTED
	}
	h := hmac.New(sha256.New, advSecret)
	if hosted {
		h.Write(paircrypto.AdvHostedAccountSignaturePrefix)
	}
	h.Write(signedDetails)

	container, err := proto.Marshal(&waAdv.ADVSignedDeviceIdentityHMAC{
		Details:     signedDetails,
		HMAC:        h.Sum(nil),
		AccountType: accountType.Enum(),
	})
	if err != nil {
		t.Fatalf("marshal container: %v", err)
	}
	return container
}

func pairJIDs(t *testing.T) (jid, lid types.JID) {
	t.Helper()
	jid = types.NewJID("5511999999999", types.DefaultUserServer)
	jid.Device = 3
	lid = types.NewJID("123456789", types.HiddenUserServer)
	lid.Device = 3
	return
}

func TestConfirmPersisteEConfirma(t *testing.T) {
	tr := newFakeTransport(t)
	jid, lid := pairJIDs(t)
	container := signedIdentity(t, tr, false)

	err := Confirm(t.Context(), tr, container, "req-1", "Loja", "android", jid, lid)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	if tr.Store().ID == nil || *tr.Store().ID != jid {
		t.Errorf("Store().ID = %v, esperado %v", tr.Store().ID, jid)
	}
	if tr.Store().LID != lid || tr.Store().BusinessName != "Loja" || tr.Store().Platform != "android" {
		t.Errorf("store nao recebeu LID/BusinessName/Platform: %+v", tr.Store())
	}
	if tr.Store().Account == nil {
		t.Error("Store().Account deveria ter sido preenchido")
	}
	if len(tr.lidMappings) != 1 || tr.lidMappings[0] != [2]types.JID{lid, jid} {
		t.Errorf("StoreLIDPNMapping = %+v, esperado uma chamada com (lid, jid)", tr.lidMappings)
	}
	if tr.expectDisconnects != 1 {
		t.Errorf("ExpectDisconnect chamado %dx, esperado 1", tr.expectDisconnects)
	}

	// A identidade do aparelho principal e' gravada com o Device zerado.
	mainLID := lid
	mainLID.Device = mainDeviceID
	if _, ok := tr.identities().put[mainLID.SignalAddress().String()]; !ok {
		t.Errorf("identidade gravada em %+v, esperado o endereco do device %d",
			tr.identities().put, mainDeviceID)
	}

	nodes := tr.snapshotNodes()
	if len(nodes) != 1 {
		t.Fatalf("%d nos enviados, esperado 1 (a confirmacao)", len(nodes))
	}
	pairSign := nodes[0].GetChildByTag("pair-device-sign")
	sign := pairSign.GetChildByTag("device-identity")
	if sign.Attrs["key-index"] != uint32(7) {
		t.Errorf("key-index = %v, esperado 7", sign.Attrs["key-index"])
	}
	// O device identity devolvido nao pode mais conter a AccountSignatureKey.
	var sent waAdv.ADVSignedDeviceIdentity
	if err := proto.Unmarshal(sign.Content.([]byte), &sent); err != nil {
		t.Fatalf("unmarshal do identity enviado: %v", err)
	}
	if sent.AccountSignatureKey != nil {
		t.Error("AccountSignatureKey deveria ter sido removida antes do envio")
	}
	if len(sent.DeviceSignature) == 0 {
		t.Error("DeviceSignature deveria ter sido preenchida")
	}
}

// Uma conta HOSTED usa o prefixo extra no HMAC e na verificacao de assinatura.
func TestConfirmAceitaContaHosted(t *testing.T) {
	tr := newFakeTransport(t)
	jid, lid := pairJIDs(t)

	err := Confirm(t.Context(), tr, signedIdentity(t, tr, true), "req", "", "", jid, lid)
	if err != nil {
		t.Fatalf("Confirm com conta hosted: %v", err)
	}
}

func TestConfirmErros(t *testing.T) {
	jid, lid := pairJIDs(t)

	// erroDoServidor confere que o <iq type="error"> saiu com o codigo e texto
	// esperados. E' o que o aparelho principal usa para abortar o pareamento.
	erroDoServidor := func(t *testing.T, tr *fakeTransport, code int, text string) {
		t.Helper()
		nodes := tr.snapshotNodes()
		if len(nodes) == 0 {
			t.Fatal("nenhum no de erro foi enviado ao servidor")
		}
		errNode := nodes[len(nodes)-1].GetChildByTag("error")
		if errNode.Attrs["code"] != code || errNode.Attrs["text"] != text {
			t.Errorf("erro enviado = %+v, esperado code=%d text=%q", errNode.Attrs, code, text)
		}
	}

	t.Run("container ilegivel", func(t *testing.T) {
		tr := newFakeTransport(t)
		err := Confirm(t.Context(), tr, []byte{0xFF, 0xFF, 0xFF}, "req", "", "", jid, lid)
		var protoErr *ProtoError
		if !errors.As(err, &protoErr) {
			t.Fatalf("erro = %v (%T), esperado *ProtoError", err, err)
		}
		erroDoServidor(t, tr, errCodeInternal, errTextInternal)
	})

	t.Run("HMAC invalido", func(t *testing.T) {
		tr := newFakeTransport(t)
		// Assinado com outro adv secret.
		container := signedIdentity(t, tr, false)
		// Troca o adv secret depois de assinar: o HMAC deixa de bater.
		tr.Store().AdvSecretKey = []byte("outro-adv-secret-de-32-bytes!!!!")
		err := Confirm(t.Context(), tr, container, "req", "", "", jid, lid)
		if !errors.Is(err, ErrInvalidDeviceIdentityHMAC) {
			t.Fatalf("erro = %v, esperado ErrInvalidDeviceIdentityHMAC", err)
		}
		erroDoServidor(t, tr, errCodeUnauthorized, errTextHMACMismatch)
	})

	t.Run("assinatura de conta invalida", func(t *testing.T) {
		tr := newFakeTransport(t)
		// Identity com assinatura de conta zerada, mas HMAC valido.
		details, _ := proto.Marshal(&waAdv.ADVDeviceIdentity{KeyIndex: proto.Uint32(1)})
		identity := &waAdv.ADVSignedDeviceIdentity{
			Details:             details,
			AccountSignatureKey: keys.NewKeyPair().Pub[:],
			AccountSignature:    make([]byte, 64),
		}
		signedDetails, _ := proto.Marshal(identity)
		h := hmac.New(sha256.New, tr.Store().AdvSecretKey)
		h.Write(signedDetails)
		container, _ := proto.Marshal(&waAdv.ADVSignedDeviceIdentityHMAC{
			Details: signedDetails, HMAC: h.Sum(nil),
		})

		err := Confirm(t.Context(), tr, container, "req", "", "", jid, lid)
		if !errors.Is(err, ErrInvalidDeviceSignature) {
			t.Fatalf("erro = %v, esperado ErrInvalidDeviceSignature", err)
		}
		erroDoServidor(t, tr, errCodeUnauthorized, errTextSignatureMismatch)
	})

	t.Run("PrePairCallback recusa", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.prePairAllows = false
		err := Confirm(t.Context(), tr, signedIdentity(t, tr, false), "req", "", "", jid, lid)
		if !errors.Is(err, ErrRejectedLocally) {
			t.Fatalf("erro = %v, esperado ErrRejectedLocally", err)
		}
		if tr.prePairCalls != 1 {
			t.Errorf("PrePairAllowed chamado %dx, esperado 1", tr.prePairCalls)
		}
		erroDoServidor(t, tr, errCodeInternal, errTextInternal)
	})

	t.Run("Save falha", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.device.Container = &fakeContainer{saveErr: errors.New("disco cheio")}
		err := Confirm(t.Context(), tr, signedIdentity(t, tr, false), "req", "", "", jid, lid)
		var dbErr *DatabaseError
		if !errors.As(err, &dbErr) || !strings.Contains(err.Error(), "failed to save device store") {
			t.Fatalf("erro = %v (%T), esperado *DatabaseError de save", err, err)
		}
		erroDoServidor(t, tr, errCodeInternal, errTextInternal)
	})

	t.Run("PutIdentity falha apaga o store", func(t *testing.T) {
		tr := newFakeTransport(t)
		tr.identities().err = errors.New("db fora do ar")
		container := &fakeContainer{}
		tr.device.Container = container

		err := Confirm(t.Context(), tr, signedIdentity(t, tr, false), "req", "", "", jid, lid)
		var dbErr *DatabaseError
		if !errors.As(err, &dbErr) || !strings.Contains(err.Error(), "failed to store main device identity") {
			t.Fatalf("erro = %v (%T), esperado *DatabaseError de identity", err, err)
		}
		if container.deletes != 1 {
			t.Errorf("DeleteDevice chamado %dx, esperado 1", container.deletes)
		}
		erroDoServidor(t, tr, errCodeInternal, errTextInternal)
	})

	t.Run("envio da confirmacao falha apaga o store", func(t *testing.T) {
		tr := newFakeTransport(t)
		container := &fakeContainer{}
		tr.device.Container = container
		tr.enqueueSendErr(errors.New("socket fechado"))

		err := Confirm(t.Context(), tr, signedIdentity(t, tr, false), "req", "", "", jid, lid)
		if err == nil || !strings.Contains(err.Error(), "failed to send pairing confirmation") {
			t.Fatalf("erro = %v, esperado falha de envio da confirmacao", err)
		}
		if container.deletes != 1 {
			t.Errorf("DeleteDevice chamado %dx, esperado 1", container.deletes)
		}
	})
}

// O `details` do container pode ser lixo mesmo com HMAC valido — sao dois
// unmarshals distintos, cada um com seu ProtoError.
func TestConfirmDetailsIlegiveis(t *testing.T) {
	jid, lid := pairJIDs(t)

	t.Run("signed device identity ilegivel", func(t *testing.T) {
		tr := newFakeTransport(t)
		garbage := []byte{0xFF, 0xFF, 0xFF}
		h := hmac.New(sha256.New, tr.Store().AdvSecretKey)
		h.Write(garbage)
		container, _ := proto.Marshal(&waAdv.ADVSignedDeviceIdentityHMAC{Details: garbage, HMAC: h.Sum(nil)})

		err := Confirm(t.Context(), tr, container, "req", "", "", jid, lid)
		if !strings.Contains(err.Error(), "failed to parse signed device identity") {
			t.Fatalf("erro = %v", err)
		}
	})

	t.Run("device identity details ilegivel", func(t *testing.T) {
		tr := newFakeTransport(t)
		identity := &waAdv.ADVSignedDeviceIdentity{Details: []byte{0xFF, 0xFF, 0xFF}}
		signedDetails, _ := proto.Marshal(identity)
		h := hmac.New(sha256.New, tr.Store().AdvSecretKey)
		h.Write(signedDetails)
		container, _ := proto.Marshal(&waAdv.ADVSignedDeviceIdentityHMAC{Details: signedDetails, HMAC: h.Sum(nil)})

		err := Confirm(t.Context(), tr, container, "req", "", "", jid, lid)
		if !strings.Contains(err.Error(), "failed to parse device identity details") {
			t.Fatalf("erro = %v", err)
		}
	})
}

// ------------------------------------------------------- HandleSuccessNode

func successNode(t *testing.T, id any, container []byte, jid, lid types.JID) *waBinary.Node {
	t.Helper()
	attrs := waBinary.Attrs{"t": time.Now().Unix()}
	if id != nil {
		attrs["id"] = id
	}
	return &waBinary.Node{
		Tag:   "iq",
		Attrs: attrs,
		Content: []waBinary.Node{{
			Tag: "pair-success",
			Content: []waBinary.Node{
				{Tag: "device-identity", Content: container},
				{Tag: "biz", Attrs: waBinary.Attrs{"name": "Loja"}},
				{Tag: "device", Attrs: waBinary.Attrs{"jid": jid, "lid": lid}},
				{Tag: "platform", Attrs: waBinary.Attrs{"name": "android"}},
			},
		}},
	}
}

// esperaEvento aguarda a goroutine de HandleSuccessNode publicar um evento.
func esperaEvento(t *testing.T, tr *fakeTransport) any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if evts := tr.snapshotEvents(); len(evts) > 0 {
			return evts[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("nenhum evento de pareamento foi emitido")
	return nil
}

func TestHandleSuccessNodeEmitePairSuccess(t *testing.T) {
	tr := newFakeTransport(t)
	jid, lid := pairJIDs(t)
	node := successNode(t, "req-9", signedIdentity(t, tr, false), jid, lid)

	HandleSuccessNode(t.Context(), tr, node)

	evt, ok := esperaEvento(t, tr).(*events.PairSuccess)
	if !ok {
		t.Fatalf("evento = %T, esperado *events.PairSuccess", esperaEvento(t, tr))
	}
	if evt.ID != jid || evt.LID != lid || evt.BusinessName != "Loja" || evt.Platform != "android" {
		t.Errorf("evento = %+v, campos nao vieram do no", evt)
	}
}

func TestHandleSuccessNodeEmitePairError(t *testing.T) {
	tr := newFakeTransport(t)
	jid, lid := pairJIDs(t)
	// Container ilegivel: Confirm falha.
	node := successNode(t, "req-9", []byte{0xFF, 0xFF, 0xFF}, jid, lid)

	HandleSuccessNode(t.Context(), tr, node)

	evt, ok := esperaEvento(t, tr).(*events.PairError)
	if !ok {
		t.Fatalf("evento = %T, esperado *events.PairError", esperaEvento(t, tr))
	}
	if evt.Error == nil {
		t.Error("PairError.Error deveria estar preenchido")
	}
}

// Um <iq> de pair-success sem atributo `id` string derrubava o processo antes
// da Fase E lote 4 (panic em goroutine de nodeHandler, sem recover). A guarda
// atravessou a extracao.
func TestHandleSuccessNodeSemIDNaoEntraEmPanico(t *testing.T) {
	tr := newFakeTransport(t)
	jid, lid := pairJIDs(t)

	for name, id := range map[string]any{
		"ausente":      nil,
		"nao e string": 42,
	} {
		t.Run(name, func(t *testing.T) {
			HandleSuccessNode(t.Context(), tr, successNode(t, id, nil, jid, lid))
			if len(tr.snapshotEvents()) != 0 {
				t.Error("nao deveria emitir evento nenhum")
			}
		})
	}
}

// -------------------------------------------------------------- SendError

func TestSendErrorLogaFalhaDeEnvio(t *testing.T) {
	tr := newFakeTransport(t)
	tr.enqueueSendErr(errors.New("socket fechado"))

	// O contrato e' nao entrar em panico e nao propagar erro.
	SendError(context.Background(), tr, "id", errCodeInternal, errTextInternal)

	if len(tr.snapshotNodes()) != 1 {
		t.Error("o no de erro deveria ter sido tentado mesmo assim")
	}
}
