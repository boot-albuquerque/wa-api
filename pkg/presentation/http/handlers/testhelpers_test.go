package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
)

// Helpers compartilhados por praticamente toda a suíte de send/* deste
// pacote, herdados de duas tabelas que existiam só para cobrir os handlers
// ainda ligados a port.MessageComposer (respondiam "validated" sem enviar
// nada de verdade) e morreram quando a última capability migrou para uma
// porta de envio real:
//
//   - msgAuthed veio de handler_message_test.go. A tabela cobria send/text
//     (migrou para port.TextMessenger no CAP-01), send/edit e delete/message
//     (CAP-10, port.ChatMessenger + port.JIDResolver), send/template
//     (CAP-15), send/buttons (CAP-21) e send/list — o último caso, no
//     CAP-22 (port.SimpleMessenger ou port.InteractiveMessenger). Os eixos
//     de send/list foram realocados, nome por nome, para
//     handler_send_list_test.go:
//
//     sucesso                     TestSendList_Success_ViaRegisteredRoute
//     não autenticado             TestSendList_RejectUnauthenticated
//     tipo errado no contexto     TestSendList_WrongTypeInContext_ViaRegisteredRoute
//     session id vazio            TestSendList_MissingSessionID_ViaRegisteredRoute
//     corpo malformado            TestSendList_MalformedBody_ViaRegisteredRoute
//     campo obrigatório ausente   TestSendList_RejectMissingRequiredField
//     falha de sessão             TestSendList_SessionFailure
//     sucesso sem log de saída    TestSendList_SuccessEmitsNoOutcomeLog
//     segredo no log              TestSendList_NoSecretLeak
//     Id do cliente               TestSendList_ClientSuppliedIDIsForwardedButServerIDWins
//
//     Os DOIS eixos de geração de ID (MessageIDFailure e a metade
//     `generatesID` de Success) não têm destino porque deixaram de existir
//     para a rota: o use case não chama mais NewMessageID, e o MessageID
//     publicado é o que a porta devolveu.
//
//   - ipmErrBoom, ipmUser, ipmWithUser e ipmServe vieram de
//     handler_interactive_test.go (Fase 12). A tabela `interactiveCases` /
//     `TestInteractiveHandlers_*` cobria /chat/send/{contact,location,
//     buttons,list,poll}; os cinco migraram para portas de envio de verdade
//     — Contact e Location no CAP-08A/CAP-08B, Poll no CAP-14, Buttons no
//     CAP-21 e List no CAP-22 — e os eixos de cada um foram realocados nome
//     por nome para o handler_send_xxx_test.go da própria rota
//     (handler_send_contact_test.go, handler_send_location_test.go,
//     handler_send_poll_test.go, handler_send_buttons_test.go,
//     handler_send_list_test.go).
//
// Todo caminho de saida (>=400) e' verificado pelo co-gate D: o registro tem
// de existir, carregar a causa e o req_id, estar em warn/error, e nao vazar
// segredo. O caminho de sucesso e' verificado pela AUSENCIA de registro — um
// handler que logasse todo request em warn passaria no (a)-(d) e ainda assim
// seria o Cenario 2 que a fase existe para evitar.

// msgAuthed e' a mutacao padrao: requisicao autenticada com sessao valida.
func msgAuthed(r *http.Request) *http.Request { return withUser(r, "user-1") }

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
