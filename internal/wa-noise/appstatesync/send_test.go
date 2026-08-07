// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstatesync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/appstate"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	"wa-api/internal/wa-noise/protocol/types"
)

// sendPatch monta um PatchInfo minimo. O conteudo nao importa para os caminhos
// exercitados aqui: todos falham antes ou depois da codificacao, que e'
// responsabilidade do subpacote appstate/.
func sendPatch() appstate.PatchInfo {
	return appstate.PatchInfo{
		Timestamp: time.Now(),
		Type:      appstate.WAPatchRegular,
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexPin, dispatchTestChatJID},
			Version: 3,
			// Value nao pode ser nil: EncodePatch escreve o timestamp nele sem
			// checar (appstate/encode.go:50).
			Value: &waSyncAction.SyncActionValue{
				PinAction: &waSyncAction.PinAction{Pinned: proto.Bool(true)},
			},
		}},
	}
}

func TestSendErroAoLerVersao(t *testing.T) {
	t.Parallel()
	tp, as := syncTransport()
	as.getErr = errors.New("db travado")
	if err := Send(context.Background(), tp, sendPatch(), true); err == nil {
		t.Fatal("esperado o erro do store")
	}
}

func TestSendErroAoLerChave(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.AppStateKeys = &fakeKeyStore{latestErr: errors.New("db travado")}
	err := Send(context.Background(), tp, sendPatch(), true)
	if err == nil || !strings.Contains(err.Error(), "failed to get latest app state key ID") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

// TestSendSemChaveNenhuma trava a mensagem do caso em que o dispositivo nunca
// recebeu chave de app state: criar chave nova nao e' suportado.
func TestSendSemChaveNenhuma(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	err := Send(context.Background(), tp, sendPatch(), true)
	if err == nil || !strings.Contains(err.Error(), "no app state keys found") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

// TestSendErroDeCodificacao: com um key ID que nao existe no store, EncodePatch
// falha e o erro sobe cru.
func TestSendErroDeCodificacao(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.AppStateKeys = &fakeKeyStore{latestID: []byte{1, 2, 3}}
	if err := Send(context.Background(), tp, sendPatch(), true); err == nil {
		t.Fatal("esperado erro de EncodePatch")
	}
	if len(tp.sentIQs) != 0 {
		t.Error("nada deveria ter sido enviado")
	}
}

// TestSendCaminhoFeliz cobre o envio completo: IQ de patch, resposta de
// sucesso, e a ressincronizacao que vem depois.
func TestSendCaminhoFeliz(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.AppStateKeys = keyStoreComChave()
	// Duas respostas: a do envio (sem type=error) e a do fetch subsequente.
	ok := collectionNode(string(appstate.WAPatchRegular), false)
	tp.iqSeq = []*waBinary.Node{ok, ok}
	tp.iqErrs = []error{nil, nil}

	if err := Send(context.Background(), tp, sendPatch(), true); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(tp.sentIQs) != 2 {
		t.Fatalf("esperado 2 IQs (envio + fetch), veio %d", len(tp.sentIQs))
	}
	envio := tp.sentIQs[0].Content.([]waBinary.Node)[0].Content.([]waBinary.Node)[0]
	if envio.Attrs[attrName] != string(appstate.WAPatchRegular) {
		t.Errorf("name = %v", envio.Attrs[attrName])
	}
	if envio.Attrs[attrReturnSnapshot] != false {
		t.Errorf("o envio nao deveria pedir snapshot: %v", envio.Attrs)
	}
	patchNode := envio.Content.([]waBinary.Node)[0]
	if patchNode.Tag != patchTag {
		t.Errorf("tag do patch = %q, esperado %q", patchNode.Tag, patchTag)
	}
	if len(patchNode.Content.([]byte)) == 0 {
		t.Error("o patch codificado nao deveria ser vazio")
	}
}

func TestSendErroNoIQ(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.AppStateKeys = keyStoreComChave()
	tp.iqErr = errors.New("timeout")
	if err := Send(context.Background(), tp, sendPatch(), true); err == nil {
		t.Fatal("esperado o erro do IQ")
	}
}

// TestSendColecaoAusenteNaResposta cobre a guarda de elemento faltando.
func TestSendColecaoAusenteNaResposta(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.AppStateKeys = keyStoreComChave()
	tp.iq = &waBinary.Node{Tag: "iq"}
	if err := Send(context.Background(), tp, sendPatch(), true); err == nil {
		t.Fatal("esperado erro de elemento ausente")
	}
	if len(tp.elementCalls) != 1 || tp.elementCalls[0] != collectionTag+"/"+sendErrContext {
		t.Errorf("ElementMissing chamado com %v", tp.elementCalls)
	}
}

// TestSendFetchPosteriorFalha: o envio deu certo, mas a ressincronizacao nao.
func TestSendFetchPosteriorFalha(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.AppStateKeys = keyStoreComChave()
	tp.iqSeq = []*waBinary.Node{collectionNode(string(appstate.WAPatchRegular), false), nil}
	tp.iqErrs = []error{nil, errors.New("timeout")}
	err := Send(context.Background(), tp, sendPatch(), true)
	if err == nil || !strings.Contains(err.Error(), "failed to fetch app state after sending update") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

// TestSendRespostaDeErroVaiParaAPolitica: uma `<collection type="error">` cai no
// tratamento de erro, e sem conflito nao ha' retentativa.
func TestSendRespostaDeErro(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.AppStateKeys = keyStoreComChave()
	collection := errorCollection("500")
	tp.iq = &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag:     syncTag,
			Content: []waBinary.Node{collection},
		}},
	}
	err := Send(context.Background(), tp, sendPatch(), true)
	if !errors.Is(err, ErrUpdate) {
		t.Fatalf("erro deveria embrulhar ErrUpdate, veio %v", err)
	}
	if len(tp.sentIQs) != 1 {
		t.Errorf("nao deveria ter retentado: %d IQs", len(tp.sentIQs))
	}
}

// --- handleSendError, testado diretamente ---
//
// O caminho completo de Send ate' a resposta exige uma chave de app state real
// no store (EncodePatch e' cripto de verdade, do subpacote appstate/). A
// politica de erro da resposta, que e' a logica DESTE pacote, e' exercitada
// chamando handleSendError direto.

func errorCollection(code string) waBinary.Node {
	attrs := waBinary.Attrs{
		attrType: respTypeError,
		// O parse da lista de patches exige o atributo name na collection.
		attrName: string(appstate.WAPatchRegular),
	}
	content := []waBinary.Node{}
	if code != "" {
		content = append(content, waBinary.Node{
			Tag:   errorTag,
			Attrs: waBinary.Attrs{attrCode: code},
		})
	}
	return waBinary.Node{Tag: collectionTag, Attrs: attrs, Content: content}
}

func TestHandleSendErrorSemTagDeErro(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	err := handleSendError(context.Background(), tp, sendPatch(), appstate.HashState{}, errorCollection(""), true)
	if !errors.Is(err, ErrUpdate) {
		t.Fatalf("erro deveria embrulhar ErrUpdate, veio %v", err)
	}
}

// TestHandleSendErrorNaoConflito: qualquer codigo que nao seja 409 devolve o
// erro sem tentar de novo.
func TestHandleSendErrorNaoConflito(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	err := handleSendError(context.Background(), tp, sendPatch(), appstate.HashState{}, errorCollection("500"), true)
	if !errors.Is(err, ErrUpdate) {
		t.Fatalf("erro deveria embrulhar ErrUpdate, veio %v", err)
	}
	if len(tp.sentIQs) != 0 {
		t.Error("nao deveria ter retentado")
	}
}

// TestHandleSendErrorConflitoSemRetentativa: com allowRetry false o conflito
// tambem so' devolve o erro. E' o que impede recursao infinita.
func TestHandleSendErrorConflitoSemRetentativa(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	err := handleSendError(context.Background(), tp, sendPatch(), appstate.HashState{}, errorCollection("409"), false)
	if !errors.Is(err, ErrUpdate) {
		t.Fatalf("erro deveria embrulhar ErrUpdate, veio %v", err)
	}
	if len(tp.sentIQs) != 0 {
		t.Error("nao deveria ter retentado")
	}
}

// TestHandleSendErrorConflitoAplicaOsPatches: com 409 e retentativa permitida,
// os patches da resposta sao aplicados. Aqui a aplicacao falha (nao ha' chave),
// e o erro resultante embrulha os dois: o original e o da aplicacao.
func TestHandleSendErrorConflitoAplicacaoFalha(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	collection := errorCollection("409")
	collection.Content = append(collection.Content.([]waBinary.Node), waBinary.Node{
		Tag: "patches",
		Content: []waBinary.Node{{
			Tag:     "patch",
			Content: mustMarshalRemovePatch(t),
		}},
	})
	err := handleSendError(context.Background(), tp, sendPatch(), appstate.HashState{}, collection, true)
	if !errors.Is(err, ErrUpdate) {
		t.Fatalf("erro deveria embrulhar ErrUpdate, veio %v", err)
	}
	if !strings.Contains(err.Error(), "applying patches in the response failed") {
		t.Errorf("erro deveria citar a falha ao aplicar: %v", err)
	}
}

// TestHandleSendErrorConflitoParseFalha: um patch que nao e' protobuf valido
// faz o parse falhar antes da aplicacao.
func TestHandleSendErrorConflitoParseFalha(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	collection := errorCollection("409")
	collection.Content = append(collection.Content.([]waBinary.Node), waBinary.Node{
		Tag: "patches",
		Content: []waBinary.Node{{
			Tag:     "patch",
			Content: []byte{0xff, 0xff, 0xff, 0xff},
		}},
	})
	err := handleSendError(context.Background(), tp, sendPatch(), appstate.HashState{}, collection, true)
	if !strings.Contains(err.Error(), "parsing patches in the response failed") {
		t.Errorf("erro deveria citar a falha de parse: %v", err)
	}
}

// --- MarkNotDirty ---

func TestMarkNotDirty(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	ts := time.Unix(1700000000, 0)
	if err := MarkNotDirty(context.Background(), tp, "account_sync", ts); err != nil {
		t.Fatalf("MarkNotDirty: %v", err)
	}
	if len(tp.sentIQs) != 1 {
		t.Fatalf("esperado 1 IQ, veio %d", len(tp.sentIQs))
	}
	iq := tp.sentIQs[0]
	if iq.Namespace != dirtyNamespace || iq.To != types.ServerJID || iq.Type != IQSet {
		t.Errorf("IQ inesperado: %+v", iq)
	}
	clean := iq.Content.([]waBinary.Node)[0]
	if clean.Tag != dirtyCleanTag {
		t.Errorf("tag = %q, esperado %q", clean.Tag, dirtyCleanTag)
	}
	if clean.Attrs[dirtyCleanAttrType] != "account_sync" {
		t.Errorf("type = %v", clean.Attrs[dirtyCleanAttrType])
	}
	// O timestamp deste protocolo vai em SEGUNDOS.
	if clean.Attrs[dirtyCleanAttrTimestamp] != ts.Unix() {
		t.Errorf("timestamp = %v, esperado %d", clean.Attrs[dirtyCleanAttrTimestamp], ts.Unix())
	}
}

func TestMarkNotDirtyPropagaErro(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.iqErr = errors.New("timeout")
	if err := MarkNotDirty(context.Background(), tp, "account_sync", time.Now()); err == nil {
		t.Fatal("esperado o erro do IQ")
	}
}

// --- dispatchAll ---

func TestDispatchAll(t *testing.T) {
	t.Parallel()
	tp := newFakeTransport()
	dispatchAll(tp, []any{"a", "b"})
	if len(tp.dispatched) != 2 {
		t.Errorf("despachados %d, esperado 2", len(tp.dispatched))
	}
}

// TestHandleSendErrorConflitoRetenta cobre o unico caminho de retentativa: com
// 409 e allowRetry, os patches conflitantes da resposta sao aplicados e o envio
// e' refeito — desta vez com allowRetry false, o que impede recursao infinita.
func TestHandleSendErrorConflitoRetenta(t *testing.T) {
	t.Parallel()
	tp, _ := syncTransport()
	tp.store.AppStateKeys = keyStoreComChave()
	collection := errorCollection("409")
	// Lista de patches vazia: a aplicacao passa sem tocar em cripto, que e'
	// responsabilidade do subpacote appstate/ e nao deste.
	collection.Content = append(collection.Content.([]waBinary.Node), waBinary.Node{
		Tag:     "patches",
		Content: []waBinary.Node{},
	})
	ok := collectionNode(string(appstate.WAPatchRegular), false)
	// Duas respostas para o reenvio: o proprio envio e o fetch seguinte.
	tp.iqSeq = []*waBinary.Node{ok, ok}
	tp.iqErrs = []error{nil, nil}

	if err := handleSendError(context.Background(), tp, sendPatch(), appstate.HashState{}, collection, true); err != nil {
		t.Fatalf("a retentativa deveria ter dado certo: %v", err)
	}
	if len(tp.sentIQs) != 2 {
		t.Errorf("esperado 2 IQs no reenvio, veio %d", len(tp.sentIQs))
	}
}
