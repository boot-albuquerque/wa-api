package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/profile"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// GET /session/profile/full é rota separada de /session/profile de propósito:
// agrega chamadas de REDE e está sujeita a rate limit, enquanto a irmã lê só
// o store local. Os dois caminhos de erro seguem o contrato da F83 —
// envelope do ADR-002 e status derivado da categoria.

type mockProfileFullUseCase struct {
	result *profile.ProfileFullResult
	err    error
	chamou bool
}

func (m *mockProfileFullUseCase) Execute(context.Context, string) (*profile.ProfileFullResult, error) {
	m.chamou = true
	return m.result, m.err
}

func serveFull(t *testing.T, uc ProfileFullUseCase, comUserInfo bool, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/session/profile/full", nil)
	if comUserInfo {
		req = req.WithContext(context.WithValue(req.Context(), appport.UserInfoKey, &mockUserInfo{id: id}))
	}
	rec := httptest.NewRecorder()
	NewProfileFullHandler(uc).ServeHTTP(rec, req)
	return rec
}

func TestProfileFull_CaminhoFeliz_200EEnvelope(t *testing.T) {
	uc := &mockProfileFullUseCase{result: &profile.ProfileFullResult{
		ProfileResult: profile.ProfileResult{Pushname: "Lucas"},
		Privacy:       domain.PrivacySettings{LastSeen: "contacts"},
	}}

	rec := serveFull(t, uc, true, "user-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Pushname string `json:"pushname"`
			Privacy  any    `json:"privacy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v", err)
	}
	if !env.Success || env.Data.Pushname != "Lucas" || env.Data.Privacy == nil {
		t.Errorf("corpo inesperado: %s", rec.Body.String())
	}
}

// TestProfileFull_NaoCacheia: o estado da sessão muda sozinho (conectar,
// desconectar, deslogar), e uma resposta em cache faria alguém depurar um
// estado que já não existe.
func TestProfileFull_NaoCacheia(t *testing.T) {
	rec := serveFull(t, &mockProfileFullUseCase{result: &profile.ProfileFullResult{}}, true, "user-1")

	if got := rec.Header().Get("Cache-Control"); got != cacheControlPerfil {
		t.Errorf("Cache-Control = %q, quero %q", got, cacheControlPerfil)
	}
}

func TestProfileFull_SemUserInfo_401NoEnvelope(t *testing.T) {
	uc := &mockProfileFullUseCase{}

	rec := serveFull(t, uc, false, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, quero 401", rec.Code)
	}
	envelopeDe(t, rec)
	if uc.chamou {
		t.Error("o use case foi alcancado apesar do 401")
	}
}

func TestProfileFull_SemID_400NoEnvelope(t *testing.T) {
	uc := &mockProfileFullUseCase{}

	rec := serveFull(t, uc, true, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, quero 400", rec.Code)
	}
	envelopeDe(t, rec)
	if uc.chamou {
		t.Error("o use case foi alcancado apesar do 400")
	}
}

// TestProfileFull_SemSessao_400: o status vem da CATEGORIA do apperr, não de
// um 500 fixo — mesmo contrato da rota irmã (F83).
func TestProfileFull_SemSessao_400(t *testing.T) {
	uc := &mockProfileFullUseCase{
		err: apperr.New("no_session", apperr.CategoryValidation, "no session", false, profile.ErrNoSession),
	}

	rec := serveFull(t, uc, true, "user-1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, quero 400 — sem sessao voltou a ser falha do servidor", rec.Code)
	}
	envelopeDe(t, rec)
}

// TestProfileFull_ErroSemTaxonomia_500: sem esta asserção, mapear tudo para
// 400 passaria no teste acima.
func TestProfileFull_ErroSemTaxonomia_500(t *testing.T) {
	rec := serveFull(t, &mockProfileFullUseCase{err: profile.NewProfileError("banco fora")}, true, "user-1")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, quero 500", rec.Code)
	}
	envelopeDe(t, rec)
}
