package newsletter

import (
	"context"
	"errors"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// Sem params, o no leva so' a identificacao do canal — nenhum atributo de
// paginacao. Mandar count=0 faria o servidor devolver zero mensagens.
func TestMessagesAttrsSemParams(t *testing.T) {
	jid := testJID()
	attrs := messagesAttrs(jid, nil)

	if attrs["type"] != "jid" {
		t.Errorf("type = %v, esperava \"jid\"", attrs["type"])
	}
	if attrs["jid"] != jid {
		t.Errorf("jid = %v, esperava %v", attrs["jid"], jid)
	}
	if _, ok := attrs["count"]; ok {
		t.Error("sem params nao deveria haver count")
	}
	if _, ok := attrs["before"]; ok {
		t.Error("sem params nao deveria haver before")
	}
}

func TestMessagesAttrsCamposZeradosSaoOmitidos(t *testing.T) {
	attrs := messagesAttrs(testJID(), &GetMessagesParams{})
	if len(attrs) != 2 {
		t.Fatalf("params zerado deveria render so' type+jid, veio %v", attrs)
	}
}

func TestMessagesAttrsComPaginacao(t *testing.T) {
	attrs := messagesAttrs(testJID(), &GetMessagesParams{
		Count:  50,
		Before: 987,
	})
	if attrs["count"] != 50 {
		t.Errorf("count = %v, esperava 50", attrs["count"])
	}
	if attrs["before"] != types.MessageServerID(987) {
		t.Errorf("before = %v, esperava 987", attrs["before"])
	}
}

// messagesNode monta uma resposta de <iq> com um <messages> dentro.
func messagesNode() *waBinary.Node {
	return &waBinary.Node{
		Tag:     "iq",
		Content: []waBinary.Node{{Tag: messagesTag}},
	}
}

// O <iq> de mensagens vai para o SERVIDOR (nao para o canal) e o parsing e'
// delegado ao Transport.
func TestGetMessagesEnviaParaOServidorEDelegaOParsing(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = messagesNode()
	f.parsed = []*types.NewsletterMessage{{MessageServerID: 1}, {MessageServerID: 2}}

	msgs, err := GetMessages(context.Background(), f, testJID(), &GetMessagesParams{Count: 10})
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("esperava 2 mensagens, veio %d", len(msgs))
	}
	iq := f.iqs[0]
	if iq.Namespace != Namespace || iq.Type != IQGet {
		t.Errorf("iq = %+v", iq)
	}
	if iq.To != types.ServerJID {
		t.Errorf("to = %v, esperava %v (o IQ de mensagens vai para o servidor)", iq.To, types.ServerJID)
	}
	nodes := iq.Content.([]waBinary.Node)
	if nodes[0].Tag != messagesTag || nodes[0].Attrs["count"] != 10 {
		t.Errorf("no = %+v", nodes[0])
	}
	if f.parseCnt != 1 {
		t.Errorf("ParseMessages chamado %d vezes, esperava 1", f.parseCnt)
	}
}

func TestGetMessagesPropagaErroDoIQ(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	msgs, err := GetMessages(context.Background(), f, testJID(), nil)
	if msgs != nil {
		t.Errorf("msgs = %v, esperava nil", msgs)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

func TestGetMessagesElementoAusente(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{Tag: "iq"}

	_, err := GetMessages(context.Background(), f, testJID(), nil)
	var eme *elementMissingError
	if !errors.As(err, &eme) {
		t.Fatalf("err = %v (%T), esperava elementMissingError", err, err)
	}
	if eme.Tag != messagesTag || eme.In != messagesErrContext {
		t.Errorf("erro = %+v", eme)
	}
	if f.parseCnt != 0 {
		t.Error("ParseMessages nao deveria ter sido chamado")
	}
}

// TestGetMessageUpdatesEnviaParaOServidorComStanzaDeMessages trava a causa
// da F265/LIB-03, nao o sintoma: antes desta correcao, o IQ ia para o JID
// do CANAL num no <message_updates>, e o servidor simplesmente nunca
// respondia (silencio ate' o timeout de 30s, nao um erro). A forma nova e'
// IDENTICA ao IQ de GetMessages — mesmo destino, mesmo no <messages> — que
// ja' e' ✅ e responde com sucesso.
func TestGetMessageUpdatesEnviaParaOServidorComStanzaDeMessages(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = messagesNode()
	f.parsed = []*types.NewsletterMessage{{MessageServerID: 9}}

	jid := testJID()
	msgs, err := GetMessageUpdates(context.Background(), f, jid, &GetUpdatesParams{Count: 3, After: 42})
	if err != nil {
		t.Fatalf("GetMessageUpdates: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("esperava 1 mensagem, veio %d", len(msgs))
	}
	iq := f.iqs[0]
	if iq.To != types.ServerJID {
		t.Errorf("to = %v, esperava %v (o IQ de updates passa a ir para o servidor, como GetMessages)", iq.To, types.ServerJID)
	}
	nodes := iq.Content.([]waBinary.Node)
	if nodes[0].Tag != messagesTag {
		t.Errorf("tag do no = %q, esperava %q (nao mais message_updates)", nodes[0].Tag, messagesTag)
	}
	if nodes[0].Attrs["count"] != 3 {
		t.Errorf("count = %v, esperava 3", nodes[0].Attrs["count"])
	}
	if nodes[0].Attrs["before"] != types.MessageServerID(42) {
		t.Errorf("before = %v, esperava 42 (After mapeado para before)", nodes[0].Attrs["before"])
	}
}

// TestGetMessageUpdatesSinceEIgnorado trava que Since nao vai mais para o
// fio: a forma nova so' entende `before` (ID de mensagem), nao tem
// equivalente para filtro por tempo.
func TestGetMessageUpdatesSinceEIgnorado(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = messagesNode()

	since := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	_, err := GetMessageUpdates(context.Background(), f, testJID(), &GetUpdatesParams{Count: 5, Since: since})
	if err != nil {
		t.Fatalf("GetMessageUpdates: %v", err)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	if _, ok := nodes[0].Attrs["since"]; ok {
		t.Errorf("no = %+v, since nao deveria aparecer no fio", nodes[0])
	}
}

func TestGetMessageUpdatesPropagaErroDoIQ(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if _, err := GetMessageUpdates(context.Background(), f, testJID(), nil); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

func TestGetMessageUpdatesElementoAusente(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{Tag: "iq"}

	_, err := GetMessageUpdates(context.Background(), f, testJID(), nil)
	var eme *elementMissingError
	if !errors.As(err, &eme) {
		t.Fatalf("err = %v (%T), esperava elementMissingError", err, err)
	}
	if eme.Tag != messagesTag {
		t.Errorf("tag = %q, esperava %q", eme.Tag, messagesTag)
	}
}
