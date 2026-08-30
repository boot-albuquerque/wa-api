package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Este arquivo cobre POST /chat/send/document desde a migração do CAP-04
// para port.MediaMessenger/port.MediaFetcher — o handler que efetivamente
// busca a URL ou decodifica a data URI, sobe o anexo e entrega o documento
// ao noise, e não mais um "validated" sem fazer nada (o stub que este
// arquivo substitui). Mesma estrutura de handler_media_send_test.go
// (POST /chat/send/image, CAP-02/CAP-03).

const sendDocumentSentinelToken = "send-document-sentinel-cause-9f3e2b"

var errSendDocumentSentinel = errors.New(sendDocumentSentinelToken)

const sendDocumentTestURL = "https://exemplo.com/relatorio.pdf"

var sendDocumentPDFBytes = []byte("%PDF-1.4\nconteudo de teste\n")

// defaultSendDocumentFetcher devolve um MediaFetcher fake que já resolve
// com bytes válidos — o caminho neutro para testes que não exercitam o
// próprio fetch.
func defaultSendDocumentFetcher() *contractsfake.MediaFetcher {
	return &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return sendDocumentPDFBytes, "application/pdf", nil
		},
	}
}

// sendDocumentRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendDocumentRouter(mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher) http.Handler {
	uc := message.NewSendDocumentUseCase(mm, jr, mf, silentLogger{})
	h := NewSendDocumentHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/document", h).Methods(http.MethodPost)
	return r
}

func sendDocumentServe(t *testing.T, mm *contractsfake.MediaMessenger, jr *contractsfake.JIDResolver, mf *contractsfake.MediaFetcher, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/document", strings.NewReader(body))
	sendDocumentRouter(mm, jr, mf).ServeHTTP(rec, mut(req))
	return rec
}

type sendDocumentResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// TestSendDocument_Success_ViaRegisteredRoute prova o caminho HTTP ->
// handler -> usecase -> MediaFetcher.FetchBytes -> MediaMessenger.SendDocument
// pela rota gorilla/mux REGISTRADA (transporte URL), com Status="sent",
// MessageID igual ao devolvido pela porta (não ao pedido), Timestamp vindo
// do envio real e FileName repassado.
func TestSendDocument_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500060)
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("target: got %q", target)
			}
			if string(payload.Bytes) != string(sendDocumentPDFBytes) {
				t.Errorf("bytes: divergem dos buscados")
			}
			if payload.Caption != "legenda" {
				t.Errorf("caption: got %q", payload.Caption)
			}
			if payload.FileName != "relatorio.pdf" {
				t.Errorf("filename: got %q", payload.FileName)
			}
			return domain.MessageSendResult{ID: "wire-id-doc-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendDocumentFetcher()

	body := `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"relatorio.pdf","caption":"legenda"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendDocumentResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-doc-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-doc-999")
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("fetch chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if got := mf.FetchBytesCalls[0].ResourceURL; got != sendDocumentTestURL {
		t.Errorf("URL buscada: got %q, want %q", got, sendDocumentTestURL)
	}
	if n := len(mm.SendDocumentCalls); n != 1 {
		t.Fatalf("SendDocument chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

// TestSendDocument_RejectUnauthenticated: sem userInfo no contexto é 401 e
// nem fetch nem SendDocument são tocados.
func TestSendDocument_RejectUnauthenticated(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendDocumentFetcher()

	body := `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou o fetch %d vez(es)", n)
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendDocument %d vez(es)", n)
	}
}

// TestSendDocument_RejectMissingRequiredField: Phone, Document ou FileName
// ausente é 400 e nem fetch nem SendDocument são tocados. FileName é o
// campo que Document tem e Image não — o 400 específico do CAP-04.
func TestSendDocument_RejectMissingRequiredField(t *testing.T) {
	bodies := map[string]string{
		"phone":     `{"document":"` + sendDocumentTestURL + `","file_name":"a.pdf"}`,
		"document":  `{"phone":"5511999999999","file_name":"a.pdf"}`,
		"file_name": `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `"}`,
	}
	for field, body := range bodies {
		t.Run(field, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendDocumentFetcher()

			rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
			}
			if field == "FileName" && rec.Code != http.StatusBadRequest {
				t.Errorf("FileName ausente: status = %d, want 400", rec.Code)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("payload invalido, mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendDocumentCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendDocument foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendDocument_DataURI_Success_ViaRegisteredRoute prova o SEGUNDO
// transporte de POST /chat/send/document (data URI) pela mesma superfície
// de rota do ramo URL: data URI chega decodificada localmente ao
// MediaMessenger — MediaFetcher nunca é tocado — com Status="sent" e
// MessageID vindo da porta de envio. Usa MIME genérico (application/pdf),
// não "data:image" — a discriminação de Document é "data:" genérico.
func TestSendDocument_DataURI_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500070)
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(_ context.Context, _ string, target domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("target: got %q", target)
			}
			if payload.MimeType != "application/pdf" {
				t.Errorf("mimetype: got %q, want application/pdf", payload.MimeType)
			}
			return domain.MessageSendResult{ID: "wire-id-doc-datauri-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendDocumentFetcher()

	encoded := base64.StdEncoding.EncodeToString(sendDocumentPDFBytes)
	body := `{"phone":"5511999999999","document":"data:application/pdf;base64,` + encoded + `","file_name":"a.pdf","mime_type":"application/pdf"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendDocumentResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-doc-datauri-999" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "wire-id-doc-datauri-999")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es) pela rota registrada", n)
	}
	if n := len(mm.SendDocumentCalls); n != 1 {
		t.Fatalf("SendDocument chamado %d vez(es) pela rota registrada, quero 1", n)
	}
	if string(mm.SendDocumentCalls[0].Payload.Bytes) != string(sendDocumentPDFBytes) {
		t.Error("bytes decodificados divergem dos originais")
	}
}

// TestSendDocument_UnsupportedSource_Rejected: fontes que não são nem URL
// http(s) nem data URI — incluindo base64 cru sem prefixo "data:" e scheme
// proibido — viram erro pela rota registrada, nunca o 200 "validated"
// falso que o stub anterior produzia.
func TestSendDocument_UnsupportedSource_Rejected(t *testing.T) {
	cases := map[string]string{
		"raw_base64_no_prefix": "JVBERi0xLjQK",
		"unsupported_scheme":   "ftp://exemplo.com/relatorio.pdf",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			jr := &contractsfake.JIDResolver{}
			mf := defaultSendDocumentFetcher()

			body := `{"phone":"5511999999999","document":"` + doc + `","file_name":"a.pdf"}`
			rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

			if rec.Code == http.StatusOK {
				t.Fatalf("fonte nao suportada %q produziu 200 falso: %s", doc, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if env.Success {
				t.Fatalf("envelope.success=true para fonte nao suportada %q: %s", doc, rec.Body.String())
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas o fetch foi chamado %d vez(es)", doc, n)
			}
			if n := len(mm.SendDocumentCalls); n != 0 {
				t.Fatalf("fonte nao suportada %q, mas SendDocument foi chamado %d vez(es)", doc, n)
			}
		})
	}
}

// TestSendDocument_SessionFailure: sessão inexistente vira erro (não 200) e
// nem fetch nem SendDocument são chamados.
func TestSendDocument_SessionFailure(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSendDocumentSentinel)}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendDocumentFetcher()

	body := `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("sessao invalida, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendDocument foi chamado %d vez(es)", n)
	}
}

// TestSendDocument_InvalidPhoneNeverFetchesOrSends: JID que não resolve
// nunca vira 200 nem toca o fetch/SendDocument.
func TestSendDocument_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := defaultSendDocumentFetcher()

	body := `{"phone":"lixo","document":"` + sendDocumentTestURL + `","file_name":"a.pdf"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendDocument foi chamado %d vez(es)", n)
	}
}

// TestSendDocument_FetchFailure_NeverReturns200: falha ao buscar a URL
// (SSRF bloqueado, timeout, 404, corpo vazio, acima do limite, etc.) nunca
// produz 200 nem Status=sent.
func TestSendDocument_FetchFailure_NeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}

	body := `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no fetch produziu 200: %s", rec.Body.String())
	}
	if n := len(mm.SendDocumentCalls); n != 0 {
		t.Fatalf("fetch falhou, mas SendDocument foi chamado %d vez(es)", n)
	}
}

// TestSendDocument_DownstreamFailureNeverReturns200: SendDocument (upload
// e/ou envio) falhando NUNCA produz 200 nem Status=sent — a garantia
// central do CAP-04 contra falso-sucesso, pela rota registrada.
func TestSendDocument_DownstreamFailureNeverReturns200(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(context.Context, string, domain.JID, domain.MediaPayload, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendDocumentSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendDocumentFetcher()

	body := `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

// TestSendDocument_ClientSuppliedIDIsForwardedButServerIDWins: o Id do
// cliente é repassado a SendDocument, mas o message_id da resposta é o que
// a porta devolveu — nunca o do request usado às cegas.
func TestSendDocument_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.MediaPayload, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendDocumentFetcher()

	body := `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.pdf","id":"id-do-cliente"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendDocumentResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendDocument_MimeType_RemoteContentTypeUsedWhenReqEmpty: pela rota
// registrada, sem MimeType no request, o Content-Type declarado pelo
// servidor remoto (ramo URL) alimenta o Mimetype final — o degrau de
// precedência extra que Document tem sobre Image.
func TestSendDocument_MimeType_RemoteContentTypeUsedWhenReqEmpty(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendDocumentFunc: func(_ context.Context, _ string, _ domain.JID, payload domain.MediaPayload, _ *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			if payload.MimeType != "application/zip" {
				t.Errorf("mimetype: got %q, want %q (Content-Type remoto)", payload.MimeType, "application/zip")
			}
			return domain.MessageSendResult{ID: "wire-id-mime-remote"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return sendDocumentPDFBytes, "application/zip", nil
		},
	}

	body := `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"a.zip"}`
	rec := sendDocumentServe(t, mm, jr, mf, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(mm.SendDocumentCalls); n != 1 {
		t.Fatalf("SendDocument chamado %d vez(es), quero 1", n)
	}
}

// TestSendDocument_NoSecretLeak restaura o eixo perdido pela delecao de
// handler_media_test.go (a tabela sobre port.MessageComposer ficou vazia
// quando CAP-07 migrou o ultimo handler para longe dela — ver
// HOUSEKEEP.md). Phone, o campo Document e o header Authorization carregam
// os tres segredos da F9.4 (a sessao usa um id NAO secreto, de proposito:
// em producao ele nunca coincide com um dos tres); a sessao falha e o log
// de saida da rota REGISTRADA nao pode carregar nenhum dos tres.
func TestSendDocument_NoSecretLeak(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-document-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}
	mf := defaultSendDocumentFetcher()

	wrapped, capture := logassert.Wrap(sendDocumentRouter(mm, jr, mf))

	body := `{"phone":"` + logassertGlobalHMACKey + `","document":"` + logassertGlobalEncryptionKey + `","file_name":"leak-test.pdf"}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/document", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}
