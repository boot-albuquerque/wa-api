package newsletter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
)

// O guard de MACOS aborta SendMexIQ antes de tocar no Transport: e' o unico
// caminho de SendMexIQ que nao depende de resposta do servidor.
func TestSendMexIQRecusaEmMacOS(t *testing.T) {
	withPlatform(t, waWa6.ClientPayload_UserAgent_MACOS.Enum())

	f := newFakeTransport()
	data, err := SendMexIQ(context.Background(), f, queryFetchNewsletter, nil)
	if data != nil {
		t.Errorf("esperava data nil, veio %s", data)
	}
	if !errors.Is(err, ErrArgoDecodingBroken) {
		t.Fatalf("err = %v, esperava ErrArgoDecodingBroken", err)
	}
	if len(f.iqs) != 0 {
		t.Errorf("o guard nao deveria mandar IQ nenhum, mandou %d", len(f.iqs))
	}
}

// Variaveis nao serializaveis abortam antes do envio.
func TestSendMexIQErroDeMarshalDasVariaveis(t *testing.T) {
	f := newFakeTransport()
	_, err := SendMexIQ(context.Background(), f, queryFetchNewsletter, make(chan int))
	if err == nil {
		t.Fatal("esperava erro de marshal")
	}
	if len(f.iqs) != 0 {
		t.Errorf("nao deveria ter mandado IQ, mandou %d", len(f.iqs))
	}
}

// O IQ montado carrega o namespace, o tipo, o destino e a query ID convertida.
func TestSendMexIQMontaOIQEConverteAQueryID(t *testing.T) {
	f := newFakeTransport()
	f.payload = desktopPayload() // forca a conversao web -> desktop
	f.iqResp = mexJSON(`{"data":{"ok":true}}`)

	data, err := SendMexIQ(context.Background(), f, queryFetchNewsletter, map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("SendMexIQ: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("data = %s", data)
	}
	if len(f.iqs) != 1 {
		t.Fatalf("esperava 1 IQ, veio %d", len(f.iqs))
	}
	iq := f.iqs[0]
	if iq.Namespace != mexNamespace || iq.Type != IQGet {
		t.Errorf("iq = %+v, esperava namespace %q tipo %q", iq, mexNamespace, IQGet)
	}
	nodes, ok := iq.Content.([]waBinary.Node)
	if !ok || len(nodes) != 1 {
		t.Fatalf("conteudo = %T %v", iq.Content, iq.Content)
	}
	if nodes[0].Tag != mexQueryTag {
		t.Errorf("tag = %q, esperava %q", nodes[0].Tag, mexQueryTag)
	}
	if nodes[0].Attrs[mexQueryIDAttr] != queryFetchNewsletterDesktop {
		t.Errorf("query_id = %v, esperava a ID de desktop %q", nodes[0].Attrs[mexQueryIDAttr], queryFetchNewsletterDesktop)
	}
	// As variaveis vao embrulhadas em {"variables": ...}.
	if string(nodes[0].Content.([]byte)) != `{"variables":{"a":1}}` {
		t.Errorf("payload = %s", nodes[0].Content)
	}
}

func TestSendMexIQPropagaErroDoIQ(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("boom")
	f.iqErr = sentinel

	if _, err := SendMexIQ(context.Background(), f, queryFetchNewsletter, nil); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

// Resposta sem <result> vira ElementMissingError, construido pelo Transport
// para preservar o tipo concreto da raiz.
func TestSendMexIQResultadoAusente(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = &waBinary.Node{Tag: "iq"}

	_, err := SendMexIQ(context.Background(), f, queryFetchNewsletter, nil)
	var eme *elementMissingError
	if !errors.As(err, &eme) {
		t.Fatalf("err = %v (%T), esperava elementMissingError", err, err)
	}
	if eme.Tag != mexResultTag || eme.In != mexErrContext {
		t.Errorf("erro = %+v, esperava tag %q em %q", eme, mexResultTag, mexErrContext)
	}
}

// <result> com conteudo que nao e' []byte (ex: filhos aninhados) e' erro de
// formato, nao de protocolo.
func TestSendMexIQConteudoDeTipoInesperado(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexNode([]waBinary.Node{{Tag: "x"}}, nil)

	_, err := SendMexIQ(context.Background(), f, queryFetchNewsletter, nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected content type") {
		t.Fatalf("err = %v, esperava erro de tipo de conteudo", err)
	}
}

// format="argo" cai no decodificador Argo, que esta' desabilitado no fork.
func TestSendMexIQFormatoArgoEstaDesabilitado(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexNode([]byte("qualquer"), waBinary.Attrs{mexFormatAttr: mexFormatArgo})

	data, err := SendMexIQ(context.Background(), f, queryFetchNewsletter, nil)
	if data != nil {
		t.Errorf("esperava data nil, veio %s", data)
	}
	if !errors.Is(err, ErrArgoDecodingBroken) {
		t.Fatalf("err = %v, esperava ErrArgoDecodingBroken", err)
	}
}

// Um format diferente de "argo" continua no caminho JSON.
func TestSendMexIQFormatoDesconhecidoVaiParaJSON(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexNode([]byte(`{"data":{"ok":1}}`), waBinary.Attrs{mexFormatAttr: "json"})

	data, err := SendMexIQ(context.Background(), f, queryFetchNewsletter, nil)
	if err != nil {
		t.Fatalf("SendMexIQ: %v", err)
	}
	if string(data) != `{"ok":1}` {
		t.Errorf("data = %s", data)
	}
}

func TestDecodeGraphQLResultJSONInvalido(t *testing.T) {
	_, err := decodeGraphQLResult([]byte("nao e' json"))
	if err == nil || !strings.Contains(err.Error(), "failed to unmarshal graphql response") {
		t.Fatalf("err = %v", err)
	}
}

// Erro GraphQL devolve o "data" PARCIAL junto com o erro: GetInfo e
// GetSubscribed dependem disso para preferir o erro do servidor.
func TestDecodeGraphQLResultErroDevolveDataParcial(t *testing.T) {
	data, err := decodeGraphQLResult([]byte(`{"data":{"parcial":true},"errors":[{"message":"nope","extensions":{"error_code":42,"severity":"CRITICAL"}}]}`))
	if err == nil || !strings.Contains(err.Error(), "graphql error") {
		t.Fatalf("err = %v, esperava erro graphql", err)
	}
	if string(data) != `{"parcial":true}` {
		t.Errorf("data = %s, esperava o data parcial", data)
	}
}

func TestDecodeGraphQLResultSucesso(t *testing.T) {
	data, err := decodeGraphQLResult([]byte(`{"data":{"x":1}}`))
	if err != nil {
		t.Fatalf("decodeGraphQLResult: %v", err)
	}
	var got map[string]int
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["x"] != 1 {
		t.Errorf("data = %s", data)
	}
}

// decodeArgoResult tem um early return incondicional enquanto o decoder Argo
// estiver quebrado; o resto do corpo e' inalcancavel hoje. O teste trava o
// estado atual para que a reabilitacao seja deliberada.
func TestDecodeArgoResultSempreRecusa(t *testing.T) {
	data, err := decodeArgoResult(newFakeTransport(), queryFetchNewsletterDesktop, []byte("x"))
	if data != nil {
		t.Errorf("esperava data nil, veio %s", data)
	}
	if !errors.Is(err, ErrArgoDecodingBroken) {
		t.Fatalf("err = %v, esperava ErrArgoDecodingBroken", err)
	}
}
