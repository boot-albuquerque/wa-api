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

// Este arquivo cobre POST /chat/send/location desde a migração do CAP-08A
// para port.SimpleMessenger — o handler que efetivamente monta o
// LocationMessage e o envia pelo wa-noise, e não mais um "validated" sem
// fazer nada. Mesma estrutura de handler_send_video_test.go (rota
// gorilla/mux registrada).

const sendLocationSentinelToken = "send-location-sentinel-cause-7f3c1d"

var errSendLocationSentinel = errors.New(sendLocationSentinelToken)

// sendLocationRouter registra o handler pela rota real (gorilla/mux), como
// wiring_routes.go faz — não handler.ServeHTTP direto (ARMADILHA 2 deste
// repo: defeito de rota só aparece testando pela rota registrada).
func sendLocationRouter(sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver) http.Handler {
	uc := message.NewSendLocationUseCase(sm, jr, silentLogger{})
	h := NewSendLocationHandler(uc)

	r := mux.NewRouter()
	r.Handle("/chat/send/location", h).Methods(http.MethodPost)
	return r
}

func sendLocationServe(t *testing.T, sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/location", strings.NewReader(body))
	sendLocationRouter(sm, jr).ServeHTTP(rec, mut(req))
	return rec
}

// sendLocationServeCapturingLog e' o sendLocationServe com a saida de log da requisicao —
// a mesma cadeia hlog que router.go instala, via logassert.Wrap. Existe para
// que os caminhos de erro possam asseverar a CAUSA (co-gate D), e nao so' o
// status: o envelope de erro deste repo e' o generico "bad request", entao
// duas rejeicoes diferentes com o mesmo status sao indistinguiveis pelo corpo.
func sendLocationServeCapturingLog(t *testing.T, sm *contractsfake.SimpleMessenger, jr *contractsfake.JIDResolver, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(sendLocationRouter(sm, jr))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/send/location", strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

type sendLocationResultBody struct {
	MessageID string `json:"message_id"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

// TestSendLocation_Success_ViaRegisteredRoute prova o caminho HTTP ->
// handler -> usecase -> SimpleMessenger.SendLocation pela rota gorilla/mux
// REGISTRADA, com Status="sent", MessageID igual ao devolvido pela porta e
// Timestamp vindo do envio real. Valores negativos e fracionários reais
// (-23.5505, -46.6333) atravessam float64 sem perda.
func TestSendLocation_Success_ViaRegisteredRoute(t *testing.T) {
	sentAt := int64(1755500110)
	sm := &contractsfake.SimpleMessenger{
		SendLocationFunc: func(_ context.Context, _ string, target domain.JID, payload domain.LocationPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if target != domain.JID("5511999999999@s.whatsapp.net") {
				t.Errorf("target: got %q", target)
			}
			if payload.Latitude != -23.5505 {
				t.Errorf("Latitude: got %v, want %v", payload.Latitude, -23.5505)
			}
			if payload.Longitude != -46.6333 {
				t.Errorf("Longitude: got %v, want %v", payload.Longitude, -46.6333)
			}
			if payload.Name != "Praça da Sé" {
				t.Errorf("Name: got %q, want %q", payload.Name, "Praça da Sé")
			}
			return domain.MessageSendResult{ID: "wire-id-location-999", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Name":"Praça da Sé","Latitude":-23.5505,"Longitude":-46.6333}`
	rec := sendLocationServe(t, sm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if !env.Success {
		t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
	}

	var data sendLocationResultBody
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.Status != domain.StatusSent {
		t.Errorf("status: got %q, want %q", data.Status, domain.StatusSent)
	}
	if data.MessageID != "wire-id-location-999" {
		t.Errorf("message_id: got %q, want %q (o que a porta devolveu)", data.MessageID, "wire-id-location-999")
	}
	if data.Timestamp != sentAt {
		t.Errorf("timestamp: got %d, want %d", data.Timestamp, sentAt)
	}
	if n := len(sm.SendLocationCalls); n != 1 {
		t.Fatalf("SendLocation chamado %d vez(es) pela rota registrada, quero 1", n)
	}
}

func TestSendLocation_RejectUnauthenticated(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Latitude":-23.5505,"Longitude":-46.6333}`
	rec := sendLocationServe(t, sm, jr, body, func(r *http.Request) *http.Request { return r })

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Fatalf("requisicao nao autenticada alcancou SendLocation %d vez(es)", n)
	}
}

func TestSendLocation_RejectMissingRequiredField(t *testing.T) {
	// A causa do use case entra na tabela: o status sozinho nao distingue
	// "faltou Phone" de "faltou Longitude", e era exatamente isso que a
	// tabela original de handler_interactive_test.go exigia (co-gate D).
	cases := map[string]struct{ body, cause string }{
		"Phone":     {`{"Latitude":-23.5505,"Longitude":-46.6333}`, "missing Phone in payload"},
		"Latitude":  {`{"Phone":"5511999999999","Longitude":-46.6333}`, "missing Latitude in payload"},
		"Longitude": {`{"Phone":"5511999999999","Latitude":-23.5505}`, "missing Longitude in payload"},
	}
	for field, tc := range cases {
		t.Run(field, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec, recs := sendLocationServeCapturingLog(t, sm, jr, tc.body, msgAuthed)

			if rec.Code < 400 {
				t.Fatalf("payload sem %s produziu status de sucesso %d", field, rec.Code)
			}
			logassert.OutcomeLogged(t, recs, tc.cause)
			if n := len(sm.SendLocationCalls); n != 0 {
				t.Fatalf("payload invalido, mas SendLocation foi chamado %d vez(es)", n)
			}
		})
	}
}

// TestSendLocation_ZeroEhAceite_ViaRegisteredRoute prova, PELA ROTA REGISTADA,
// que zero é coordenada válida.
//
// Este teste dizia o CONTRÁRIO até 2026-08-22: chamava-se
// `ZeroLatitudeOrLongitude_Rejected_ViaRegisteredRoute` e tratava o 400 como o
// comportamento a preservar, descrevendo-o como "defeito HISTÓRICO preservado".
// Era a quarta vez nesta sessão que um teste travava o defeito em vez do
// comportamento correto.
//
// A F121 trocou Latitude/Longitude para *float64: nil é "não informado", e 0 é
// o equador ou o meridiano de Greenwich. Verificado em campo — um envio para
// (0, 0), no golfo da Guiné, devolveu 200 e a mensagem saiu.
func TestSendLocation_ZeroEhAceite_ViaRegisteredRoute(t *testing.T) {
	bodies := map[string]string{
		"zero_latitude":  `{"Phone":"5511999999999","Latitude":0,"Longitude":-46.6333}`,
		"zero_longitude": `{"Phone":"5511999999999","Latitude":-23.5505,"Longitude":0}`,
		"ambos_zero":     `{"Phone":"5511999999999","Latitude":0,"Longitude":0}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			jr := &contractsfake.JIDResolver{}

			rec := sendLocationServe(t, sm, jr, body, msgAuthed)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s recusado com %d: zero é coordenada válida (F121). corpo: %s",
					name, rec.Code, rec.Body.String())
			}
			if n := len(sm.SendLocationCalls); n != 1 {
				t.Fatalf("%s aceite, mas SendLocation foi chamado %d vez(es)", name, n)
			}
		})
	}
}

// E o limite: campo AUSENTE continua a ser 400. Sem isto, a correção teria
// trocado uma recusa errada por uma aceitação errada.
func TestSendLocation_CampoAusenteContinua400_ViaRegisteredRoute(t *testing.T) {
	bodies := map[string]string{
		"sem_latitude":  `{"Phone":"5511999999999","Longitude":-46.6333}`,
		"sem_longitude": `{"Phone":"5511999999999","Latitude":-23.5505}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			sm := &contractsfake.SimpleMessenger{}
			rec := sendLocationServe(t, sm, &contractsfake.JIDResolver{}, body, msgAuthed)
			if rec.Code == http.StatusOK {
				t.Fatalf("%s produziu 200: campo ausente não é zero", name)
			}
			if n := len(sm.SendLocationCalls); n != 0 {
				t.Fatalf("%s recusado, mas SendLocation foi chamado %d vez(es)", name, n)
			}
		})
	}
}

func TestSendLocation_SessionFailure(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errSendLocationSentinel)}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Latitude":-23.5505,"Longitude":-46.6333}`
	rec, recs := sendLocationServeCapturingLog(t, sm, jr, body, msgAuthed)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	// A CAUSA da porta tem de chegar ao log — o token sentinela e' unico
	// neste arquivo, entao nenhuma outra falha o produziria por acidente.
	logassert.OutcomeLogged(t, recs, sendLocationSentinelToken)
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Fatalf("sessao invalida, mas SendLocation foi chamado %d vez(es)", n)
	}
}

func TestSendLocation_InvalidPhoneNeverSends(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}

	body := `{"Phone":"lixo","Latitude":-23.5505,"Longitude":-46.6333}`
	rec := sendLocationServe(t, sm, jr, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("JID invalido produziu 200: %s", rec.Body.String())
	}
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Fatalf("JID invalido, mas SendLocation foi chamado %d vez(es)", n)
	}
}

func TestSendLocation_DownstreamFailureNeverReturns200(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendLocationFunc: func(context.Context, string, domain.JID, domain.LocationPayload, *domain.ReplyContext, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errSendLocationSentinel
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Latitude":-23.5505,"Longitude":-46.6333}`
	rec := sendLocationServe(t, sm, jr, body, msgAuthed)

	if rec.Code == http.StatusOK {
		t.Fatalf("falha no envio produziu 200: %s", rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Success {
		t.Fatalf("envelope.success=true com envio falho: %s", rec.Body.String())
	}
}

func TestSendLocation_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{
		SendLocationFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.LocationPayload, _ *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	jr := &contractsfake.JIDResolver{}

	body := `{"Phone":"5511999999999","Latitude":-23.5505,"Longitude":-46.6333,"Id":"id-do-cliente"}`
	rec := sendLocationServe(t, sm, jr, body, msgAuthed)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var data sendLocationResultBody
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &data); err != nil {
		t.Fatalf("envelope.data invalido: %v", err)
	}
	if data.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("message_id: got %q, want %q", data.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendLocation_NoSecretLeak segue o mesmo padrão de
// TestSendVideo_NoSecretLeak: Phone e o header Authorization carregam
// segredos da F9.4 (a sessao usa um id NAO secreto, de proposito); a sessao
// falha e o log de saida da rota REGISTRADA nao pode carregar nenhum deles.
func TestSendLocation_NoSecretLeak(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{SessionGuard: contractsfake.FailSession(errors.New("send-location-secret-leak-cause"))}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendLocationRouter(sm, jr))

	body := `{"Phone":"` + logassertGlobalHMACKey + `","Latitude":-23.5505,"Longitude":-46.6333}`
	req := httptest.NewRequest(http.MethodPost, "/chat/send/location", strings.NewReader(body))
	req = withUser(req, "no-secret-leak-session")
	req.Header.Set("Authorization", logassertAdminToken)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code < 400 {
		t.Fatalf("falha de sessao produziu status de sucesso %d", rec.Code)
	}
	logassert.NoSecrets(t, capture.Records(t))
}

// TestSendLocation_MalformedBody_ViaRegisteredRoute recupera o eixo CORPO
// MALFORMADO que a tabela de handler_interactive_test.go cobria antes da
// migração do CAP-08A (TestInteractiveHandlers_MalformedBody, mesmo JSON
// truncado). Requisição AUTENTICADA de propósito: sem isso o 401 mascara o
// 400 e o teste não mede o decode.
//
// A segunda requisição trava a CAUSA, não só o sintoma: o envelope de erro
// é o genérico "bad request", e o payload truncado também produziria 400 se
// o decode fosse ignorado (o use case rejeitaria por missing_phone). Só o
// registro de saída distingue os dois — o campo error carrega a causa CRU
// do decoder (F141).
func TestSendLocation_MalformedBody_ViaRegisteredRoute(t *testing.T) {
	const malformed = `{"Phone":"5511`

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendLocationServe(t, sm, jr, malformed, msgAuthed)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendLocation %d vez(es)", n)
	}

	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendLocationRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, msgAuthed(httptest.NewRequest(http.MethodPost, "/chat/send/location", strings.NewReader(malformed))))

	if logRec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unexpected EOF")
	if n := len(smLog.SendLocationCalls); n != 0 {
		t.Fatalf("corpo malformado alcancou SendLocation %d vez(es)", n)
	}
}

// TestSendLocation_MissingSessionID_ViaRegisteredRoute recupera o eixo
// MISSING SESSION ID que a tabela de handler_interactive_test.go cobria
// antes da migração do CAP-08A (TestInteractiveHandlers_MissingSessionID).
// A requisição é AUTENTICADA — userinfo presente no contexto — mas com o
// `Id` VAZIO: é 400 do cliente, não 401, e a distinção entre os dois é o
// ponto do teste. O corpo é VÁLIDO de propósito: se a guarda não disparar,
// nada mais impede o envio, e a porta é alcançada.
func TestSendLocation_MissingSessionID_ViaRegisteredRoute(t *testing.T) {
	const body = `{"Phone":"5511999999999","Latitude":-23.5505,"Longitude":-46.6333}`
	noSessionID := func(r *http.Request) *http.Request { return withUser(r, "") }

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendLocationServe(t, sm, jr, body, noSessionID)

	assertErrorEnvelope(t, rec, http.StatusBadRequest)
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendLocation %d vez(es)", n)
	}

	// A CAUSA, não só o sintoma: o envelope de erro é o genérico "bad
	// request", então só o registro de saída distingue a guarda de sessão
	// de qualquer outra rejeição com 400.
	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendLocationRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, noSessionID(httptest.NewRequest(http.MethodPost, "/chat/send/location", strings.NewReader(body))))

	if logRec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "missing session id")
	if n := len(smLog.SendLocationCalls); n != 0 {
		t.Fatalf("requisicao sem session id alcancou SendLocation %d vez(es)", n)
	}
}

// TestSendLocation_WrongTypeInContext_ViaRegisteredRoute recupera o eixo
// WRONG TYPE IN CONTEXT que a tabela de handler_interactive_test.go cobria
// antes da migração do CAP-08A (TestInteractiveHandlers_WrongTypeInContext).
// A chave do contexto é tipada, mas o VALOR é `any`: um valor que não
// satisfaz userInfo tem de virar 401, não pânico — a asserção do type
// assertion do handler é a de duas variáveis (`info, ok := ...`), e é isso
// que este teste trava.
func TestSendLocation_WrongTypeInContext_ViaRegisteredRoute(t *testing.T) {
	const body = `{"Phone":"5511999999999","Latitude":-23.5505,"Longitude":-46.6333}`
	wrongType := func(r *http.Request) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
	}

	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	rec := sendLocationServe(t, sm, jr, body, wrongType)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if n := len(sm.SendLocationCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendLocation %d vez(es)", n)
	}

	// A CAUSA, não só o sintoma: 401 tem de vir da guarda de userinfo.
	smLog := &contractsfake.SimpleMessenger{}
	wrapped, capture := logassert.Wrap(sendLocationRouter(smLog, jr))
	logRec := httptest.NewRecorder()
	wrapped.ServeHTTP(logRec, wrongType(httptest.NewRequest(http.MethodPost, "/chat/send/location", strings.NewReader(body))))

	if logRec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401 (corpo: %s)", logRec.Code, logRec.Body.String())
	}
	logassert.OutcomeLogged(t, capture.Records(t), "unauthorized")
	if n := len(smLog.SendLocationCalls); n != 0 {
		t.Fatalf("contexto com tipo errado alcancou SendLocation %d vez(es)", n)
	}
}

// TestSendLocation_SuccessEmitsNoOutcomeLog recupera o eixo AUSÊNCIA DE LOG
// NO CAMINHO FELIZ (assertNoOutcomeLog na tabela original). É o eixo que
// os outros não pegam: um handler que logasse TODO request em warn passaria
// em cada asserção de caminho de erro deste arquivo e ainda assim seria o
// ruído que a Fase 12 existe para evitar. O logger do use case é
// silentLogger{}, então o que se mede aqui é só o registro do HANDLER, pela
// mesma cadeia hlog que router.go instala.
func TestSendLocation_SuccessEmitsNoOutcomeLog(t *testing.T) {
	sm := &contractsfake.SimpleMessenger{}
	jr := &contractsfake.JIDResolver{}

	wrapped, capture := logassert.Wrap(sendLocationRouter(sm, jr))
	rec := httptest.NewRecorder()
	body := `{"Phone":"5511999999999","Latitude":-23.5505,"Longitude":-46.6333}`
	wrapped.ServeHTTP(rec, msgAuthed(httptest.NewRequest(http.MethodPost, "/chat/send/location", strings.NewReader(body))))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if n := len(sm.SendLocationCalls); n != 1 {
		t.Fatalf("SendLocation chamado %d vez(es), quero 1", n)
	}
	assertNoOutcomeLog(t, capture.Records(t))
}
