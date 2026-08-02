package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
)

// captureGlobalLog redireciona o logger de pacote para um buffer pelo tempo
// do teste.
func captureGlobalLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = original })
	return &buf
}

// serveWithRequestLogger roda handler com um logger de requisição instalado,
// como o faz a cadeia hlog de bootstrap/router.go, para que
// hlog.FromRequest(r) escreva em vez de descartar.
func serveWithRequestLogger(t *testing.T, handler http.Handler, r *http.Request) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	rec := httptest.NewRecorder()
	hlog.NewHandler(zerolog.New(&buf))(handler).ServeHTTP(rec, r)
	return rec, &buf
}

// TestRespondJSONMarshalFailureLogsAndFallsBack cobre o único caminho em que
// o corpo entregue não é o corpo pretendido: json.Marshal falha depois de o
// status já ter sido escrito, então o WriteHeader(500) seguinte é um no-op e
// o cliente recebe 200-com-corpo-de-erro. Nada nessa situação é observável
// pela resposta — o registro de log é a única evidência, e é por isso que
// ele é testado junto com o corpo.
func TestRespondJSONMarshalFailureLogsAndFallsBack(t *testing.T) {
	logs := captureGlobalLog(t)

	rec := httptest.NewRecorder()
	// Um canal não tem representação JSON: json.Marshal do envelope falha.
	RespondJSON(rec, http.StatusOK, make(chan int), nil)

	body := rec.Body.String()
	if !strings.Contains(body, `"error":"internal server error"`) {
		t.Errorf("body = %q, want the generic fallback envelope", body)
	}
	if strings.Contains(body, `"success"`) {
		t.Errorf("fallback body should be the minimal envelope, got %q", body)
	}

	out := logs.String()
	if !strings.Contains(out, "failed to marshal JSON response envelope") {
		t.Fatalf("marshal failure was not logged; logs: %s", out)
	}
	if !strings.Contains(out, `"level":"error"`) {
		t.Errorf("marshal failure was not logged at ERROR; logs: %s", out)
	}
	if !strings.Contains(out, `"status":200`) {
		t.Errorf("log record carries no status field; logs: %s", out)
	}
}

// TestRespondJSONSuccessDoesNotLog: o caminho normal escreve uma resposta por
// request e não pode acrescentar um registro por resposta — seria dobrar o
// volume de log do serviço inteiro sem acrescentar informação ao registro de
// fronteira que hlog.AccessHandler já emite.
func TestRespondJSONSuccessDoesNotLog(t *testing.T) {
	logs := captureGlobalLog(t)

	RespondJSON(httptest.NewRecorder(), http.StatusOK, map[string]string{"a": "b"}, nil)
	RespondJSON(httptest.NewRecorder(), http.StatusBadRequest, nil, errors.New("boom"))

	if logs.Len() != 0 {
		t.Errorf("RespondJSON logged on a path that writes a well-formed envelope: %s", logs.String())
	}
}

// TestProfileHandlerLogsRejections prova que as três saídas de erro do
// handler deixam rastro no logger de requisição — a decisão de negar é
// tomada aqui e o corpo devolvido é deliberadamente genérico, então sem o
// registro não há como distinguir "sem userinfo" de "sem session id" depois
// do fato.
func TestProfileHandlerLogsRejections(t *testing.T) {
	handler := NewProfileHandler(&mockGetProfileUseCase{err: errors.New("upstream down")})

	tests := []struct {
		name       string
		ctxValue   interface{}
		wantStatus int
		wantLevel  string
		wantMsg    string
	}{
		{"no user info", nil, http.StatusUnauthorized, "warn", "profile request without user info in context"},
		{"empty session id", &mockUserInfo{id: ""}, http.StatusBadRequest, "warn", "profile request with empty session id"},
		{"use case failure", &mockUserInfo{id: "u1"}, http.StatusInternalServerError, "error", "get profile use case failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/session/profile", nil)
			if tt.ctxValue != nil {
				r = r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, tt.ctxValue))
			}

			rec, logs := serveWithRequestLogger(t, handler, r)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			out := logs.String()
			if !strings.Contains(out, tt.wantMsg) {
				t.Errorf("logs do not contain %q; logs: %s", tt.wantMsg, out)
			}
			if !strings.Contains(out, `"level":"`+tt.wantLevel+`"`) {
				t.Errorf("record not logged at %s; logs: %s", tt.wantLevel, out)
			}
		})
	}
}

// TestProfileHandlerSuccessDoesNotLogAtWarn: só as saídas de erro logam. Um
// WARN no caminho feliz tornaria o sinal inútil.
func TestProfileHandlerSuccessDoesNotLogAtWarn(t *testing.T) {
	handler := NewProfileHandler(&mockGetProfileUseCase{result: `{"id":"u1"}`})
	r := httptest.NewRequest(http.MethodGet, "/session/profile", nil)
	r = r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, &mockUserInfo{id: "u1"}))

	rec, logs := serveWithRequestLogger(t, handler, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if out := logs.String(); strings.Contains(out, `"level":"warn"`) || strings.Contains(out, `"level":"error"`) {
		t.Errorf("successful profile request emitted a warn/error record: %s", out)
	}
}
