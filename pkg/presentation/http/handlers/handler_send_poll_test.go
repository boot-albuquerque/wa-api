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

// Este arquivo cobre POST /chat/send/poll desde a migração do CAP-14 para
// port.SimpleMessenger — o handler que efetivamente monta o
// PollCreationMessage e o envia pelo wa-noise, e não mais um "validated" sem
// fazer nada. Mesma estrutura de handler_send_location_test.go (rota
// gorilla/mux registrada).

const sendPollSentinelToken = "send-poll-sentinel-cause-4a91e2"

const sendPollGroup = "120363313346913103@g.us"

// sendPollBody é o menor corpo VÁLIDO da rota.
const sendPollBody = `{"Group":"` + sendPollGroup + `","Header":"Que horas almocamos?","Options":["12h","13h"]}`

var errSendPollSentinel = errors.New(sendPollSentinelToken)

// sendPollRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendPollRouter(sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver) http.Handler {
	uc := message.NewSendPollUseCase(sm, jr, silentLogger{})
	h := NewSendPollHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/poll", h).Methods(http.MethodPost)
	return r
}

func sendPollServe(t *testing.T, sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/poll", strings.NewReader(body))
	sendPollRouter(sm, jr).ServeHTTP(rec, mut(req))
	return rec
}

// sendPollServeCapturingLog e' o sendPollServe com a saida de log da
// requisicao — a mesma cadeia hlog que router.go instala, via logassert.Wrap.
// Existe para que os caminhos de erro possam asseverar a CAUSA (co-gate D), e
// nao so' o status: o envelope de erro deste repo e' o generico "bad request",
// entao duas rejeicoes diferentes com o mesmo status sao indistinguiveis pelo
// corpo.
func sendPollServeCapturingLog(t *testing.T, sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(sendPollRouter(sm, jr))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/poll", strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

type sendPollResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// TestSendPoll_Success_ViaRegisteredRoute prova o caminho HTTP -> handler ->
// usecase -> SimpleMessenger.SendPoll pela rota gorilla/mux REGISTRADA, com
// Status="sent", MessageID igual ao devolvido pela porta e Timestamp vindo do
// envio real.
func TestSendPoll_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500115)
	sm := &contractsfake.SimpleMessenger{
		SendPollFunc: func(_ context.Context, _ string, target domain.JID, payload domain.PollPayload, _ string) (domain.MessageSendResult, error) {
			if target != domain.JID(sendPollGroup) {
				t.Errorf("target: got %q, want %q", target, sendPollGroup)
			}
			if payload.Name != "Que horas almocamos?" {
				t.Errorf("Name: got %q, want o Header", payload.Name)
			}
			if len(payload.Options) != 2 || payload.Options[0] != "12h" || payload.Options[1] != "13h" {
				t.Errorf("Options: got %q, want [12h 13h] na ordem", payload.Options)
			}
			return domain.MessageSendResult{ID: "wire-id-poll-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollServe(t, sm, jr, sendPollBody, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendPollResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-poll-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-poll-999")
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(sm.SendPollCalls); n != 1 {
		t.Fatalf("SendPoll chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

func TestSendPoll_RejectUnauthenticated(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollServe(t, sm, jr, sendPollBody, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(sm.SendPollCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendPoll %d vez(es)", n)
	}
}

// TestSendPoll_RejectMissingRequiredField cobre as TRÊS recusas de validação
// do histórico pela rota registrada, cada uma pela sua CAUSA no log: o status
// sozinho não distingue "faltou Group" de "faltou Header" de "só uma opção".
func TestSendPoll_RejectMissingRequiredField(t *testing.T) {
	cases := map[string]struct{ body, cause string }{
		"Group":              {`{"Header":"Qual?","Options":["a","b"]}`, "missing Group in payload"},
		"Header":             {`{"Group":"` + sendPollGroup + `","Options":["a","b"]}`, "missing Header in payload"},
		"Options_nenhuma":    {`{"Group":"` + sendPollGroup + `","Header":"Qual?"}`, "at least 2 options are required"},
		"Options_apenas_uma": {`{"Group":"` + sendPollGroup + `","Header":"Qual?","Options":["a"]}`, "at least 2 options are required"},
	}
	for field, tc := range cases {
		t.Run(field, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec, recs := sendPollServeCapturingLog(t, sm, jr, tc.body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload invalido (%s) produziu status de sucesso %d", field, rec.Code)
			}
			logassert.OutcomeLogged(t, recs, tc.cause)
			if n := len(sm.SendPollCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendPoll foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendPoll_SessionFailure(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSendPollSentinel)}
	jr := &contractsfake.JIDResolver{}

	rec, recs := sendPollServeCapturingLog(t, sm, jr, sendPollBody, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	// A CAUSA da porta tem de chegar ao log — o token sentinela e' unico
	// neste arquivo, entao nenhuma outra falha o produziria por acidente.
	logassert.OutcomeLogged(t, recs, sendPollSentinelToken)
	if n := len(sm.SendPollCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendPoll foi chamado %d vez(es)", n)
	}
}

func TestSendPoll_InvalidGroupNeverSends(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}

	rec := sendPollServe(t, sm, jr, `{"Group":"lixo","Header":"Qual?","Options":["a","b"]}`, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(sm.SendPollCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendPoll foi chamado %d vez(es)", n)
	}
}

// TestSendPoll_DownstreamFailureNeverReturns200 é o eixo do defeito original:
// a rota devolvia 200 sem enviar nada. Aqui o envio FALHA, e um 200 seria a
// mentira de volta.
func TestSendPoll_DownstreamFailureNeverReturns200(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendPollFunc: func(context.Context, string, domain.JID, domain.PollPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendPollSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollServe(t, sm, jr, sendPollBody, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendPoll_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendPollFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.PollPayload, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Group":"` + sendPollGroup + `","Header":"Qual?","Options":["a","b"],"Id":"id-do-cliente"}`
	rec := sendPollServe(t, sm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendPollResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendPoll_NoSecretLeak segue o mesmo padrão de
// TestSendLocation_NoSecretLeak: o Header e as opções da enquete e o header
// Authorization carregam segredos da F9.4 (a sessao usa um id NAO secreto, de
// proposito); a sessao falha e o log de saida da rota REGISTRADA nao pode
// carregar nenhum deles.
func TestSendPoll_NoSecretLeak(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-poll-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendPollRouter(sm, jr))

	body := `{"Group":"` + sendPollGroup + `","Header":"` + logassertGlobalHMACKey + `","Options":["` + logassertGlobalHMACKey + `","nao"]}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/poll", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}

// TestSendPoll_MalformedBody_ViaRegisteredRoute recupera o eixo CORPO
// MALFORMADO que a tabela de handler_interactive_test.go cobria antes da
// migração do CAP-14. Requisição AUTENTICADA de propósito: sem isso o 401
// mascara o 400 e o teste não mede o decode.
//
// A segunda requisição trava a CAUSA, não só o sintoma: o envelope de erro é
// o genérico "bad request", e o payload truncado também produziria 400 se o
// decode fosse ignorado (o use case rejeitaria por missing_group). Só o
// registro de saída distingue os dois.
func TestSendPoll_MalformedBody_ViaRegisteredRoute(t *testing.T) {
	const malformed = `{"Group":"12036`

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollServe(t, sm, jr, malformed, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(sm.SendPollCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendPoll %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendPollRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, msgAuthed(httptest.NewRequest(http.MethodPost, "/chat/send/poll", strings.NewReader(malformed))))

	if logRec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unexpected EOF")
	if n := len(smLog.SendPollCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendPoll %d vez(es)", n)
	}
}

// TestSendPoll_MissingSessionID_ViaRegisteredRoute recupera o eixo MISSING
// SESSION ID. A requisição é AUTENTICADA — userinfo presente no contexto —
// mas com o `Id` VAZIO: é 400 do cliente, não 401, e a distinção entre os
// dois é o ponto do teste. O corpo é VÁLIDO de propósito: se a guarda não
// disparar, nada mais impede o envio, e a porta é alcançada.
func TestSendPoll_MissingSessionID_ViaRegisteredRoute(t *testing.T) {
	noSessionID := func(r *http.Request) *http.Request { return withUser(r, "") }

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollServe(t, sm, jr, sendPollBody, noSessionID)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(sm.SendPollCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendPoll %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendPollRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, noSessionID(httptest.NewRequest(http.MethodPost, "/chat/send/poll", strings.NewReader(sendPollBody))))

	if logRec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "missing session id")
	if n := len(smLog.SendPollCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendPoll %d vez(es)", n)
	}
}

// TestSendPoll_WrongTypeInContext_ViaRegisteredRoute recupera o eixo WRONG
// TYPE IN CONTEXT. A chave do contexto é tipada, mas o VALOR é `any`: um
// valor que não satisfaz userInfo tem de virar 401, não pânico.
func TestSendPoll_WrongTypeInContext_ViaRegisteredRoute(t *testing.T) {
	wrongType := func(r *http.Request) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
	}

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendPollServe(t, sm, jr, sendPollBody, wrongType)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(sm.SendPollCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendPoll %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendPollRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, wrongType(httptest.NewRequest(http.MethodPost, "/chat/send/poll", strings.NewReader(sendPollBody))))

	if logRec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unauthorized")
	if n := len(smLog.SendPollCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendPoll %d vez(es)", n)
	}
}

// TestSendPoll_SuccessEmitsNoOutcomeLog recupera o eixo AUSÊNCIA DE LOG NO
// CAMINHO FELIZ. É o eixo que os outros não pegam: um handler que logasse
// TODO request em warn passaria em cada asserção de caminho de erro deste
// arquivo e ainda assim seria o ruído que a Fase 12 existe para evitar. O
// logger do use case é silentLogger{}, então o que se mede aqui é só o
// registro do HANDLER, pela mesma cadeia hlog que router.go instala.
func TestSendPoll_SuccessEmitsNoOutcomeLog(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendPollRouter(sm, jr))
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, msgAuthed(httptest.NewRequest(http.MethodPost, "/chat/send/poll", strings.NewReader(sendPollBody))))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(sm.SendPollCalls); n != 1 {
		t.Fatalf("SendPoll chamado %d vez(es), quero 1", n)
	}
	assertNoOutcomeLog(t, capture.Records(t))
}
