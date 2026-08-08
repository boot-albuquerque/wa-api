package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/profile"
	"wa-api/pkg/domain/apperr"
)

// F83: os três caminhos de erro de /session/profile usavam http.Error, que
// escreve text/plain. O caminho de SUCESSO já devolvia o envelope do ADR-002
// (commit 7f89d49) — os de erro ficaram para trás, e um cliente que sempre
// desserializa o envelope quebrava em qualquer falha desta rota.
//
// Junto vinha um erro de classificação: "não há sessão" é condição esperada,
// causada pelo cliente, e saía como 500.
//
// Medido antes da correção:
//   HTTP/1.1 500 Internal Server Error
//   Content-Type: text/plain; charset=utf-8

// envelopeDe exige que a resposta seja o envelope do ADR-002 e o devolve.
func envelopeDe(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, quero application/json (corpo: %s)", ct, rec.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v (corpo: %s)", err, rec.Body.String())
	}
	for _, chave := range []string{"code", "success"} {
		if _, ok := env[chave]; !ok {
			t.Fatalf("envelope sem %q: %s", chave, rec.Body.String())
		}
	}
	if env["success"] != false {
		t.Fatalf("success = %v numa resposta de erro", env["success"])
	}
	return env
}

func serveProfile(t *testing.T, uc ProfileUseCase, comUserInfo bool, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/session/profile", nil)
	if comUserInfo {
		req = req.WithContext(context.WithValue(req.Context(), appport.UserInfoKey, &mockUserInfo{id: id}))
	}
	rec := httptest.NewRecorder()
	NewProfileHandler(uc).ServeHTTP(rec, req)
	return rec
}

func TestProfileHandler_ErroSemUserInfo_SaiNoEnvelope(t *testing.T) {
	rec := serveProfile(t, &mockGetProfileUseCase{}, false, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quero 401", rec.Code)
	}
	envelopeDe(t, rec)
}

func TestProfileHandler_ErroSemID_SaiNoEnvelope(t *testing.T) {
	rec := serveProfile(t, &mockGetProfileUseCase{}, true, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400", rec.Code)
	}
	envelopeDe(t, rec)
}

// TestProfileHandler_SemSessao_400: a asserção que mais importa da F83. Sem
// sessão é recusa de CLIENTE, não falha do servidor, e o status tem de vir da
// categoria do apperr — não de um 500 fixo no handler.
func TestProfileHandler_SemSessao_400(t *testing.T) {
	uc := &mockGetProfileUseCase{
		err: apperr.New("no_session", apperr.CategoryValidation, "no session", false, profile.ErrNoSession),
	}

	rec := serveProfile(t, uc, true, "user-1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 — sem sessao voltou a ser tratado como falha do servidor", rec.Code)
	}
	env := envelopeDe(t, rec)
	erro, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("campo error nao e' objeto tipado: %v", env["error"])
	}
	if erro["code"] != "no_session" {
		t.Errorf(`error.code = %v, quero "no_session"`, erro["code"])
	}
}

// TestProfileHandler_ErroSemTaxonomia_500: um erro sem apperr continua sendo
// 500. Sem esta asserção, mapear tudo para 400 passaria nos outros testes.
func TestProfileHandler_ErroSemTaxonomia_500(t *testing.T) {
	rec := serveProfile(t, &mockGetProfileUseCase{err: profile.NewProfileError("banco fora do ar")}, true, "user-1")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500", rec.Code)
	}
	envelopeDe(t, rec)
}
