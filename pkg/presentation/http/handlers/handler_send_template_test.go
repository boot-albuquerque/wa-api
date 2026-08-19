package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// Este arquivo cobre POST /chat/send/template desde a migração do CAP-15 para
// port.SimpleMessenger — o handler que efetivamente monta o TemplateMessage e
// o envia pelo wa-noise, e não mais um "validated" sem fazer nada. Mesma
// estrutura de handler_send_poll_test.go (rota gorilla/mux registrada).
//
// O eixo próprio deste bloco é `Buttons`: o DTO tinha perdido o campo que dá
// sentido à capability (HOUSEKEEP F139), então a rota tem de recusar o corpo
// sem botão e tem de entregar os botões INTEIROS à porta. Qual protobuf cada
// tipo vira está medido no adapter
// (pkg/infra/wa-noise/adapters/chat/messenger_template_test.go).

const sendTemplateSentinelToken = "send-template-sentinel-cause-7c31d5"

const sendTemplatePhone = "5511999999999"

// sendTemplateBody é o menor corpo VÁLIDO da rota: um botão já basta.
const sendTemplateBody = `{"Phone":"` + sendTemplatePhone + `","Content":"Escolha","Footer":"Equipe",` +
	`"Buttons":[{"DisplayText":"Sim","Type":"quickreply"}]}`

var errSendTemplateSentinel = errors.New(sendTemplateSentinelToken)

// sendTemplateRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendTemplateRouter(sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver) http.Handler {
	uc := message.NewSendTemplateUseCase(sm, jr, silentLogger{})
	h := NewSendTemplateHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/template", h).Methods(http.MethodPost)
	return r
}

func sendTemplateServe(t *testing.T, sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/template", strings.NewReader(body))
	sendTemplateRouter(sm, jr).ServeHTTP(rec, mut(req))
	return rec
}

// sendTemplateServeCapturingLog é o sendTemplateServe com a saída de log da
// requisição — a mesma cadeia hlog que router.go instala, via logassert.Wrap.
// Existe para que os caminhos de erro possam asseverar a CAUSA (co-gate D), e
// não só o status: o envelope de erro deste repo é o genérico "bad request",
// então duas rejeições diferentes com o mesmo status são indistinguíveis pelo
// corpo.
func sendTemplateServeCapturingLog(t *testing.T, sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(sendTemplateRouter(sm, jr))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/template", strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

type sendTemplateResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// TestSendTemplate_Success_ViaRegisteredRoute prova o caminho HTTP -> handler
// -> usecase -> SimpleMessenger.SendTemplate pela rota gorilla/mux
// REGISTRADA, com Status="sent", MessageID igual ao devolvido pela porta e
// Timestamp vindo do envio real. Os TRÊS tipos de botão atravessam a
// fronteira HTTP aqui, na ordem, com os campos que cada um usa.
func TestSendTemplate_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500131)
	body := `{"Phone":"` + sendTemplatePhone + `","Content":"Escolha uma opcao","Footer":"Equipe wa-api","Buttons":[` +
		`{"DisplayText":"Sim","Id":"cta-42","Type":"quickreply"},` +
		`{"DisplayText":"Site","Url":"https://example.invalid/promo","Type":"url"},` +
		`{"DisplayText":"Ligar","PhoneNumber":"+5511987654321","Type":"call"}]}`

	sm := &contractsfake.SimpleMessenger{
		SendTemplateFunc: func(_ context.Context, _ string, target domain.JID, payload domain.TemplatePayload, _ string) (domain.MessageSendResult, error) {
			if target != domain.JID(sendTemplatePhone) {
				t.Errorf("target: got %q, want %q", target, sendTemplatePhone)
			}
			if payload.Content != "Escolha uma opcao" {
				t.Errorf("Content: got %q, want o Content do corpo", payload.Content)
			}
			if payload.Footer != "Equipe wa-api" {
				t.Errorf("Footer: got %q, want o Footer do corpo", payload.Footer)
			}
			want := []domain.TemplateButton{
				{DisplayText: "Sim", ID: "cta-42", Type: domain.TemplateButtonQuickReply},
				{DisplayText: "Site", URL: "https://example.invalid/promo", Type: domain.TemplateButtonURL},
				{DisplayText: "Ligar", PhoneNumber: "+5511987654321", Type: domain.TemplateButtonCall},
			}
			if len(payload.Buttons) != len(want) {
				t.Fatalf("Buttons: got %d, want %d", len(payload.Buttons), len(want))
			}
			for i := range want {
				if payload.Buttons[i] != want[i] {
					t.Errorf("Buttons[%d]: got %+v, want %+v (a ORDEM importa)", i, payload.Buttons[i], want[i])
				}
			}
			return domain.MessageSendResult{ID: "wire-id-template-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendTemplateServe(t, sm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendTemplateResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-template-999" {
		t.Errorf("message_id: got %q, want o que a porta devolveu", data.MessageID)
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(sm.SendTemplateCalls); n != 1 {
		t.Fatalf("SendTemplate chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

func TestSendTemplate_RejectUnauthenticated(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendTemplateServe(t, sm, jr, sendTemplateBody, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendTemplate %d vez(es)", n)
	}
}

// TestSendTemplate_RejectMissingRequiredField cobre as QUATRO recusas de
// validação do histórico pela rota registrada, cada uma pela sua CAUSA no
// log: o status sozinho não distingue "faltou Phone" de "faltou Content" de
// "faltou Footer" de "nenhum botão".
func TestSendTemplate_RejectMissingRequiredField(t *testing.T) {
	const buttons = `,"Buttons":[{"DisplayText":"Sim","Type":"quickreply"}]`

	cases := map[string]struct{ body, cause string }{
		"Phone":           {`{"Content":"c","Footer":"f"` + buttons + `}`, "missing Phone in payload"},
		"Content":         {`{"Phone":"` + sendTemplatePhone + `","Footer":"f"` + buttons + `}`, "missing Content in payload"},
		"Footer":          {`{"Phone":"` + sendTemplatePhone + `","Content":"c"` + buttons + `}`, "missing Footer in payload"},
		"Buttons_ausente": {`{"Phone":"` + sendTemplatePhone + `","Content":"c","Footer":"f"}`, "missing Buttons in payload"},
		"Buttons_vazio":   {`{"Phone":"` + sendTemplatePhone + `","Content":"c","Footer":"f","Buttons":[]}`, "missing Buttons in payload"},
	}
	for field, tc := range cases {
		t.Run(field, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec, recs := sendTemplateServeCapturingLog(t, sm, jr, tc.body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload invalido (%s) produziu status de sucesso %d", field, rec.Code)
			}
			logassert.OutcomeLogged(t, recs, tc.cause)
			if n := len(sm.SendTemplateCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendTemplate foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendTemplate_SessionFailure(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSendTemplateSentinel)}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendTemplateServeCapturingLog(t, sm, jr, sendTemplateBody, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	// A CAUSA da porta tem de chegar ao log — o token sentinela é único
	// neste arquivo, então nenhuma outra falha o produziria por acidente.
	logassert.OutcomeLogged(t, recs, sendTemplateSentinelToken)
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendTemplate foi chamado %d vez(es)", n)
	}
}

func TestSendTemplate_InvalidPhoneNeverSends(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}

	body := `{"Phone":"lixo","Content":"c","Footer":"f","Buttons":[{"DisplayText":"Sim","Type":"quickreply"}]}`
	rec := sendTemplateServe(t, sm, jr, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendTemplate foi chamado %d vez(es)", n)
	}
}

// TestSendTemplate_DownstreamFailureNeverReturns200 é o eixo do defeito
// original: a rota devolvia 200 sem enviar nada. Aqui o envio FALHA, e um 200
// seria a mentira de volta.
func TestSendTemplate_DownstreamFailureNeverReturns200(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendTemplateFunc: func(context.Context, string, domain.JID, domain.TemplatePayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendTemplateSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendTemplateServe(t, sm, jr, sendTemplateBody, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendTemplate_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendTemplateFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.TemplatePayload, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"` + sendTemplatePhone + `","Content":"c","Footer":"f","Id":"id-do-cliente",` +
		`"Buttons":[{"DisplayText":"Sim","Type":"quickreply"}]}`
	rec := sendTemplateServe(t, sm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendTemplateResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendTemplate_NoSecretLeak segue o mesmo padrão de
// TestSendPoll_NoSecretLeak: o Content, o Footer e o texto dos botões e o
// header Authorization carregam segredos da F9.4 (a sessão usa um id NÃO
// secreto, de propósito); a sessão falha e o log de saída da rota REGISTRADA
// não pode carregar nenhum deles.
func TestSendTemplate_NoSecretLeak(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-template-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendTemplateRouter(sm, jr))

	body := `{"Phone":"` + sendTemplatePhone + `","Content":"` + logassertGlobalHMACKey + `","Footer":"` +
		logassertGlobalHMACKey + `","Buttons":[{"DisplayText":"` + logassertGlobalHMACKey + `","Type":"quickreply"}]}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/template", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}

// TestSendTemplate_MalformedBody_ViaRegisteredRoute recupera o eixo CORPO
// MALFORMADO que a tabela de handler_message_test.go cobria antes da migração
// do CAP-15. Requisição AUTENTICADA de propósito: sem isso o 401 mascara o
// 400 e o teste não mede o decode.
//
// A segunda requisição trava a CAUSA, não só o sintoma: o envelope de erro é
// o genérico "bad request", e o payload truncado também produziria 400 se o
// decode fosse ignorado (o use case rejeitaria por missing_phone). Só o
// registro de saída distingue os dois.
//
// A causa asseverada é "unexpected EOF", e não "could not decode payload"
// como em handler_send_poll_test.go, porque ESTE handler loga o erro CRU do
// decoder (handler_message_template.go, `Err(err)`) enquanto os handlers de
// handler_interactive.go logam o erro-sentinela genérico
// (`Err(errDecodePayload)`). A diferença é anterior ao CAP-15 e é a forma
// MAIS operável das duas: o erro cru diz onde o JSON quebrou. Registrada em
// HOUSEKEEP F141, sem correção nesta sessão.
func TestSendTemplate_MalformedBody_ViaRegisteredRoute(t *testing.T) {
	const malformed = `{"Phone":"55119`

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendTemplateServe(t, sm, jr, malformed, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendTemplate %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendTemplateRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, msgAuthed(httptest.NewRequest(http.MethodPost, "/chat/send/template", strings.NewReader(malformed))))

	if logRec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unexpected EOF")
	if n := len(smLog.SendTemplateCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendTemplate %d vez(es)", n)
	}
}

// TestSendTemplate_MalformedButtons_ViaRegisteredRoute é o eixo próprio deste
// bloco: `Buttons` com o tipo errado no JSON (objeto onde o schema pede
// lista) é 400 do cliente, não pânico e não 200. É a forma de corpo que o DTO
// só passou a poder receber no CAP-15.
func TestSendTemplate_MalformedButtons_ViaRegisteredRoute(t *testing.T) {
	const body = `{"Phone":"` + sendTemplatePhone + `","Content":"c","Footer":"f","Buttons":{"DisplayText":"Sim"}}`

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendTemplateServeCapturingLog(t, sm, jr, body, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	// A causa nomeia o CAMPO: sem `Buttons` no DTO o decoder nem chegaria a
	// reclamar dele, e o corpo seria aceito com a lista simplesmente
	// ausente — que é exatamente o estado anterior ao CAP-15 (F139).
	logassert.OutcomeLogged(t, recs, "cannot unmarshal object", "SendTemplateRequest.Buttons")
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("Buttons malformado alcancou SendTemplate %d vez(es)", n)
	}
}

// TestSendTemplate_MissingSessionID_ViaRegisteredRoute recupera o eixo
// MISSING SESSION ID. A requisição é AUTENTICADA — userinfo presente no
// contexto — mas com o `Id` VAZIO: é 400 do cliente, não 401, e a distinção
// entre os dois é o ponto do teste. O corpo é VÁLIDO de propósito: se a
// guarda não disparar, nada mais impede o envio, e a porta é alcançada.
func TestSendTemplate_MissingSessionID_ViaRegisteredRoute(t *testing.T) {
	noSessionID := func(r *http.Request) *http.Request { return withUser(r, "") }

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendTemplateServe(t, sm, jr, sendTemplateBody, noSessionID)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendTemplate %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendTemplateRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, noSessionID(httptest.NewRequest(http.MethodPost, "/chat/send/template", strings.NewReader(sendTemplateBody))))

	if logRec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "missing session id")
	if n := len(smLog.SendTemplateCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendTemplate %d vez(es)", n)
	}
}

// TestSendTemplate_WrongTypeInContext_ViaRegisteredRoute recupera o eixo
// WRONG TYPE IN CONTEXT. A chave do contexto é tipada, mas o VALOR é `any`:
// um valor que não satisfaz userInfo tem de virar 401, não pânico.
func TestSendTemplate_WrongTypeInContext_ViaRegisteredRoute(t *testing.T) {
	wrongType := func(r *http.Request) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
	}

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendTemplateServe(t, sm, jr, sendTemplateBody, wrongType)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(sm.SendTemplateCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendTemplate %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendTemplateRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, wrongType(httptest.NewRequest(http.MethodPost, "/chat/send/template", strings.NewReader(sendTemplateBody))))

	if logRec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unauthorized")
	if n := len(smLog.SendTemplateCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendTemplate %d vez(es)", n)
	}
}

// TestSendTemplate_SuccessEmitsNoOutcomeLog recupera o eixo AUSÊNCIA DE LOG
// NO CAMINHO FELIZ. É o eixo que os outros não pegam: um handler que logasse
// TODO request em warn passaria em cada asserção de caminho de erro deste
// arquivo e ainda assim seria o ruído que a Fase 12 existe para evitar. O
// logger do use case é silentLogger{}, então o que se mede aqui é só o
// registro do HANDLER, pela mesma cadeia hlog que router.go instala.
func TestSendTemplate_SuccessEmitsNoOutcomeLog(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendTemplateRouter(sm, jr))
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, msgAuthed(httptest.NewRequest(http.MethodPost, "/chat/send/template", strings.NewReader(sendTemplateBody))))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(sm.SendTemplateCalls); n != 1 {
		t.Fatalf("SendTemplate chamado %d vez(es), quero 1", n)
	}
	// A asserção é por NÍVEL do registro, e não por presença do campo
	// "error": um Warn de ruído no caminho feliz não traz "error" nenhum, e
	// has("error") o deixaria passar. A forma forte é a da tabela original de
	// handler_message_test.go, e o FIX-10 já pagou esta lição uma vez.
	for _, r := range capture.Records(t) {
		if lvl := r.str("level"); lvl == "warn" || lvl == "error" {
			t.Fatalf("caminho de sucesso emitiu registro %s: %s", lvl, r.Raw)
		}
	}
}

// TestSendTemplate_AuthenticatedSessionReachesPort recupera o eixo do txtID
// que a tabela de handler_message_test.go travava
// (`EnsureSessionCalls[0].TxtID`) e que não veio junto na migração do CAP-15.
// Sem ele, trocar `txtID` por qualquer outra coisa no handler deixa a suíte
// inteira verde — e uma mensagem sairia pela SESSÃO ERRADA sem que nada
// acuse.
//
// O id da sessão é um sentinela DIFERENTE do "user-1" de msgAuthed de
// propósito: com "user-1" um handler que ignorasse o contexto e usasse uma
// constante ainda passaria. Aqui só passa quem lê o contexto autenticado.
//
// Os DOIS lados são asseverados, porque são chamadas distintas da porta e um
// defeito pode atingir só uma: a guarda de sessão (EnsureSession) e o envio
// (SendTemplate) têm de receber o MESMO id, o do contexto.
func TestSendTemplate_AuthenticatedSessionReachesPort(t *testing.T) {
	const sessionID = "send-template-session-3f9a1c"

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendTemplateServe(t, sm, jr, sendTemplateBody, func(r *http.Request) *http.Request {
		return withUser(r, sessionID)
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(sm.EnsureSessionCalls); n != 1 {
		t.Fatalf("EnsureSession chamado %d vez(es), quero 1", n)
	}
	if got := sm.EnsureSessionCalls[0].TxtID; got != sessionID {
		t.Errorf("EnsureSession recebeu txtID %q, quero %q (o do contexto autenticado)", got, sessionID)
	}
	if n := len(sm.SendTemplateCalls); n != 1 {
		t.Fatalf("SendTemplate chamado %d vez(es), quero 1", n)
	}
	if got := sm.SendTemplateCalls[0].TxtID; got != sessionID {
		t.Errorf("SendTemplate recebeu txtID %q, quero %q (o do contexto autenticado)", got, sessionID)
	}
}
