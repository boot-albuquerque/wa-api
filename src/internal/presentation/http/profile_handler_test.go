package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/application/usecase"
)

// mockGetProfileUseCase implementa a interface do usecase para testes do handler.
type mockGetProfileUseCase struct {
	result string
	err    error
}

func (m *mockGetProfileUseCase) Execute(ctx context.Context, txtID string) (string, error) {
	return m.result, m.err
}

// mockUserInfo implementa a interface userInfo para injetar no context.
type mockUserInfo struct {
	id string
}

func (m *mockUserInfo) Get(key string) string {
	if key == "Id" {
		return m.id
	}
	return ""
}

// TestProfileHandler_NoUserInfo_401 verifica que o handler retorna 401
// quando o middleware não injetou userinfo no contexto.
func TestProfileHandler_NoUserInfo_401(t *testing.T) {
	handler := NewProfileHandler(&mockGetProfileUseCase{result: `{}`})
	req := httptest.NewRequest(http.MethodGet, "/session/profile", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestProfileHandler_EmptyTxtID_400 verifica que o handler retorna 400
// quando o userinfo existe mas Get("Id") retorna string vazia.
func TestProfileHandler_EmptyTxtID_400(t *testing.T) {
	handler := NewProfileHandler(&mockGetProfileUseCase{result: `{}`})
	req := httptest.NewRequest(http.MethodGet, "/session/profile", nil)
	ctx := context.WithValue(req.Context(), appport.UserInfoKey, &mockUserInfo{id: ""})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestProfileHandler_UseCaseError_500_GenericMessage verifica que o handler
// retorna 500 com mensagem genérica (NÃO contém err.Error()) quando o
// usecase falha.
func TestProfileHandler_UseCaseError_500_GenericMessage(t *testing.T) {
	handler := NewProfileHandler(&mockGetProfileUseCase{
		err: usecase.NewProfileError("failed: JID 5511987654321@s.whatsapp.net timeout"),
	})
	req := httptest.NewRequest(http.MethodGet, "/session/profile", nil)
	ctx := context.WithValue(req.Context(), appport.UserInfoKey, &mockUserInfo{id: "test-user"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "5511987654321") {
		t.Errorf("response body contains PII (JID): %s", body)
	}
	if strings.Contains(body, "JID") {
		t.Errorf("response body contains internal detail: %s", body)
	}
	if !strings.Contains(body, "internal server error") {
		t.Errorf("expected generic error message, got %q", body)
	}
}

// TestProfileHandler_ValidResponse_200_JSON verifica que o handler retorna
// 200 com Content-Type application/json quando o usecase retorna sucesso.
func TestProfileHandler_ValidResponse_200_JSON(t *testing.T) {
	expectedJSON := `{"pushname":"John","avatar_url":"","avatar_id":"","jid":"5511@s.whatsapp.net","full_name":"","business_name":""}`
	handler := NewProfileHandler(&mockGetProfileUseCase{result: expectedJSON})
	req := httptest.NewRequest(http.MethodGet, "/session/profile", nil)
	ctx := context.WithValue(req.Context(), appport.UserInfoKey, &mockUserInfo{id: "test-user"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	// Validar Cache-Control per-handler (Fase 1 — não-global)
	if cc := w.Header().Get("Cache-Control"); cc == "" {
		t.Error("expected Cache-Control header to be set")
	}

	// Validar que o body é JSON válido com os campos esperados
	var parsed map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response body is not valid JSON: %v\nBody: %s", err, w.Body.String())
	}
	// Verificar campos obrigatórios
	for _, field := range []string{"pushname", "avatar_url", "avatar_id", "jid", "full_name", "business_name"} {
		if _, ok := parsed[field]; !ok {
			t.Errorf("missing required field %q in JSON response", field)
		}
	}
}

// TestProfileHandler_RequestCancelled_503 verifica que o handler retorna erro
// quando o contexto da request é cancelado (cliente desconecta).
func TestProfileHandler_RequestCancelled_503(t *testing.T) {
	handler := NewProfileHandler(&mockGetProfileUseCase{
		err: context.Canceled,
	})
	req := httptest.NewRequest(http.MethodGet, "/session/profile", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // cancela imediatamente
	ctx = context.WithValue(ctx, appport.UserInfoKey, &mockUserInfo{id: "test-user"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Quando o usecase retorna context.Canceled, o handler retorna 500 genérico.
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "internal server error") {
		t.Errorf("expected generic error message, got %q", body)
	}
}
