package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wa-api/pkg/domain/apperr"
)

// decodeEnvelope unmarshals a RespondJSON body into a generic map for
// assertions on individual fields.
func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to unmarshal response body %q: %v", rec.Body.String(), err)
	}
	return envelope
}

// TestRespondJSON_AppError_DerivesStatusFromCategory is the core of the
// Fase 4a envelope change, and the one branch the golden files (all
// non-typed 401s) never exercise: passing a *apperr.AppError overrides
// whatever statusCode the call site wrote, using Category.HTTPStatus()
// instead.
func TestRespondJSON_AppError_DerivesStatusFromCategory(t *testing.T) {
	tests := []struct {
		name       string
		category   apperr.Category
		wantStatus int
	}{
		{"validation overrides to 400", apperr.CategoryValidation, http.StatusBadRequest},
		{"unauthorized overrides to 401", apperr.CategoryUnauthorized, http.StatusUnauthorized},
		{"internal overrides to 500", apperr.CategoryInternal, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := apperr.New("test_code", tt.category, "safe message", false, nil)

			// Deliberately pass a CONFLICTING statusCode (200) to prove the
			// AppError's Category wins, not the call site's argument — the
			// entire point of this phase (ADR-002).
			rec := httptest.NewRecorder()
			RespondJSON(rec, http.StatusOK, nil, appErr)

			if rec.Code != tt.wantStatus {
				t.Errorf("HTTP status = %d, want %d (Category should override the passed statusCode)", rec.Code, tt.wantStatus)
			}

			envelope := decodeEnvelope(t, rec)
			if got := envelope["code"]; got != float64(tt.wantStatus) {
				t.Errorf("envelope[\"code\"] = %v, want %v", got, tt.wantStatus)
			}
			if got := envelope["success"]; got != false {
				t.Errorf("envelope[\"success\"] = %v, want false", got)
			}

			errObj, ok := envelope["error"].(map[string]interface{})
			if !ok {
				t.Fatalf("envelope[\"error\"] = %#v, want an object with code/message (ADR-002)", envelope["error"])
			}
			if errObj["code"] != "test_code" {
				t.Errorf("error.code = %v, want %q", errObj["code"], "test_code")
			}
			if errObj["message"] != "safe message" {
				t.Errorf("error.message = %v, want %q", errObj["message"], "safe message")
			}
		})
	}
}

// TestRespondJSON_AppError_ThroughWrappedError proves errors.As reaches an
// AppError wrapped by fmt.Errorf("...: %w", err), the pattern the rest of
// the repository already uses for every other error.
func TestRespondJSON_AppError_ThroughWrappedError(t *testing.T) {
	appErr := apperr.New("db_error", apperr.CategoryInternal, "database error", true, nil)
	wrapped := fmt.Errorf("use case failed: %w", appErr)

	rec := httptest.NewRecorder()
	RespondJSON(rec, http.StatusOK, nil, wrapped)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d — errors.As should unwrap through fmt.Errorf's %%w", rec.Code, http.StatusInternalServerError)
	}
	envelope := decodeEnvelope(t, rec)
	errObj, ok := envelope["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope[\"error\"] = %#v, want an object", envelope["error"])
	}
	if errObj["code"] != "db_error" {
		t.Errorf("error.code = %v, want %q", errObj["code"], "db_error")
	}
}

// TestRespondJSON_UntypedError_UsesCanonicalErrorObject proves the branch
// that catches everything the taxonomy did not: the call site's statusCode is
// kept, and `error` is the canonical OBJECT for that status — never the
// wrapped error's own text, and never a bare string.
//
// The old version of this test asserted `envelope["error"] == "bad request"`,
// a plain string. That contract is gone: a client cannot branch on a sentence,
// and the sentence changed shape per status. See docs/HTTP-DTO-CONVENTIONS.md.
func TestRespondJSON_UntypedError_UsesCanonicalErrorObject(t *testing.T) {
	tests := []struct {
		status   int
		wantCode string
	}{
		{http.StatusBadRequest, "invalid_request"},
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not_found"},
		{http.StatusConflict, "conflict"},
		{http.StatusUnprocessableEntity, "unprocessable_entity"},
		{http.StatusTooManyRequests, "rate_limited"},
		{http.StatusNotImplemented, "not_implemented"},
		{http.StatusBadGateway, "bad_gateway"},
		{http.StatusServiceUnavailable, "service_unavailable"},
		{http.StatusGatewayTimeout, "gateway_timeout"},
		{http.StatusInternalServerError, "internal_error"},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			sensitive := errors.New("connection string: postgres://user:hunter2@internal-host/db")

			rec := httptest.NewRecorder()
			RespondJSON(rec, tt.status, nil, sensitive)

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			envelope := decodeEnvelope(t, rec)
			if got := envelope["success"]; got != false {
				t.Errorf("envelope[\"success\"] = %v, want false", got)
			}
			errObj, ok := envelope["error"].(map[string]interface{})
			if !ok {
				t.Fatalf("envelope[\"error\"] = %#v, want an object {code, message} for EVERY status", envelope["error"])
			}
			if errObj["code"] != tt.wantCode {
				t.Errorf("error.code = %v, want %q", errObj["code"], tt.wantCode)
			}
			if msg, _ := errObj["message"].(string); msg == "" {
				t.Errorf("error.message is empty for status %d", tt.status)
			}
			if len(errObj) != 2 {
				t.Errorf("error object has %d keys (%v), want exactly code+message in production", len(errObj), errObj)
			}
			body := rec.Body.String()
			if strings.Contains(body, "hunter2") || strings.Contains(body, "postgres://") {
				t.Fatalf("response body leaked the wrapped error's text: %s", body)
			}
		})
	}
}

// TestRespondJSON_UnrecognizedStatusCode_FallsBackSafely proves the fallback
// for a status this taxonomy does not enumerate: the most conservative body,
// because a status nobody planned for is by definition one we cannot describe.
func TestRespondJSON_UnrecognizedStatusCode_FallsBackSafely(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondJSON(rec, 999, nil, errors.New("some error"))

	envelope := decodeEnvelope(t, rec)
	errObj, ok := envelope["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope[\"error\"] = %#v, want an object", envelope["error"])
	}
	if errObj["code"] != "internal_error" {
		t.Errorf("error.code = %v, want %q (fallback for unrecognized status)", errObj["code"], "internal_error")
	}
}

// TestRespondJSON_ErrorIsNeverAString is the shape assertion on its own,
// separate from the code/message values, because it is the property the six
// route migrations depend on: a client may ALWAYS read `error.code`.
func TestRespondJSON_ErrorIsNeverAString(t *testing.T) {
	casos := []struct {
		nome string
		err  error
	}{
		{"erro tipado", apperr.New("x_failed", apperr.CategoryValidation, "mensagem segura", false, nil)},
		{"erro tipado embrulhado", fmt.Errorf("camada acima: %w", apperr.New("y_failed", apperr.CategoryNotFound, "não existe", false, nil))},
		{"erro nu", errors.New("boom")},
		{"erro nu embrulhado", fmt.Errorf("camada acima: %w", errors.New("boom"))},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			rec := httptest.NewRecorder()
			RespondJSON(rec, http.StatusInternalServerError, nil, caso.err)
			envelope := decodeEnvelope(t, rec)
			if _, ehTexto := envelope["error"].(string); ehTexto {
				t.Fatalf("error veio como TEXTO (%q): o contrato exige objecto para todo erro", envelope["error"])
			}
			errObj, ok := envelope["error"].(map[string]interface{})
			if !ok {
				t.Fatalf("error = %#v, want object", envelope["error"])
			}
			if code, _ := errObj["code"].(string); code == "" {
				t.Error("error.code vazio")
			}
			if msg, _ := errObj["message"].(string); msg == "" {
				t.Error("error.message vazia")
			}
		})
	}
}

// TestRespondJSON_Success proves the success envelope: data flows through
// as-is (json-marshaled once, not pre-serialized then re-parsed — the
// dados/F12 bug this phase fixed), and success is true.
func TestRespondJSON_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondJSON(rec, http.StatusOK, map[string]interface{}{"webhook": "https://example.com"}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	envelope := decodeEnvelope(t, rec)
	if got := envelope["success"]; got != true {
		t.Errorf("envelope[\"success\"] = %v, want true", got)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("envelope[\"data\"] = %#v, want an object", envelope["data"])
	}
	if data["webhook"] != "https://example.com" {
		t.Errorf("data.webhook = %v, want %q", data["webhook"], "https://example.com")
	}
	if _, hasError := envelope["error"]; hasError {
		t.Error("success envelope should not have an \"error\" key")
	}
}
