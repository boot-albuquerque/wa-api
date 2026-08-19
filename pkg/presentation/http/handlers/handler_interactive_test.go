package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
)

// Fase 12 — os handlers de /chat/send/{contact,location,buttons,list,poll}.
//
// Os cinco tem a MESMA forma: guarda de userinfo inline, guarda de session id,
// decode do corpo, use case, envelope. E' o que torna a tabela obrigatoria —
// uma divergencia entre eles (um que esqueca a guarda, um que responda 200 com
// erro do use case) aparece como uma linha vermelha e nao como um teste que
// ninguem escreveu.
//
// Todo caminho de saida (>=400) e' verificado pelo co-gate D: o registro tem de
// existir, carregar a causa e o req_id, estar em warn/error, e nao vazar
// segredo. O caminho de sucesso e' verificado pela AUSENCIA de registro — um
// handler que logasse todo request em warn passaria no (a)-(d) e ainda assim
// seria o Cenario 2 que a fase existe para evitar.

// ipmErrBoom e' a falha generica injetada nas portas. Texto proprio para que a
// assercao de substring do co-gate D nao possa casar por acidente.
var ipmErrBoom = errors.New("ipm-port-boom")

// ipmUser e' o valor que o middleware de autenticacao guarda no contexto.
//
// Get("Token") devolve o valor-sentinela de admin token do co-gate D de
// proposito: e' o que torna a clausula (d) NAO-VACUA nestes testes. O segredo
// esta' no caminho do handler, em memoria, a um Get de distancia; se algum
// caminho de saida passar a logar o userinfo inteiro, a clausula acusa.
type ipmUser struct{ id string }

func (u ipmUser) Get(key string) string {
	switch key {
	case "Id":
		return u.id
	case "Token":
		return logassertAdminToken
	default:
		return ""
	}
}

// ipmWithUser injeta ipmUser sob a chave TIPADA, como AuthAlice faz.
func ipmWithUser(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, ipmUser{id: id}))
}

// ipmServe roda o handler sob a mesma cadeia hlog que router.go instala e
// devolve a resposta junto da saida de log da requisicao.
func ipmServe(t *testing.T, h http.Handler, method, path, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(h)
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if mut != nil {
		r = mut(r)
	}
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, r)
	return rec, capture.Records(t)
}

// ipmAssertNoOutcomeLog trava o caminho feliz: sucesso nao emite registro de
// caminho de saida.
func ipmAssertNoOutcomeLog(t *testing.T, recs []logLine) {
	t.Helper()
	for _, r := range recs {
		if r.has("error") {
			t.Fatalf("caminho de sucesso emitiu registro de erro: %s", r.Raw)
		}
	}
	logassert.NoSecrets(t, recs)
}

// interactiveCase descreve um dos handlers de envio interativo que ainda
// dependem de port.MessageComposer.
type interactiveCase struct {
	name string
	path string
	// build liga o handler ao fake da porta de composicao de mensagem.
	build func(mc *contractsfake.MessageComposer) http.Handler
	// validBody e' o menor corpo que o use case aceita.
	validBody string
	// emptyBodyErr e' a causa que o use case produz para o corpo `{}`.
	emptyBodyErr string
}

// interactiveCases cobria SendContact e SendLocation até CAP-08A/CAP-08B, e
// SendPoll até o CAP-14, migrá-los para port.SimpleMessenger (envio de
// verdade). Os oito eixos que esta tabela cobria para os três foram
// realocados para handler_send_location_test.go, handler_send_contact_test.go
// e handler_send_poll_test.go, no mesmo padrão de
// handler_send_video_test.go (rota gorilla/mux registrada). O destino de cada
// um, por nome — `Xxx` é `Location`, `Contact` ou `Poll`:
//
//	unauthorized                     TestSendXxx_RejectUnauthenticated
//	missing session id               TestSendXxx_MissingSessionID_ViaRegisteredRoute
//	malformed body                   TestSendXxx_MalformedBody_ViaRegisteredRoute
//	campo obrigatório ausente        TestSendXxx_RejectMissingRequiredField
//	session failure                  TestSendXxx_SessionFailure
//	wrong type in context            TestSendXxx_WrongTypeInContext_ViaRegisteredRoute
//	no-secret-leak                   TestSendXxx_NoSecretLeak
//	ausência de log no caminho feliz TestSendXxx_SuccessEmitsNoOutcomeLog
//
// Esta lista é verificável nome por nome, de propósito: a versão anterior
// deste comentário afirmava a realocação em bloco, e três dos oito eixos
// (malformed body, missing session id, wrong type in context) não tinham
// vindo junto — foi o que reprovou a EVAL-08, com as duas ServeHTTP caindo
// de 100,0% para 72,7% de cobertura. Ver HOUSEKEEP.md F122.
//
// CINCO dos oito destinos asseveram também a CAUSA no log (co-gate D,
// `logassert.OutcomeLogged`): `missing session id`, `malformed body`, `campo
// obrigatório ausente`, `session failure` e `wrong type in context`. Isso
// importa porque o envelope de erro deste repo é o genérico "bad request" —
// sem a causa, duas rejeições diferentes com o mesmo status são
// indistinguíveis, e o teste passa com o defeito no lugar. Os outros três,
// e por quê:
//
//   - `no-secret-leak` e `ausência de log no caminho feliz` asseveram
//     AUSÊNCIA (nenhum segredo em nenhum registro; nenhum registro de erro
//     num 200), e por isso não comportam `OutcomeLogged`, que exige a
//     PRESENÇA de um registro de saída.
//   - `unauthorized` (TestSendXxx_RejectUnauthenticated) assevera só status e
//     porta intocada. A causa desse ramo é travada por
//     `TestSendXxx_WrongTypeInContext_ViaRegisteredRoute`, porque a guarda é
//     UMA só (`if !ok || info == nil`, handler_interactive.go:32 e :77) e
//     emite UMA linha de log — os dois casos caem no mesmo ramo.
//
// Só DOIS destinos usam o helper
// `sendLocationServeCapturingLog`/`sendContactServeCapturingLog`/`sendPollServeCapturingLog`, que é o
// `sendXxxServe` acrescido da saída de log da requisição: `campo obrigatório
// ausente` e `session failure`. Os que precisam do log e não usam o helper
// montam `logassert.Wrap` inline, no padrão de `handler_send_video_test.go`.
func interactiveCases() []interactiveCase {
	log := &contractsfake.Logger{}
	return []interactiveCase{
		{
			name: "SendButtons",
			path: "/chat/send/buttons",
			build: func(mc *contractsfake.MessageComposer) http.Handler {
				return NewSendButtonsHandler(message.NewSendButtonsUseCase(mc, log))
			},
			validBody:    `{"Phone":"5511999999999","Body":"escolha"}`,
			emptyBodyErr: "missing Phone in payload",
		},
		{
			name: "SendList",
			path: "/chat/send/list",
			build: func(mc *contractsfake.MessageComposer) http.Handler {
				return NewSendListHandler(message.NewSendListUseCase(mc, log))
			},
			validBody:    `{"Phone":"5511999999999","Desc":"cardapio"}`,
			emptyBodyErr: "missing Phone in payload",
		},
	}
}

// TestInteractiveHandlers_Success: corpo valido, sessao valida, 200 — e nenhum
// registro de caminho de saida.
func TestInteractiveHandlers_Success(t *testing.T) {
	for _, tc := range interactiveCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}

			rec, recs := ipmServe(t, tc.build(mc), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if !env.Success {
				t.Fatalf("envelope.success=false num 200: %s", rec.Body.String())
			}
			if len(mc.NewMessageIDCalls) != 1 {
				t.Fatalf("NewMessageID chamado %d vez(es), quero 1", len(mc.NewMessageIDCalls))
			}
			ipmAssertNoOutcomeLog(t, recs)
		})
	}
}

// TestInteractiveHandlers_Unauthorized: sem userinfo no contexto, 401 logado e
// porta intocada.
func TestInteractiveHandlers_Unauthorized(t *testing.T) {
	for _, tc := range interactiveCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}

			rec, recs := ipmServe(t, tc.build(mc), http.MethodPost, tc.path, tc.validBody, nil)

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			logassert.OutcomeLogged(t, recs, "unauthorized")
			if len(mc.EnsureSessionCalls) != 0 {
				t.Fatalf("requisicao nao autenticada alcancou a porta")
			}
		})
	}
}

// TestInteractiveHandlers_MissingSessionID: autenticado sem Id e' 400, nao 401.
func TestInteractiveHandlers_MissingSessionID(t *testing.T) {
	for _, tc := range interactiveCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}

			rec, recs := ipmServe(t, tc.build(mc), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "") })

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, recs, "missing session id")
			if len(mc.EnsureSessionCalls) != 0 {
				t.Fatalf("requisicao sem session id alcancou a porta")
			}
		})
	}
}

// TestInteractiveHandlers_MalformedBody: JSON truncado e' 400 do cliente.
func TestInteractiveHandlers_MalformedBody(t *testing.T) {
	for _, tc := range interactiveCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}

			rec, recs := ipmServe(t, tc.build(mc), http.MethodPost, tc.path, `{"Phone":"5511`,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, recs, "could not decode payload")
			if len(mc.EnsureSessionCalls) != 0 {
				t.Fatalf("corpo malformado alcancou a porta")
			}
		})
	}
}

// TestInteractiveHandlers_RejectIncompletePayload: o corpo e' JSON valido mas
// falta campo obrigatorio. A causa que o use case produziu tem de chegar ao
// log — e' exatamente o que o registro de fronteira do middleware nao tem.
func TestInteractiveHandlers_RejectIncompletePayload(t *testing.T) {
	for _, tc := range interactiveCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}

			rec, recs := ipmServe(t, tc.build(mc), http.MethodPost, tc.path, `{}`,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			// 400 desde a F66: campo obrigatorio ausente e erro do CLIENTE.
			// Este teste exigia 500, fixando o defeito — 500 dizia ao cliente
			// "o servidor quebrou" quando o remedio estava no payload dele.
			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, recs, tc.emptyBodyErr)
			if len(mc.EnsureSessionCalls) != 0 {
				t.Fatalf("payload incompleto alcancou a porta")
			}
		})
	}
}

// TestInteractiveHandlers_SessionFailure: a sessao do wa-noise nao existe.
func TestInteractiveHandlers_SessionFailure(t *testing.T) {
	for _, tc := range interactiveCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{SessionGuard: contractsfake.FailSession(ipmErrBoom)}

			rec, recs := ipmServe(t, tc.build(mc), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, ipmErrBoom.Error())
			if len(mc.NewMessageIDCalls) != 0 {
				t.Fatalf("sessao invalida seguiu para a geracao de ID")
			}
		})
	}
}

// TestInteractiveHandlers_MessageIDFailure: a sessao existe, mas a geracao de
// ID falha — o unico caminho de erro depois da guarda de sessao.
func TestInteractiveHandlers_MessageIDFailure(t *testing.T) {
	for _, tc := range interactiveCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{
				NewMessageIDFunc: func(context.Context, string) (string, error) { return "", ipmErrBoom },
			}

			rec, recs := ipmServe(t, tc.build(mc), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request { return ipmWithUser(r, "user-1") })

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			logassert.OutcomeLogged(t, recs, ipmErrBoom.Error())
		})
	}
}

// TestInteractiveHandlers_WrongTypeInContext: o contexto carrega `any`; um
// valor que nao satisfaz userInfo e' 401, nao panico.
func TestInteractiveHandlers_WrongTypeInContext(t *testing.T) {
	for _, tc := range interactiveCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}

			rec, recs := ipmServe(t, tc.build(mc), http.MethodPost, tc.path, tc.validBody,
				func(r *http.Request) *http.Request {
					return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
				})

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			logassert.OutcomeLogged(t, recs, "unauthorized")
		})
	}
}
