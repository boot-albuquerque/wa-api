package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// Testes da F81: GET /user/lid/{jid} devolvia 400 em toda chamada.
//
// A rota declara `{jid}` no caminho e o campo do domínio se documenta como
// `JID string // from URL`, mas o handler decodificava o CORPO. Um GET não
// tem corpo, o decode falhava com EOF, e o parâmetro da URL era ignorado —
// provado em campo: `/user/lid/ignorado` com corpo `{"JID":"...@s.whatsapp.net"}`
// respondia 200 com o LID, enquanto `/user/lid/<jid-real>` sem corpo dava 400.
//
// O ponto destes testes é passar pelo ROUTER de verdade. Os testes que já
// existiam montam o handler direto, sem padrão de rota, e por isso nunca
// exercitaram a extração do parâmetro — que é onde o defeito morava.
//
// Isso importa duas vezes: o router é gorilla/mux, não o ServeMux nativo.
// Um teste que montasse http.ServeMux com o mesmo padrão passaria com
// r.PathValue e a produção continuaria quebrada.

// lidPorta reaproveita spyPort — que já satisfaz ContactDirectory e
// JIDResolver inteiros — e sobrescreve apenas GetLIDForPN para registrar o
// JID que chegou ao use case. Essa é a asserção central destes testes.
type lidPorta struct {
	spyPort
	recebido domain.JID
	lid      domain.JID
}

func (p *lidPorta) GetLIDForPN(_ context.Context, _ string, jid domain.JID) (domain.JID, error) {
	p.recebido = jid
	return p.lid, nil
}

// Os dois resolvedores devolvem o valor cru: o JID do caminho já vem
// qualificado nestes testes, e o que se quer comparar é exatamente o que
// entrou pela URL.
func (p *lidPorta) ResolveQualifiedJID(_ context.Context, raw string) (domain.JID, error) {
	return domain.JID(raw), nil
}

func (p *lidPorta) ResolveJID(_ context.Context, raw string) (domain.JID, error) {
	return domain.JID(raw), nil
}

// rotaLID monta o handler sob o MESMO padrão que wiring_routes.go:92 registra.
func rotaLID(t *testing.T, porta *lidPorta) http.Handler {
	t.Helper()
	uc := user.NewGetUserLIDUseCase(porta, porta, silentLogger{})
	h := NewUserHandlers(nil, nil, nil, nil, nil, nil, uc, nil, nil, nil, nil)

	r := mux.NewRouter()
	r.Handle("/user/lid/{jid}", h.GetUserLID()).Methods(http.MethodGet)
	return r
}

func TestGetUserLID_LeOJIDDoCaminho(t *testing.T) {
	const jid = "5516981818244@s.whatsapp.net"
	porta := &lidPorta{lid: "29343770251463@lid"}

	req := withUser(httptest.NewRequest(http.MethodGet, "/user/lid/"+jid, nil), "user-1")
	rec := httptest.NewRecorder()
	rotaLID(t, porta).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	// A asserção que importa: o JID do CAMINHO chegou ao use case. Sem ela, um
	// handler que devolvesse 200 com JID vazio passaria.
	if string(porta.recebido) != jid {
		t.Fatalf("o use case recebeu %q, quero %q — o parametro do caminho voltou a ser ignorado", porta.recebido, jid)
	}

	var env struct {
		Success bool `json:"success"`
		Data    struct {
			JID string `json:"jid"`
			LID string `json:"lid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v", err)
	}
	if !env.Success || env.Data.LID != "29343770251463@lid" {
		t.Errorf("envelope inesperado: %s", rec.Body.String())
	}
}

// TestGetUserLID_NaoExigeCorpo é o coração da F81: a chamada natural de um
// GET — sem corpo nenhum — tem de funcionar. Era exatamente ela que dava 400.
func TestGetUserLID_NaoExigeCorpo(t *testing.T) {
	porta := &lidPorta{lid: "1@lid"}

	req := withUser(httptest.NewRequest(http.MethodGet, "/user/lid/5511999999999@s.whatsapp.net", nil), "user-1")
	rec := httptest.NewRecorder()
	rotaLID(t, porta).ServeHTTP(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Fatalf("GET sem corpo voltou a dar 400: %s", rec.Body.String())
	}
}

// TestGetUserLID_CaminhoVenceOCorpo: com o defeito, o corpo era a ÚNICA
// fonte do JID. Aqui o caminho e o corpo discordam de propósito, e quem tem
// de vencer é o caminho — senão a correção teria trocado uma fonte errada
// por duas fontes concorrentes, que é pior.
func TestGetUserLID_CaminhoVenceOCorpo(t *testing.T) {
	const doCaminho = "5516981818244@s.whatsapp.net"
	const doCorpo = "5599999999999@s.whatsapp.net"
	porta := &lidPorta{lid: "1@lid"}

	req := httptest.NewRequest(http.MethodGet, "/user/lid/"+doCaminho,
		strings.NewReader(`{"JID":"`+doCorpo+`"}`))
	rec := httptest.NewRecorder()
	rotaLID(t, porta).ServeHTTP(rec, withUser(req, "user-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if string(porta.recebido) == doCorpo {
		t.Fatal("o corpo venceu o caminho — o handler voltou a decodificar o payload")
	}
	if string(porta.recebido) != doCaminho {
		t.Fatalf("o use case recebeu %q, quero o JID do caminho %q", porta.recebido, doCaminho)
	}
}
