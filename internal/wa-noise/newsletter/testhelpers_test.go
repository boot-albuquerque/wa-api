// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package newsletter

import (
	"context"
	"fmt"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// elementMissingError replica o *whatsmeow.ElementMissingError da raiz (que
// este pacote nao pode importar) para que os testes possam afirmar que o erro
// devolvido veio de Transport.ElementMissing, com tag e contexto corretos.
type elementMissingError struct {
	Tag string
	In  string
}

func (e *elementMissingError) Error() string {
	return fmt.Sprintf("missing <%s> element in %s", e.Tag, e.In)
}

// fakeTransport e' o duble de Transport usado por todo o pacote. Nao tem
// socket, store nem sessao: cada metodo devolve o que o teste programou e
// registra o que recebeu.
type fakeTransport struct {
	// programaveis
	iqResp    *waBinary.Node
	iqErr     error
	nodeErr   error
	reqID     string
	respChan  chan *waBinary.Node
	msgID     types.MessageID
	parsed    []*types.NewsletterMessage
	payload   *waWa6.ClientPayload
	parseNode *waBinary.Node

	// registros
	iqs       []IQ
	nodes     []waBinary.Node
	canceled  []string
	waitedFor []string
	parseCnt  int
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		reqID:   "REQ1",
		msgID:   "GENERATED",
		payload: webPayload(),
	}
}

func (f *fakeTransport) SendIQ(_ context.Context, query IQ) (*waBinary.Node, error) {
	f.iqs = append(f.iqs, query)
	return f.iqResp, f.iqErr
}

func (f *fakeTransport) SendNode(_ context.Context, node waBinary.Node) error {
	f.nodes = append(f.nodes, node)
	return f.nodeErr
}

func (f *fakeTransport) GenerateRequestID() string { return f.reqID }

func (f *fakeTransport) WaitResponse(reqID string) chan *waBinary.Node {
	f.waitedFor = append(f.waitedFor, reqID)
	if f.respChan == nil {
		// Bufferizado e ja' preenchido: MarkViewed bloqueia em <-resp, e um
		// canal vazio travaria o teste.
		f.respChan = make(chan *waBinary.Node, 1)
		f.respChan <- &waBinary.Node{Tag: "ack"}
	}
	return f.respChan
}

func (f *fakeTransport) CancelResponse(reqID string, _ chan *waBinary.Node) {
	f.canceled = append(f.canceled, reqID)
}

func (f *fakeTransport) GenerateMessageID() types.MessageID { return f.msgID }

func (f *fakeTransport) ParseMessages(node *waBinary.Node) []*types.NewsletterMessage {
	f.parseCnt++
	f.parseNode = node
	return f.parsed
}

func (f *fakeTransport) ClientPayload() *waWa6.ClientPayload { return f.payload }

func (f *fakeTransport) ElementMissing(tag, in string) error {
	return &elementMissingError{Tag: tag, In: in}
}

func (f *fakeTransport) Log() waLog.Logger { return waLog.Noop }

// mexNode monta uma resposta de <iq> MEX com o conteudo e os atributos dados no
// no <result>.
func mexNode(content any, attrs waBinary.Attrs) *waBinary.Node {
	if attrs == nil {
		attrs = waBinary.Attrs{}
	}
	return &waBinary.Node{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag:     mexResultTag,
			Attrs:   attrs,
			Content: content,
		}},
	}
}

// mexJSON monta uma resposta MEX em JSON puro (o caminho vivo hoje).
func mexJSON(body string) *waBinary.Node {
	return mexNode([]byte(body), nil)
}

// withWebInfo garante que o payload BASE (global do pacote store, lido pelo
// guard de MACOS de SendMexIQ) esteja no estado de cliente web durante o teste.
// Restaura no fim para nao contaminar os outros testes do pacote.
func withPlatform(t *testing.T, p *waWa6.ClientPayload_UserAgent_Platform) {
	t.Helper()
	original := store.BaseClientPayload.UserAgent.Platform
	store.BaseClientPayload.UserAgent.Platform = p
	t.Cleanup(func() {
		store.BaseClientPayload.UserAgent.Platform = original
	})
}

func testJID() types.JID {
	return types.NewJID("1234567890", types.NewsletterServer)
}
