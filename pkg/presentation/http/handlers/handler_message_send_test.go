package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Este arquivo cobre POST /chat/send/text (SendMessage) desde a migração do
// CAP-01 para port.TextMessenger — o handler que efetivamente entrega texto
// ao wa-noise, e não mais um "validated" sem envio.

const sendTextSentinelToken = "send-text-sentinel-cause-9f8e7d"

var errSendTextSentinel = errors.New(sendTextSentinelToken)

// sendTextRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto. ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada.
func sendTextRouter(tm *contractsfake.TextMessenger, jr *contractsfake.JIDResolver) http.Handler {
	return sendTextRouterWithPreview(tm, jr, &contractsfake.LinkPreviewFetcher{})
}

// sendTextRouterWithPreview é sendTextRouter com o fetcher de link preview
// explícito — usado pelos testes de CAP-01.1 (LP-6) que precisam controlar
// o que port.LinkPreviewFetcher devolve pela rota REGISTRADA.
func sendTextRouterWithPreview(tm *contractsfake.TextMessenger, jr *contractsfake.JIDResolver, lpf *contractsfake.LinkPreviewFetcher) http.Handler {
	uc := message.NewSendMessageUseCase(tm, jr, lpf, silentLogger{})
	h := NewSendMessageHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/text", h).Methods(http.MethodPost)
	return r
}

func sendTextServe(t *testing.T, tm *contractsfake.TextMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/text", strings.NewReader(body))
	sendTextRouter(tm, jr).ServeHTTP(rec, mut(req))
	return rec
}

// sendTextServeWithPreview é sendTextServe com o fetcher de link preview
// explícito (LP-6).
func sendTextServeWithPreview(t *testing.T, tm *contractsfake.TextMessenger, jr *contractsfake.JIDResolver, lpf *contractsfake.LinkPreviewFetcher, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/text", strings.NewReader(body))
	sendTextRouterWithPreview(tm, jr, lpf).ServeHTTP(rec, mut(req))
	return rec
}

// TestSendText_Success_ViaRegisteredRoute prova o caminho HTTP -> handler ->
// usecase -> TextMessenger.SendText pela rota gorilla/mux REGISTRADA, com
// Status="sent", MessageID igual ao devolvido pela porta (não ao pedido) e
// Timestamp vindo de MessageSendResult.Timestamp.
func TestSendText_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500000)
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, target domain.JID, text string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("target: got %q", target)
			}
			if text != "ola" {
				t.Errorf("text: got %q", text)
			}
			return domain.MessageSendResult{
				ID:        "wire-id-999",
				Timestamp: time.Unix(sentAt, 0),
			}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendTextServe(t, tm, jr, `{"Phone":"5511999999999","Body":"ola"}`, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data struct {
		MessageID string `json:"message_id"`
		Timestamp int64  `json:"timestamp"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-999")
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(tm.SendTextCalls); n != 1 {
		t.Fatalf("SendText chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

// TestSendText_RejectUnauthenticated: sem userInfo no contexto é 401 e a
// porta não é tocada.
func TestSendText_RejectUnauthenticated(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendTextServe(t, tm, jr, `{"Phone":"5511999999999","Body":"ola"}`, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(tm.SendTextCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendText %d vez(es)", n)
	}
}

// TestSendText_RejectMissingRequiredField: Phone ou Body ausente é 400 e a
// porta não é tocada.
func TestSendText_RejectMissingRequiredField(t *testing.T) {
	bodies := map[string]string{
		"Phone": `{"Body":"ola"}`,
		"Body":  `{"Phone":"5511999999999"}`,
	}
	for field, body := range bodies {
		t.Run(field, func(t *testing.T) {
			tm := &contractsfake.TextMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec := sendTextServe(t, tm, jr, body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
			}
			if n := len(tm.SendTextCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendText foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendText_SessionFailure: sessão inexistente vira erro (não 200) e
// SendText nunca é chamado.
func TestSendText_SessionFailure(t *testing.T) {
	tm := &contractsfake.TextMessenger{SessionGuard: contractsfake.FailSession(errSendTextSentinel)}
	jr := &contractsfake.JIDResolver{}

	rec := sendTextServe(t, tm, jr, `{"Phone":"5511999999999","Body":"ola"}`, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	if n := len(tm.SendTextCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendText foi chamado %d vez(es)", n)
	}
}

// TestSendText_InvalidPhoneNeverSends: JID que não resolve nunca vira 200
// nem toca SendText — falso-sucesso de JID inválido é proibido pelo CAP-01.
func TestSendText_InvalidPhoneNeverSends(t *testing.T) {
	tm := &contractsfake.TextMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}

	rec := sendTextServe(t, tm, jr, `{"Phone":"lixo","Body":"ola"}`, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(tm.SendTextCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendText foi chamado %d vez(es)", n)
	}
}

// TestSendText_DownstreamFailureNeverReturns200: SendText falhando NUNCA
// produz 200 nem Status=sent — a garantia central do CAP-01 contra
// falso-sucesso.
func TestSendText_DownstreamFailureNeverReturns200(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(context.Context, string, domain.JID, string, *domain.LinkPreviewData, *domain.ReplyContext, []string, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendTextSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendTextServe(t, tm, jr, `{"Phone":"5511999999999","Body":"ola"}`, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

// TestSendText_ClientSuppliedIDIsForwardedButServerIDWins: o Id do cliente é
// repassado a SendText, mas o message_id da resposta é o que a porta
// devolveu de volta — nunca o do request usado às cegas.
func TestSendText_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendTextServe(t, tm, jr, `{"Phone":"5511999999999","Body":"ola","Id":"id-do-cliente"}`, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendText_LinkPreview_ViaRegisteredRoute (LP-6, CAP-01.1) prova o
// caminho HTTP -> handler -> usecase -> LinkPreviewFetcher -> SendText pela
// rota gorilla/mux REGISTRADA (não pelo handler cru — ARMADILHA 2): com
// LinkPreview=true, o preview resolvido pelo fetcher chega inteiro a
// SendText, e a resposta HTTP continua vindo do envio real (status="sent"),
// não da resolução do preview.
func TestSendText_LinkPreview_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500001)
	wantPreview := domain.LinkPreviewData{
		MatchedURL:    "https://exemplo.com/artigo",
		Title:         "Um Artigo",
		Description:   "Resumo do artigo",
		ThumbnailJPEG: []byte{0xFF, 0xD8, 0xFF},
	}
	var gotPreview *domain.LinkPreviewData
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, preview *domain.LinkPreviewData, _ *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			gotPreview = preview
			return domain.MessageSendResult{ID: "wire-id-preview-route", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	lpf := &contractsfake.LinkPreviewFetcher{
		FetchLinkPreviewFunc: func(_ context.Context, text string) (domain.LinkPreviewData, bool) {
			if text != "olha https://exemplo.com/artigo" {
				t.Errorf("texto repassado ao fetcher: got %q", text)
			}
			return wantPreview, true
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendTextServeWithPreview(t, tm, jr, lpf, `{"Phone":"5511999999999","Body":"olha https://exemplo.com/artigo","LinkPreview":true}`, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if gotPreview == nil {
		t.Fatal("SendText nao recebeu preview algum")
	}
	if !reflect.DeepEqual(*gotPreview, wantPreview) {
		t.Errorf("preview: got %+v, want %+v", *gotPreview, wantPreview)
	}

	var data struct {
		MessageID string `json:"message_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-preview-route" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "wire-id-preview-route")
	}
}

// TestSendText_NoSecretLeak restaura o eixo perdido pela delecao de
// handler_media_test.go (a tabela sobre port.MessageComposer ficou vazia
// quando CAP-07 migrou o ultimo handler para longe dela — ver
// HOUSEKEEP.md). Phone, o campo Body e o header Authorization carregam os
// tres segredos da F9.4 (a sessao usa um id NAO secreto, de proposito: em
// producao ele nunca coincide com um dos tres); a sessao falha e o log de
// saida da rota REGISTRADA nao pode carregar nenhum dos tres.
func TestSendText_NoSecretLeak(t *testing.T) {
	tm := &contractsfake.TextMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-text-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendTextRouter(tm, jr))

	body := `{"Phone":"` + logassertGlobalHMACKey + `","Body":"` + logassertGlobalEncryptionKey + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/text", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}

// --- CAP-46A: ReplyTo via registered route --------------------------------

// TestSendText_ReplyTo_ViaRegisteredRoute: ReplyTo in the JSON payload is
// forwarded through the full HTTP -> handler -> usecase -> port chain via
// the gorilla/mux registered route (ARMADILHA 2).
func TestSendText_ReplyTo_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500002)
	var gotReply *domain.ReplyContext
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			gotReply = replyTo
			return domain.MessageSendResult{ID: "wire-reply-route", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Body":"my reply","ReplyTo":{"StanzaId":"quoted-123","Participant":"5511888888888@s.whatsapp.net","QuotedText":"original msg"}}`
	rec := sendTextServe(t, tm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if gotReply == nil {
		t.Fatal("ReplyTo not forwarded through the registered route")
	}
	if gotReply.StanzaID != "quoted-123" {
		t.Errorf("StanzaID = %q, want %q", gotReply.StanzaID, "quoted-123")
	}
	if gotReply.Participant != "5511888888888@s.whatsapp.net" {
		t.Errorf("Participant = %q, want %q", gotReply.Participant, "5511888888888@s.whatsapp.net")
	}
	if gotReply.QuotedText != "original msg" {
		t.Errorf("QuotedText = %q, want %q", gotReply.QuotedText, "original msg")
	}
}

// TestSendText_WithoutReplyTo_ViaRegisteredRoute: without ReplyTo in JSON,
// SendText receives nil — anti-regression via the registered route.
func TestSendText_WithoutReplyTo_ViaRegisteredRoute(t *testing.T) {
	var gotReply *domain.ReplyContext
	tm := &contractsfake.TextMessenger{
		SendTextFunc: func(_ context.Context, _ string, _ domain.JID, _ string, _ *domain.LinkPreviewData, replyTo *domain.ReplyContext, _ []string, _ string) (domain.MessageSendResult, error) {
			gotReply = replyTo
			return domain.MessageSendResult{ID: "wire-no-reply"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendTextServe(t, tm, jr, `{"Phone":"5511999999999","Body":"plain msg"}`, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if gotReply != nil {
		t.Errorf("ReplyTo = %+v, want nil (no ReplyTo in request)", gotReply)
	}
}

// TestSendText_ErrorLogsOmitSessionID locks the F120 parity decision: the text
// handler must NOT stamp the caller's session id on its error logs, because
// the six media handlers do not, and a session id in a log is a correlation
// surface.
//
// This is NOT the F9.4 axis: a session id is not one of the three global
// secrets, and logassert.NoSecrets would not catch it. The field name is
// asserted directly, on purpose.
//
// Both error branches are exercised, because they were BOTH stamping it
// (handler_message_send.go, decode failure and use-case failure). Covering one
// would leave the other free — the same shape as the two write points in F163.
func TestSendText_ErrorLogsOmitSessionID(t *testing.T) {
	const sessionID = "f120-session-id"

	cases := []struct {
		name string
		tm   *contractsfake.TextMessenger
		body string
	}{
		{
			name: "decode failure",
			tm:   &contractsfake.TextMessenger{},
			body: `{not json`,
		},
		{
			name: "use case failure",
			tm:   &contractsfake.TextMessenger{SessionGuard: contractsfake.FailSession(errors.New("f120-cause"))},
			body: `{"Phone":"5511999999999","Body":"ola"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped, capture := logassert.Wrap(sendTextRouter(tc.tm, &contractsfake.JIDResolver{}))

			req := httptest.NewRequest(http.MethodPost, "/chat/send/text", strings.NewReader(tc.body))
			req = withUser(req, sessionID)

			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			if rec.Code < 400 {
				t.Fatalf("status = %d, want an error status (body %s)", rec.Code, rec.Body.String())
			}

			for _, r := range capture.Records(t) {
				if _, present := r.Fields["user_id"]; present {
					t.Fatalf("field user_id is back on the text handler error log (F120); the six media handlers do not stamp it. record: %s", r.Raw)
				}
				if strings.Contains(r.Raw, sessionID) {
					t.Fatalf("session id leaked into the error log under another field name (F120). record: %s", r.Raw)
				}
			}
		})
	}
}
