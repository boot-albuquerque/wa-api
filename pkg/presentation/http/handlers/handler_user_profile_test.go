package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/usecase/user"
)

// GET /user/profile/{jid} entrega o perfil consolidado a partir de qualquer
// identidade. Como em handler_user_lid_test.go, o teste passa pelo ROUTER de
// verdade: é onde a F81 morava, e servir o handler cru não exercitaria a
// extração do parâmetro — que é a única fonte do alvo nesta rota.

func rotaPerfil(t *testing.T, porta *lidPorta) http.Handler {
	t.Helper()
	uc := user.NewGetUserProfileUseCase(porta, porta, silentLogger{})
	h := NewUserHandlers(nil, nil, nil, nil, nil, nil, nil, uc, nil, nil, nil)

	r := mux.NewRouter()
	r.Handle("/user/profile/{jid}", h.GetUserProfile()).Methods(http.MethodGet)
	return r
}

func TestGetUserProfile_LeOAlvoDoCaminho(t *testing.T) {
	const jid = "5516981818244@s.whatsapp.net"
	porta := &lidPorta{lid: "29343770251463@lid"}

	req := withUser(httptest.NewRequest(http.MethodGet, "/user/profile/"+jid, nil), "user-1")
	rec := httptest.NewRecorder()
	rotaPerfil(t, porta).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	if string(porta.recebido) != jid {
		t.Fatalf("o use case recebeu %q, quero %q — o parametro do caminho nao chegou", porta.recebido, jid)
	}

	var env struct {
		Success bool `json:"success"`
		Data    struct {
			JID   string `json:"jid"`
			LID   string `json:"lid"`
			Query string `json:"query"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v", err)
	}
	if !env.Success {
		t.Fatalf("success=false no caminho feliz: %s", rec.Body.String())
	}
	if env.Data.JID != jid || env.Data.LID != "29343770251463@lid" {
		t.Errorf("identidades erradas no corpo: %s", rec.Body.String())
	}
	if env.Data.Query != jid {
		t.Errorf("query = %q, quero o alvo pedido %q", env.Data.Query, jid)
	}
}

// TestGetUserProfile_NaoExigeCorpo: a chamada natural de um GET. É a asserção
// que a F81 mostrou não ser óbvia — o handler anterior desta família exigia
// corpo e dava 400 em toda chamada.
func TestGetUserProfile_NaoExigeCorpo(t *testing.T) {
	porta := &lidPorta{lid: "1@lid"}

	req := withUser(httptest.NewRequest(http.MethodGet, "/user/profile/5511999@s.whatsapp.net", nil), "user-1")
	rec := httptest.NewRecorder()
	rotaPerfil(t, porta).ServeHTTP(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Fatalf("GET sem corpo deu 400: %s", rec.Body.String())
	}
}

// TestGetUserProfile_SemUserInfo_401: a fronteira compartilhada. Sem o valor
// que o middleware injeta, a rota não pode alcançar o use case.
func TestGetUserProfile_SemUserInfo_401(t *testing.T) {
	porta := &lidPorta{lid: "1@lid"}

	req := httptest.NewRequest(http.MethodGet, "/user/profile/5511999@s.whatsapp.net", nil)
	rec := httptest.NewRecorder()
	rotaPerfil(t, porta).ServeHTTP(rec, req)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
	if porta.recebido != "" {
		t.Errorf("o use case foi alcancado apesar do 401: recebeu %q", porta.recebido)
	}
}
