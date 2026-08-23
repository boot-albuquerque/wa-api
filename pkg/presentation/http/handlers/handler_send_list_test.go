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

// Este arquivo cobre POST /chat/send/list desde a migração do CAP-22 para
// port.SimpleMessenger — o handler que efetivamente monta o ListMessage e o
// envia pelo wa-noise, e não mais um "validated" sem fazer nada. Mesma
// estrutura de handler_send_buttons_test.go/handler_send_template_test.go
// (rota gorilla/mux registrada). É o ÚLTIMO stub da superfície de envio
// (HOUSEKEEP F149).

const sendListSentinelToken = "send-list-sentinel-cause-7f31be"

const sendListPhone = "5511999999999@s.whatsapp.net"

// sendListBody é o menor corpo VÁLIDO da rota: uma seção com uma linha já
// basta.
const sendListBody = `{"Phone":"` + sendListPhone + `","Desc":"Escolha",` +
	`"Sections":[{"title":"Cardapio","rows":[{"title":"Item 1"}]}]}`

var errSendListSentinel = errors.New(sendListSentinelToken)

// sendListRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendListRouter(sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver) http.Handler {
	uc := message.NewSendListUseCase(sm, jr, silentLogger{})
	h := NewSendListHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/list", h).Methods(http.MethodPost)
	return r
}

func sendListServe(t *testing.T, sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/list", strings.NewReader(body))
	sendListRouter(sm, jr).ServeHTTP(rec, mut(req))
	return rec
}

// sendListServeCapturingLog é o sendListServe com a saída de log da
// requisição, para asseverar a CAUSA (co-gate D), e não só o status.
func sendListServeCapturingLog(t *testing.T, sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(sendListRouter(sm, jr))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/list", strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

type sendListResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// TestSendList_Success_ViaRegisteredRoute prova o caminho HTTP -> handler ->
// usecase -> SimpleMessenger.SendList pela rota gorilla/mux REGISTRADA, com
// Status="sent", MessageID igual ao devolvido pela porta e Timestamp vindo
// do envio real.
func TestSendList_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500141)
	body := `{"Phone":"` + sendListPhone + `","Desc":"Escolha um prato","ButtonText":"Ver cardapio",` +
		`"TopText":"Cabecalho","FooterText":"Equipe wa-api","Sections":[` +
		`{"title":"Pratos","rows":[{"title":"Feijoada","desc":"com torresmo","RowId":"feijoada"}]}]}`

	sm := &contractsfake.SimpleMessenger{
		SendListFunc: func(_ context.Context, _ string, target domain.JID, payload domain.ListPayload, _ *domain.ReplyContext, _ string) (domain.MessageSendResult, error) {
			if target != domain.JID(sendListPhone) {
				t.Errorf("target: got %q, want %q", target, sendListPhone)
			}
			if payload.Body != "Escolha um prato" {
				t.Errorf("Body: got %q, want o Desc do corpo", payload.Body)
			}
			if payload.ButtonText != "Ver cardapio" {
				t.Errorf("ButtonText: got %q", payload.ButtonText)
			}
			if payload.Title != "Cabecalho" {
				t.Errorf("Title: got %q, want o TopText do corpo", payload.Title)
			}
			if payload.Footer != "Equipe wa-api" {
				t.Errorf("Footer: got %q, want o FooterText do corpo", payload.Footer)
			}
			want := []domain.ListSection{
				{Title: "Pratos", Rows: []domain.ListRow{{Title: "Feijoada", Description: "com torresmo", RowId: "feijoada"}}},
			}
			if len(payload.Sections) != len(want) {
				t.Fatalf("Sections: got %d, want %d", len(payload.Sections), len(want))
			}
			for i := range want {
				if payload.Sections[i].Title != want[i].Title {
					t.Errorf("Sections[%d].Title: got %+v, want %+v", i, payload.Sections[i], want[i])
				}
				if len(payload.Sections[i].Rows) != len(want[i].Rows) || payload.Sections[i].Rows[0] != want[i].Rows[0] {
					t.Errorf("Sections[%d].Rows: got %+v, want %+v (a ORDEM importa)", i, payload.Sections[i].Rows, want[i].Rows)
				}
			}
			return domain.MessageSendResult{ID: "wire-id-list-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendListServe(t, sm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendListResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-list-999" {
		t.Errorf("message_id: got %q, want o que a porta devolveu", data.MessageID)
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(sm.SendListCalls); n != 1 {
		t.Fatalf("SendList chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

// TestSendList_RowWithoutTitleIsSilentlyDiscarded_ViaRegisteredRoute é a
// trava do PRIMEIRO descarte silencioso (HOUSEKEEP F149) medida na FRONTEIRA
// HTTP: o cliente manda TRÊS linhas, uma sem título, recebe 200 — e saem
// DUAS. Nada avisa.
func TestSendList_RowWithoutTitleIsSilentlyDiscarded_ViaRegisteredRoute(t *testing.T) {
	body := `{"Phone":"` + sendListPhone + `","Desc":"Escolha","Sections":[{"title":"Sec","rows":[` +
		`{"title":"Sobrevive 1"},{"title":"   "},{"title":"Sobrevive 2"}]}]}`

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendListServeCapturingLog(t, sm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (o descarte e' SILENCIOSO; corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(sm.SendListCalls); n != 1 {
		t.Fatalf("SendList chamado %d vez(es), quero 1", n)
	}
	rows := sm.SendListCalls[0].Payload.Sections[0].Rows
	if len(rows) != 2 {
		t.Fatalf("chegaram %d linha(s) a porta, quero 2 — a sem titulo SOME", len(rows))
	}
	if rows[0].Title != "Sobrevive 1" || rows[1].Title != "Sobrevive 2" {
		t.Errorf("sobraram %q e %q, quero \"Sobrevive 1\" e \"Sobrevive 2\"", rows[0].Title, rows[1].Title)
	}
	assertNoOutcomeLog(t, recs)
}

func TestSendList_RejectUnauthenticated(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendListServe(t, sm, jr, sendListBody, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendList %d vez(es)", n)
	}
}

// TestSendList_RejectMissingRequiredField cobre as recusas de validação do
// histórico pela rota registrada, cada uma pela sua CAUSA no log — DUAS
// causas distintas para Phone/Desc, mais a rede do terceiro descarte
// silencioso.
func TestSendList_RejectMissingRequiredField(t *testing.T) {
	const sections = `,"Sections":[{"title":"Sec","rows":[{"title":"Item"}]}]`

	cases := map[string]struct{ body, cause string }{
		"Phone":                    {`{"Desc":"Escolha"` + sections + `}`, "missing Phone in payload"},
		"Desc":                     {`{"Phone":"` + sendListPhone + `"` + sections + `}`, "missing Desc/Body in payload"},
		"Sections_e_List_ausentes": {`{"Phone":"` + sendListPhone + `","Desc":"Escolha"}`, "missing Sections (or List) in payload"},
		"Sections_e_List_vazios":   {`{"Phone":"` + sendListPhone + `","Desc":"Escolha","Sections":[],"List":[]}`, "missing Sections (or List) in payload"},
		"Sections_todas_descartadas": {
			`{"Phone":"` + sendListPhone + `","Desc":"Escolha","Sections":[{"title":"Vazia","rows":[{"title":"  "}]}]}`,
			"no valid sections/rows found in payload"},
	}
	for field, tc := range cases {
		t.Run(field, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec, recs := sendListServeCapturingLog(t, sm, jr, tc.body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload invalido (%s) produziu status de sucesso %d", field, rec.Code)
			}
			logassert.OutcomeLogged(t, recs, tc.cause)
			if n := len(sm.SendListCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendList foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendList_SessionFailure(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSendListSentinel)}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendListServeCapturingLog(t, sm, jr, sendListBody, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.OutcomeLogged(t, recs, sendListSentinelToken)
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendList foi chamado %d vez(es)", n)
	}
}

func TestSendList_InvalidPhoneNeverSends(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}

	body := `{"Phone":"lixo","Desc":"Escolha","Sections":[{"title":"Sec","rows":[{"title":"Item"}]}]}`
	rec := sendListServe(t, sm, jr, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendList foi chamado %d vez(es)", n)
	}
}

// TestSendList_DownstreamFailureNeverReturns200 é o eixo do defeito
// original: a rota devolvia 200 sem enviar nada. Aqui o envio FALHA, e um
// 200 seria a mentira de volta.
func TestSendList_DownstreamFailureNeverReturns200(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendListFunc: func(context.Context, string, domain.JID, domain.ListPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendListSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendListServe(t, sm, jr, sendListBody, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendList_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendListFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.ListPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"` + sendListPhone + `","Desc":"Escolha","Id":"id-do-cliente",` +
		`"Sections":[{"title":"Sec","rows":[{"title":"Item"}]}]}`
	rec := sendListServe(t, sm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendListResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendList_NoSecretLeak segue o mesmo padrão de
// TestSendButtons_NoSecretLeak: Desc, TopText, FooterText e o título da
// seção/linha carregam segredos da F9.4; a sessão falha e o log de saída da
// rota REGISTRADA não pode carregar nenhum deles.
func TestSendList_NoSecretLeak(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-list-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendListRouter(sm, jr))

	body := `{"Phone":"` + sendListPhone + `","Desc":"` + logassertGlobalHMACKey + `","TopText":"` +
		logassertGlobalHMACKey + `","FooterText":"` + logassertGlobalHMACKey +
		`","Sections":[{"title":"` + logassertGlobalHMACKey + `","rows":[{"title":"` + logassertGlobalHMACKey + `"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/list", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}

// TestSendList_MalformedBody_ViaRegisteredRoute recupera o eixo CORPO
// MALFORMADO que handler_message_test.go/handler_interactive_test.go
// cobriam antes da migração do CAP-22. Requisição AUTENTICADA de propósito.
//
// A causa asseverada é "unexpected EOF", o erro CRU do decoder — este
// handler fecha HOUSEKEEP F141 para a última rota que ainda logava o
// sentinela genérico.
func TestSendList_MalformedBody_ViaRegisteredRoute(t *testing.T) {
	const malformed = `{"Phone":"55119`

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendListServe(t, sm, jr, malformed, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendList %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	rec2, recs := sendListServeCapturingLog(t, smLog, jr, malformed, msgAuthed)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", rec2.Code, rec2.Body.String())
	}
	logassert.OutcomeLogged(t, recs, "unexpected EOF")
	if n := len(smLog.SendListCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendList %d vez(es)", n)
	}
}

// TestSendList_MalformedSections_ViaRegisteredRoute é o eixo próprio deste
// bloco: `Sections` com o tipo errado no JSON (objeto onde o schema pede
// lista) é 400 do cliente, não pânico e não 200.
func TestSendList_MalformedSections_ViaRegisteredRoute(t *testing.T) {
	const body = `{"Phone":"` + sendListPhone + `","Desc":"Escolha","Sections":{"title":"Sec"}}`

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendListServeCapturingLog(t, sm, jr, body, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	logassert.OutcomeLogged(t, recs, "cannot unmarshal object", "SendListRequest.Sections")
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("Sections malformado alcancou SendList %d vez(es)", n)
	}
}

// TestSendList_MissingSessionID_ViaRegisteredRoute recupera o eixo MISSING
// SESSION ID. A requisição é AUTENTICADA — userinfo presente no contexto —
// mas com o `Id` VAZIO: é 400 do cliente, não 401.
func TestSendList_MissingSessionID_ViaRegisteredRoute(t *testing.T) {
	noSessionID := func(r *http.Request) *http.Request { return withUser(r, "") }

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendListServe(t, sm, jr, sendListBody, noSessionID)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendList %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	rec2, recs := sendListServeCapturingLog(t, smLog, jr, sendListBody, noSessionID)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", rec2.Code, rec2.Body.String())
	}
	logassert.OutcomeLogged(t, recs, "missing session id")
	if n := len(smLog.SendListCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendList %d vez(es)", n)
	}
}

// TestSendList_WrongTypeInContext_ViaRegisteredRoute recupera o eixo WRONG
// TYPE IN CONTEXT. A chave do contexto é tipada, mas o VALOR é `any`: um
// valor que não satisfaz userInfo tem de virar 401, não pânico.
func TestSendList_WrongTypeInContext_ViaRegisteredRoute(t *testing.T) {
	wrongType := func(r *http.Request) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
	}

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendListServe(t, sm, jr, sendListBody, wrongType)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(sm.SendListCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendList %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	rec2, recs := sendListServeCapturingLog(t, smLog, jr, sendListBody, wrongType)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401 (corpo: %s)", rec2.Code, rec2.Body.String())
	}
	logassert.OutcomeLogged(t, recs, "unauthorized")
	if n := len(smLog.SendListCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendList %d vez(es)", n)
	}
}

// TestSendList_SuccessEmitsNoOutcomeLog recupera o eixo AUSÊNCIA DE LOG NO
// CAMINHO FELIZ. A asserção é por NÍVEL do registro (assertNoOutcomeLog).
func TestSendList_SuccessEmitsNoOutcomeLog(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendListServeCapturingLog(t, sm, jr, sendListBody, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(sm.SendListCalls); n != 1 {
		t.Fatalf("SendList chamado %d vez(es), quero 1", n)
	}
	assertNoOutcomeLog(t, recs)
}

// TestSendList_AuthenticatedSessionReachesPort recupera o eixo do txtID que
// handler_message_test.go travava — o eixo do CAP-16, sem o qual trocar
// `txtID` por qualquer outra coisa no handler deixa a suíte inteira verde.
func TestSendList_AuthenticatedSessionReachesPort(t *testing.T) {
	const sessionID = "send-list-session-9c1f42"

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendListServe(t, sm, jr, sendListBody, func(r *http.Request) *http.Request {
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
	if n := len(sm.SendListCalls); n != 1 {
		t.Fatalf("SendList chamado %d vez(es), quero 1", n)
	}
	if got := sm.SendListCalls[0].TxtID; got != sessionID {
		t.Errorf("SendList recebeu txtID %q, quero %q (o do contexto autenticado)", got, sessionID)
	}
}
