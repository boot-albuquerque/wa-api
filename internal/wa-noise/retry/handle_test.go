// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package retry

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mau.fi/libsignal/keys/prekey"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/prekeys"
	"wa-api/internal/wa-noise/proto/waCommon"
	"wa-api/internal/wa-noise/proto/waConsumerApplication"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/proto/waMsgApplication"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/security/keys"
)

// --- ShouldRecreateSession ---

func TestShouldRecreateSession(t *testing.T) {
	for name, tc := range map[string]struct {
		hasSession   bool
		sessionErr   error
		retryCount   int
		previous     time.Duration // ha' quanto tempo a sessao foi recriada; 0 = nunca
		wantRecreate bool
	}{
		"erro no store nao recria":              {false, errors.New("boom"), 5, 0, false},
		"sem sessao recria sempre":              {false, nil, 0, 0, true},
		"com sessao e count baixo nao recria":   {true, nil, MinCountForSessionRecreate - 1, 0, false},
		"com sessao, count alto, sem historico": {true, nil, MinCountForSessionRecreate, 0, true},
		"recriada ha' pouco nao recria de novo": {true, nil, MinCountForSessionRecreate, time.Minute, false},
		"recriada ha' mais de uma hora recria":  {true, nil, MinCountForSessionRecreate, 2 * time.Hour, true},
	} {
		t.Run(name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.stores.hasSession = tc.hasSession
			tr.stores.hasSessionErr = tc.sessionErr
			if tc.previous != 0 {
				tr.state.LockSessionRecreate()
				tr.state.MarkSessionRecreated(testPeerJID, time.Now().Add(-tc.previous))
				tr.state.UnlockSessionRecreate()
			}
			reason, recreate := ShouldRecreateSession(context.Background(), tr, tc.retryCount, testPeerJID)
			if recreate != tc.wantRecreate {
				t.Fatalf("recreate = %v, esperado %v (motivo %q)", recreate, tc.wantRecreate, reason)
			}
			if recreate && reason == "" {
				t.Error("recriar sem motivo no log nao ajuda ninguem")
			}
			if !recreate && reason != "" {
				t.Errorf("motivo = %q, esperado vazio quando nao recria", reason)
			}
		})
	}
}

// Recriar grava o carimbo, e o carimbo e' o que impede a recriacao seguinte.
func TestShouldRecreateSessionRecordsTimestamp(t *testing.T) {
	tr := newFakeTransport()
	tr.stores.hasSession = true
	if _, recreate := ShouldRecreateSession(context.Background(), tr, MinCountForSessionRecreate, testPeerJID); !recreate {
		t.Fatal("a primeira deveria recriar")
	}
	if _, recreate := ShouldRecreateSession(context.Background(), tr, MinCountForSessionRecreate, testPeerJID); recreate {
		t.Error("a segunda, logo em seguida, nao deveria recriar")
	}
}

// --- TryHandleReceipt ---

// O recover existe porque o call site chama isto com `go`: um panic aqui
// derrubaria o processo inteiro.
func TestTryHandleReceiptRecoversFromPanic(t *testing.T) {
	tr := &panickyTransport{Transport: newFakeTransport()}
	TryHandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
	// Chegar aqui ja' e' o teste: sem o recover, o processo morreria — o call
	// site original chama isto com `go`, sem ninguem para pegar o panic.
}

// panickyTransport estoura no primeiro ponto que HandleReceipt consulta depois
// de parsear o <retry>.
type panickyTransport struct {
	Transport
}

func (p *panickyTransport) GetMessageForRetry(types.JID, types.JID, types.MessageID) *waE2E.Message {
	panic("boom")
}

// Guarda de regressao da Fase E lote 5: um recibo sem MessageIDs no caminho de
// ERRO nao pode entrar em panico ao indexar MessageIDs[0]. Antes da correcao
// esta chamada derrubava o processo.
func TestTryHandleReceiptWithoutMessageIDs(t *testing.T) {
	tr := newFakeTransport()
	r := dmReceipt("MSG1")
	r.MessageIDs = nil
	// Sem filho <retry>, HandleReceipt devolve erro e cai no ramo do log, que
	// e' onde o indexar cru estava.
	TryHandleReceipt(context.Background(), tr, r, &waBinary.Node{Tag: "receipt"})
	if len(tr.sent()) != 0 {
		t.Errorf("nos enviados = %d, esperado 0", len(tr.sent()))
	}
}

// O semaforo limita o paralelismo; com 1 permit, duas chamadas se serializam.
func TestTryHandleReceiptRespectsSemaphore(t *testing.T) {
	tr := newFakeTransport()
	tr.state.SetMaxParallel(1)
	var wg sync.WaitGroup
	wg.Add(4)
	for i := 0; i < 4; i++ {
		go func() {
			defer wg.Done()
			TryHandleReceipt(context.Background(), tr, dmReceipt("MSG1"), &waBinary.Node{Tag: "receipt"})
		}()
	}
	wg.Wait()
	if tr.state.Sema() == nil {
		t.Error("o semaforo sumiu")
	}
}

// Contexto ja' cancelado: Acquire falha e a funcao volta sem tratar nada.
func TestTryHandleReceiptGivesUpOnCancelledContext(t *testing.T) {
	tr := newFakeTransport()
	tr.state.SetMaxParallel(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	TryHandleReceipt(ctx, tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
	if len(tr.sent()) != 0 {
		t.Errorf("nos enviados = %d, esperado 0", len(tr.sent()))
	}
}

// --- HandleReceipt: caminhos de erro precoces ---

func TestHandleReceiptWithoutRetryChild(t *testing.T) {
	tr := newFakeTransport()
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), &waBinary.Node{Tag: "receipt"})
	var missing errMissing
	if !errors.As(err, &missing) || missing.tag != "retry" {
		t.Errorf("erro = %v, esperado ElementMissing(retry)", err)
	}
}

func TestHandleReceiptWithBadRetryAttrs(t *testing.T) {
	tr := newFakeTransport()
	node := &waBinary.Node{Tag: "receipt", Attrs: waBinary.Attrs{"from": testPeerJID}, Content: []waBinary.Node{
		{Tag: "retry", Attrs: waBinary.Attrs{}}, // sem id/t/count
	}}
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), node); err == nil {
		t.Error("esperado erro de atributos")
	}
}

func TestHandleReceiptMessageNotFound(t *testing.T) {
	tr := newFakeTransport()
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
	if err == nil || !strings.Contains(err.Error(), "couldn't find message") {
		t.Errorf("erro = %v, esperado 'couldn't find message'", err)
	}
}

func TestHandleReceiptLookupError(t *testing.T) {
	tr := newFakeTransport()
	tr.lids.err = errors.New("boom")
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err == nil {
		t.Error("esperado erro da busca")
	}
}

// --- HandleReceipt: caminho WA completo ---

// cacheMessage coloca a mensagem no cache para que GetForRetry a ache.
func cacheMessage(tr *fakeTransport, id types.MessageID, msg RecentMessage) {
	tr.state.AddRecent(RecentKey{testPeerJID, id}, msg)
}

func TestHandleReceiptWASuccess(t *testing.T) {
	tr := newFakeTransport()
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	sent := tr.sent()
	if len(sent) != 1 || sent[0].Tag != "message" {
		t.Fatalf("nos enviados = %+v", sent)
	}
	if sent[0].Attrs["id"] != types.MessageID("MSG1") {
		t.Errorf("id = %v", sent[0].Attrs["id"])
	}
	// Fora de grupo, device_fanout=false entra.
	if sent[0].Attrs["device_fanout"] != false {
		t.Errorf("device_fanout = %v, esperado false", sent[0].Attrs["device_fanout"])
	}
	enc, ok := childByTag(sent[0], "enc")
	if !ok {
		t.Fatal("faltou o <enc>")
	}
	if enc.Attrs["count"] != 1 {
		t.Errorf("count no <enc> = %v, esperado 1", enc.Attrs["count"])
	}
}

// Os tres atributos opcionais do no original sao copiados quando presentes.
func TestHandleReceiptCopiesOptionalAttrs(t *testing.T) {
	tr := newFakeTransport()
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	extra := waBinary.Attrs{"participant": testPeerJID, "recipient": testOwnJID, "edit": "1"}
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, extra))
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	attrs := tr.sent()[0].Attrs
	for k, want := range extra {
		if attrs[k] != want {
			t.Errorf("%s = %v, esperado %v", k, attrs[k], want)
		}
	}
}

// Em grupo: nao ha device_fanout, e a SKDM e' criada e embutida na mensagem.
func TestHandleReceiptGroup(t *testing.T) {
	tr := newFakeTransport()
	msg := waMessage("oi")
	tr.state.AddRecent(RecentKey{testGroupJID, "MSG1"}, RecentMessage{WA: msg})
	receipt := dmReceipt("MSG1")
	receipt.Chat = testGroupJID
	receipt.IsGroup = true
	if err := HandleReceipt(context.Background(), tr, receipt, retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.skdmChats) != 1 || tr.skdmChats[0] != testGroupJID {
		t.Errorf("SKDM pedida para %v", tr.skdmChats)
	}
	if msg.GetSenderKeyDistributionMessage().GetGroupID() != testGroupJID.String() {
		t.Error("a SKDM nao foi embutida na mensagem")
	}
	if _, has := tr.sent()[0].Attrs["device_fanout"]; has {
		t.Error("device_fanout nao deveria aparecer em grupo")
	}
}

// Falha ao criar a SKDM e' logada e ignorada — o retry sai assim mesmo.
func TestHandleReceiptGroupSKDMErrorIsTolerated(t *testing.T) {
	tr := newFakeTransport()
	tr.skdmErr = errors.New("boom")
	tr.state.AddRecent(RecentKey{testGroupJID, "MSG1"}, RecentMessage{WA: waMessage("oi")})
	receipt := dmReceipt("MSG1")
	receipt.Chat = testGroupJID
	receipt.IsGroup = true
	if err := HandleReceipt(context.Background(), tr, receipt, retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.sent()) != 1 {
		t.Error("o retry deveria ter saido mesmo sem SKDM")
	}
}

// Mensagem para si mesmo vira DeviceSentMessage.
func TestHandleReceiptFromMeWrapsInDeviceSentMessage(t *testing.T) {
	tr := newFakeTransport()
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	receipt := dmReceipt("MSG1")
	receipt.IsFromMe = true
	if err := HandleReceipt(context.Background(), tr, receipt, retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.sent()) != 1 {
		t.Error("esperado um no enviado")
	}
}

// Remetente com servidor de telefone e mapeamento LID conhecido: a sessao
// migra e a cifragem passa a usar o LID.
func TestHandleReceiptMigratesToLID(t *testing.T) {
	tr := newFakeTransport()
	tr.lids.pnToLID[testPeerJID] = testPeerLID
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.migrated) != 1 || tr.migrated[0] != [2]types.JID{testPeerJID, testPeerLID} {
		t.Errorf("migracoes = %v", tr.migrated)
	}
}

// --- teto de retries recebidos (F36) ---

func TestHandleReceiptStopsAtMaxIncoming(t *testing.T) {
	tr := newFakeTransport()
	for i := 0; i < MaxIncomingRequests+2; i++ {
		cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
		if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err != nil {
			t.Fatalf("iteracao %d: %v", i, err)
		}
	}
	if n := len(tr.sent()); n != MaxIncomingRequests-1 {
		t.Errorf("nos enviados = %d, esperado %d", n, MaxIncomingRequests-1)
	}
}

// --- PreRetryCallback ---

func TestHandleReceiptPreRetryCallbackCancels(t *testing.T) {
	tr := newFakeTransport()
	tr.preRetryAllowed = false
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 3, nil)); err != nil {
		t.Fatalf("cancelar no callback nao e' erro: %v", err)
	}
	if len(tr.sent()) != 0 {
		t.Error("nada deveria ter sido enviado")
	}
	if len(tr.preRetryArgs) != 1 || tr.preRetryArgs[0] != 3 {
		t.Errorf("retryCount passado ao callback = %v, esperado [3]", tr.preRetryArgs)
	}
}

// --- bundle de prekey ---

// Com <keys> no proprio recibo, o bundle vem de la' e o servidor nao e'
// consultado.
func TestHandleReceiptUsesBundleFromNode(t *testing.T) {
	tr := newFakeTransport()
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	// Um <keys> mal formado ja' basta para provar que o ramo foi tomado: o erro
	// vem do parser de bundle, nao da busca no servidor.
	node := retryNode("MSG1", 1, nil, waBinary.Node{Tag: "keys"})
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), node)
	if err == nil || !strings.Contains(err.Error(), "prekey bundle in retry receipt") {
		t.Errorf("erro = %v, esperado falha ao ler o bundle do no", err)
	}
}

func TestHandleReceiptFetchesBundleWhenSessionMissing(t *testing.T) {
	tr := newFakeTransport()
	tr.stores.hasSession = false // forca ShouldRecreateSession a devolver true
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	tr.fetchPreKeysResp[testPeerJID] = prekeys.Resp{Bundle: &prekey.Bundle{}}
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.sent()) != 1 {
		t.Error("esperado um no enviado")
	}
}

func TestHandleReceiptBundleErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(*fakeTransport)
		want  string
	}{
		"falha na busca": {func(tr *fakeTransport) {
			tr.fetchPreKeysErr = errors.New("boom")
		}, "boom"},
		"erro por dispositivo": {func(tr *fakeTransport) {
			tr.fetchPreKeysResp[testPeerJID] = prekeys.Resp{Err: errors.New("recusado")}
		}, "failed to fetch prekeys"},
		"resposta sem bundle": {func(tr *fakeTransport) {}, "didn't get prekey bundle"},
	} {
		t.Run(name, func(t *testing.T) {
			tr := newFakeTransport()
			tr.stores.hasSession = false
			cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
			tc.setup(tr)
			err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("erro = %v, esperado conter %q", err, tc.want)
			}
		})
	}
}

// --- erros de cifragem e envio ---

func TestHandleReceiptEncryptError(t *testing.T) {
	tr := newFakeTransport()
	tr.encryptErr = errors.New("boom")
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
	if err == nil || !strings.Contains(err.Error(), "failed to encrypt message for retry") {
		t.Errorf("erro = %v", err)
	}
}

func TestHandleReceiptSendError(t *testing.T) {
	tr := newFakeTransport()
	tr.sendNodeErr = errors.New("boom")
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
	if err == nil || !strings.Contains(err.Error(), "failed to send retry message") {
		t.Errorf("erro = %v", err)
	}
}

// --- caminho FB ---

func fbMessage() *waMsgApplication.MessageApplication {
	return &waMsgApplication.MessageApplication{
		Metadata: &waMsgApplication.MessageApplication_Metadata{FrankingKey: []byte("chave")},
		Payload: &waMsgApplication.MessageApplication_Payload{
			Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{
				SubProtocol: &waMsgApplication.MessageApplication_SubProtocolPayload{
					FutureProof: waCommon.FutureProofBehavior_PLACEHOLDER.Enum(),
				},
			},
		},
	}
}

func TestHandleReceiptFBSuccess(t *testing.T) {
	tr := newFakeTransport()
	cacheMessage(tr, "MSG1", RecentMessage{FB: fbMessage()})
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	sent := tr.sent()
	if len(sent) != 1 {
		t.Fatalf("nos enviados = %d, esperado 1", len(sent))
	}
	// O caminho FB manda <enc> + <franking>, e nao o conteudo montado por
	// MessageContent.
	franking, ok := childByTag(sent[0], "franking")
	if !ok {
		t.Fatal("faltou o <franking>")
	}
	tag, ok := childByTag(franking, "franking_tag")
	if !ok {
		t.Fatal("faltou o <franking_tag>")
	}
	if len(tag.Content.([]byte)) != 32 {
		t.Errorf("franking tag tem %d bytes, esperado 32 (HMAC-SHA256)", len(tag.Content.([]byte)))
	}
	// O payload V3 carrega a versao de aplicacao FB.
	if tr.encV3Payload.GetApplicationPayload().GetVersion() != FBApplicationVersion {
		t.Errorf("version = %d, esperado %d",
			tr.encV3Payload.GetApplicationPayload().GetVersion(), FBApplicationVersion)
	}
	// Sem ConsumerApplication decodificavel, o tipo cai no default "text".
	if sent[0].Attrs["type"] != "text" {
		t.Errorf("type = %v, esperado text", sent[0].Attrs["type"])
	}
}

func TestHandleReceiptFBEncryptError(t *testing.T) {
	tr := newFakeTransport()
	tr.encryptV3Err = errors.New("boom")
	cacheMessage(tr, "MSG1", RecentMessage{FB: fbMessage()})
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
	if err == nil || !strings.Contains(err.Error(), "failed to encrypt message for retry") {
		t.Errorf("erro = %v", err)
	}
}

// ConsumerApplication invalido dentro do payload FB aborta com erro proprio.
func TestHandleReceiptFBBadConsumerMessage(t *testing.T) {
	tr := newFakeTransport()
	fb := fbMessage()
	fb.Payload.GetSubProtocol().SubProtocol = &waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage{
		ConsumerMessage: &waCommon.SubProtocol{Payload: []byte{0xFF, 0xFF, 0xFF}, Version: proto.Int32(1)},
	}
	cacheMessage(tr, "MSG1", RecentMessage{FB: fb})
	err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil))
	if err == nil || !strings.Contains(err.Error(), "failed to decode consumer message") {
		t.Errorf("erro = %v", err)
	}
}

// Grupo no caminho FB: a SKDM vai pelo campo do transporte, nao dentro da
// mensagem.
func TestHandleReceiptFBGroup(t *testing.T) {
	tr := newFakeTransport()
	tr.state.AddRecent(RecentKey{testGroupJID, "MSG1"}, RecentMessage{FB: fbMessage()})
	receipt := dmReceipt("MSG1")
	receipt.Chat = testGroupJID
	receipt.IsGroup = true
	if err := HandleReceipt(context.Background(), tr, receipt, retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.sent()) != 1 {
		t.Error("esperado um no enviado")
	}
}

// FB para si mesmo: DeviceSentMessage vai pelo campo do transporte.
func TestHandleReceiptFBFromMe(t *testing.T) {
	tr := newFakeTransport()
	cacheMessage(tr, "MSG1", RecentMessage{FB: fbMessage()})
	receipt := dmReceipt("MSG1")
	receipt.IsFromMe = true
	if err := HandleReceipt(context.Background(), tr, receipt, retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.sent()) != 1 {
		t.Error("esperado um no enviado")
	}
}

// --- includeDeviceIdentity ---

func TestHandleReceiptIncludesDeviceIdentity(t *testing.T) {
	tr := newFakeTransport()
	tr.includeIdentity = true
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if _, ok := childByTag(tr.sent()[0], "device-identity"); !ok {
		t.Error("faltou o <device-identity>")
	}
}

// Mensagem de midia leva o atributo mediatype no <enc>.
func TestHandleReceiptSetsMediaType(t *testing.T) {
	tr := newFakeTransport()
	cacheMessage(tr, "MSG1", RecentMessage{WA: &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{Mimetype: proto.String("image/jpeg")},
	}})
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	enc, _ := childByTag(tr.sent()[0], "enc")
	if enc.Attrs["mediatype"] != "image" {
		t.Errorf("mediatype = %v, esperado image", enc.Attrs["mediatype"])
	}
}

// Payload FB com ConsumerApplication decodificavel: os atributos da mensagem
// saem de GetAttrsFromFBMessage, e nao do default "text".
func TestHandleReceiptFBUsesConsumerAttrs(t *testing.T) {
	tr := newFakeTransport()
	consumer, err := proto.Marshal(&waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{
				Content: &waConsumerApplication.ConsumerApplication_Content{
					Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{
						MessageText: &waCommon.MessageText{Text: proto.String("oi")},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	fb := fbMessage()
	fb.Payload.GetSubProtocol().SubProtocol = &waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage{
		ConsumerMessage: &waCommon.SubProtocol{Payload: consumer, Version: proto.Int32(1)},
	}
	cacheMessage(tr, "MSG1", RecentMessage{FB: fb})
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got := tr.sent()[0].Attrs["type"]; got != "text" {
		t.Errorf("type = %v", got)
	}
}

// Falha ao consultar o LID na hora de cifrar e' logada e ignorada: o retry sai
// cifrado para o JID original. Diferente do mesmo erro dentro de GetForRetry,
// que aborta — a assimetria e' do upstream.
func TestHandleReceiptLIDLookupErrorAtEncryptIsTolerated(t *testing.T) {
	tr := newFakeTransport()
	// A mensagem esta' no cache sob o proprio JID do chat, entao GetForRetry
	// devolve antes de tocar em LIDs; o erro so' aparece na cifragem.
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})
	tr.lids.err = errors.New("boom")
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), retryNode("MSG1", 1, nil)); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.migrated) != 0 {
		t.Errorf("nao deveria ter migrado sessao: %v", tr.migrated)
	}
	if len(tr.sent()) != 1 {
		t.Error("o retry deveria ter saido assim mesmo")
	}
}

// Com um <keys> VALIDO no proprio recibo, o bundle vem de la' e o servidor nao
// e' consultado — e' o caminho normal quando o par acabou de perder a sessao.
func TestHandleReceiptUsesValidBundleFromNode(t *testing.T) {
	tr := newFakeTransport()
	tr.stores.hasSession = false // se caisse no fetch, nao haveria bundle
	cacheMessage(tr, "MSG1", RecentMessage{WA: waMessage("oi")})

	signed := &keys.PreKey{KeyPair: *keys.NewKeyPair(), KeyID: 7, Signature: &[64]byte{}}
	var regBytes [prekeys.RegistrationIDLength]byte
	binary.BigEndian.PutUint32(regBytes[:], 0xCAFEBABE)
	node := retryNode("MSG1", 1, nil,
		waBinary.Node{Tag: "registration", Content: regBytes[:]},
		waBinary.Node{Tag: "keys", Content: []waBinary.Node{
			{Tag: "identity", Content: tr.device.IdentityKey.Pub[:]},
			prekeys.ToNode(signed),
		}},
	)
	if err := HandleReceipt(context.Background(), tr, dmReceipt("MSG1"), node); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(tr.sent()) != 1 {
		t.Error("esperado um no enviado")
	}
}
