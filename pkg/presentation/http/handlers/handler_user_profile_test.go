package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain/apperr"
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

// F238: malformed JID in path returns 400 invalid_jid, not 500.

func TestGetUserProfile_MalformedJID_Returns400(t *testing.T) {
	porta := &lidPorta{lid: "1@lid"}

	cases := []struct {
		name string
		jid  string
	}{
		{"garbage_string", "not-a-phone"},
		{"too_short", "123"},
		{"has_letters", "551abc9999"},
		{"empty_via_router", "%20"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := withUser(httptest.NewRequest(http.MethodGet, "/user/profile/"+tc.jid, nil), "user-1")
			rec := httptest.NewRecorder()
			rotaPerfil(t, porta).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400 for malformed jid %q (body: %s)", rec.Code, tc.jid, rec.Body.String())
			}

			var env struct {
				Code  int `json:"code"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("response is not valid JSON: %v", err)
			}
			if env.Error.Code != CodeInvalidJID {
				t.Fatalf("error.code = %q, want %q", env.Error.Code, CodeInvalidJID)
			}
			if porta.recebido != "" {
				t.Fatalf("use case was reached despite 400: received %q", porta.recebido)
			}
		})
	}
}

func TestGetUserProfile_ValidJIDFormats_Pass(t *testing.T) {
	porta := &lidPorta{lid: "1@lid"}

	cases := []struct {
		name string
		jid  string
	}{
		{"qualified_jid", "5516981818244@s.whatsapp.net"},
		{"bare_phone", "5516981818244"},
		{"lid", "29343770251463@lid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			porta.recebido = ""
			req := withUser(httptest.NewRequest(http.MethodGet, "/user/profile/"+tc.jid, nil), "user-1")
			rec := httptest.NewRecorder()
			rotaPerfil(t, porta).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, want 200 for valid jid %q (body: %s)", rec.Code, tc.jid, rec.Body.String())
			}
		})
	}
}

func TestGetUserProfile_MalformedJID_IsAppError(t *testing.T) {
	porta := &lidPorta{lid: "1@lid"}

	req := withUser(httptest.NewRequest(http.MethodGet, "/user/profile/not-a-phone", nil), "user-1")
	rec := httptest.NewRecorder()
	rotaPerfil(t, porta).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}

	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if env.Error.Code != CodeInvalidJID {
		t.Fatalf("error.code = %q, want %q", env.Error.Code, CodeInvalidJID)
	}
	if env.Error.Message == "" {
		t.Fatal("error.message is empty — apperr must carry a human-readable message")
	}
}

// isPlausibleJIDOrPhone unit tests — exercised here because the function is
// unexported and lives in handler_user.go.

func TestIsPlausibleJIDOrPhone(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"5516981818244@s.whatsapp.net", true},
		{"29343770251463@lid", true},
		{"group@g.us", true},
		{"5516981818244", true},
		{"5511999999999", true},
		{"1234567", true},
		{"12345678901234567890", true},
		// rejections
		{"", false},
		{"123", false},
		{"abc", false},
		{"123456789012345678901", false},
		{"551abc9999", false},
		{"not-a-phone", false},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := isPlausibleJIDOrPhone(tc.input)
			if got != tc.want {
				t.Fatalf("isPlausibleJIDOrPhone(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// normalizePhones unit tests.

func TestNormalizePhones(t *testing.T) {
	input := []string{"5511999", "5511888@s.whatsapp.net", "group@g.us"}
	got := normalizePhones(input)

	want := []string{
		"5511999@s.whatsapp.net",
		"5511888@s.whatsapp.net",
		"group@g.us",
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("normalizePhones[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizePhones_Empty(t *testing.T) {
	got := normalizePhones(nil)
	if len(got) != 0 {
		t.Fatalf("normalizePhones(nil) returned %d items, want 0", len(got))
	}
}

// F236 sentinel contract: middleware errUnauthorized is *apperr.AppError.

func TestMiddleware_ErrUnauthorized_IsAppError(t *testing.T) {
	var appErr *apperr.AppError
	if !errors.As(errUnauthorized, &appErr) {
		t.Fatal("errUnauthorized is not *apperr.AppError")
	}
	if appErr.Code != CodeUnauthorized {
		t.Fatalf("code = %q, want %q", appErr.Code, CodeUnauthorized)
	}
	if appErr.Category.HTTPStatus() != http.StatusUnauthorized {
		t.Fatalf("HTTPStatus = %d, want 401", appErr.Category.HTTPStatus())
	}
}
