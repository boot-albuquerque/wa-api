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

// Fase 12 — os handlers de /chat/send/{contact,location,buttons,list,poll}
// que ainda dependiam de port.MessageComposer e respondiam "validated" sem
// enviar nada.
//
// Os cinco migraram para portas de envio de verdade: Contact e Location no
// CAP-08A/CAP-08B, Poll no CAP-14, Buttons no CAP-21 e List — o último — no
// CAP-22. A tabela `interactiveCases`/`TestInteractiveHandlers_*` que cobria
// esta migração morreu com o último migrado; os eixos de cada capability
// foram realocados nome por nome para o `handler_send_xxx_test.go` da sua
// própria rota (Contact/Location: `handler_send_contact_test.go`,
// `handler_send_location_test.go`; Poll: `handler_send_poll_test.go`;
// Buttons: `handler_send_buttons_test.go`; List: `handler_send_list_test.go`,
// no CAP-22). O que sobra aqui são os helpers COMPARTILHADOS por outros
// arquivos de teste deste pacote (ipmUser, ipmWithUser, ipmServe,
// ipmErrBoom) — não apagar por engano ao tirar o último uso local.
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
