package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
)

// Testes da F209: `{"Phone":"abc"}` virava `abc@s.whatsapp.net`, ia à REDE, e o
// pedido ficava pendurado até o info query esgotar o prazo.
//
// Medido em campo a 2026-08-21, sessão `lucas`, servidor vivo em :8099:
//
//	{"Phone":"abc"}            -> HTTP 500 após duration_ms=75003.445
//	   log: ERR Failed to block user error="info query timed out"
//	        jid=abc@s.whatsapp.net
//	{"Phone":"5511000000001"}  -> ~1s (esse o servidor RECUSA; é a F204)
//
// A diferença importa: o número inexistente é recusado pelo servidor, o lixo é
// IGNORADO por ele. Setenta e cinco segundos de ligação e de slot de tratamento
// por entrada que podíamos recusar em microssegundos.
//
// Estes testes passam pelo ROUTER registado, e não pelo handler cru — armadilha
// nº2 do ARMADILHAS.md, e o router é gorilla/mux.
//
// O dublê de JIDResolver é o `contractsfake`, cujo comportamento por omissão
// imita `JIDResolverAdapter.ResolveJID` INCLUSIVE na guarda acrescentada pela
// F209. Um dublê mais permissivo que a produção esconderia exatamente o defeito
// que a guarda existe para travar.
func rotaBlock(t *testing.T, bl *contractsfake.BlocklistManager) http.Handler {
	t.Helper()
	uc := user.NewBlockUserUseCase(bl, &contractsfake.JIDResolver{}, silentLogger{})
	h := NewUserHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, uc, nil)

	r := mux.NewRouter()
	r.Handle("/user/block", h.BlockUser()).Methods(http.MethodPost)
	return r
}

func TestBlockUser_LixoNaoChegaAhRede(t *testing.T) {
	for _, corpo := range []string{
		`{"Phone":"abc"}`,              // o valor medido em campo
		`{"Phone":"55 11 98765-4321"}`, // formatado para humano
		`{"Phone":"1234"}`,             // curto de mais
		`{"Phone":"+"}`,
	} {
		t.Run(corpo, func(t *testing.T) {
			bl := &contractsfake.BlocklistManager{}
			req := withUser(httptest.NewRequest(http.MethodPost, "/user/block", strings.NewReader(corpo)), "user-1")
			rec := httptest.NewRecorder()
			rotaBlock(t, bl).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status %d, quero 400 (corpo %s)", rec.Code, rec.Body.String())
			}
			// A asserção que importa, e sem a qual o teste não morde: a rede
			// não pode ter sido tocada. Um handler que devolvesse 400 DEPOIS de
			// chamar o transporte passaria no teste de status e continuaria a
			// pendurar 75 segundos.
			if n := len(bl.UpdateBlocklistCalls); n != 0 {
				t.Errorf("o transporte foi chamado %d vez(es) com %s: a recusa "+
					"aconteceu tarde de mais, e o pedido pendura até o info query "+
					"esgotar o prazo (75s medidos em campo)", n, corpo)
			}
		})
	}
}

// O caminho de SUCESSO, que é o que a armadilha nº2 exige e o que impede a
// guarda de recusar de mais: um número real continua a chegar ao transporte,
// já com o servidor por omissão aplicado (a leniência da F203, decisão 35=a).
func TestBlockUser_NumeroRealContinuaAhChegar(t *testing.T) {
	bl := &contractsfake.BlocklistManager{}
	req := withUser(httptest.NewRequest(http.MethodPost, "/user/block",
		strings.NewReader(`{"Phone":"5516981818244"}`)), "user-1")
	rec := httptest.NewRecorder()
	rotaBlock(t, bl).ServeHTTP(rec, req)

	if len(bl.UpdateBlocklistCalls) != 1 {
		t.Fatalf("o transporte foi chamado %d vez(es), quero 1: a guarda da F209 "+
			"está a recusar um número válido, o que é pior que o defeito que ela corrige",
			len(bl.UpdateBlocklistCalls))
	}
	if got := string(bl.UpdateBlocklistCalls[0].Target); got != "5516981818244@s.whatsapp.net" {
		t.Errorf("alvo = %q; a leniência da F203 aplica o servidor por omissão", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
}
