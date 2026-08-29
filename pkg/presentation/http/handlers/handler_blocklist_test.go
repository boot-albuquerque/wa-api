package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
	"wa-api/pkg/presentation/http/contracttest"
)

// GET /user/blocklist ERA a unica rota do pacote que NAO respondia no
// envelope do ADR-002 no caminho feliz: escrevia o JSON cru "para preservar o
// formato legado". A migracao de DTO da familia de utilizadores acabou com a
// excecao, e este teste passou a afirmar o envelope como todas as outras.

func blNewHandler(bm *contractsfake.BlocklistManager) *GetBlocklistHandler {
	return NewGetBlocklistHandler(user.NewGetBlocklistUseCase(bm, &contractsfake.Logger{}))
}

func TestGetBlocklistHandler_Sucesso(t *testing.T) {
	bm := &contractsfake.BlocklistManager{}
	bm.GetBlocklistFunc = func(context.Context, string) (domain.Blocklist, error) {
		return domain.Blocklist{JIDs: []string{"5511999@s.whatsapp.net"}, DHash: "h1"}, nil
	}

	rec, capture := uhServe(blNewHandler(bm),
		withUser(uhRequest(http.MethodGet, "/user/blocklist", "", nil), "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool `json:"success"`
		Code    int  `json:"code"`
		Data    struct {
			Blocklist []string `json:"blocklist"`
			DHash     string   `json:"dhash"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("corpo nao e' o envelope canonico: %v (%s)", err, rec.Body.String())
	}
	if !env.Success || env.Code != 200 {
		t.Fatalf("envelope = %+v, quero success=true code=200", env)
	}
	if len(env.Data.Blocklist) != 1 || env.Data.Blocklist[0] != "5511999@s.whatsapp.net" {
		t.Fatalf("blocklist = %#v", env.Data.Blocklist)
	}
	if env.Data.DHash != "h1" {
		t.Fatalf("dhash: got %v, want h1", env.Data.DHash)
	}
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
	contracttest.AssertNoKeys(t, rec.Body.Bytes(), "Blocklist", "DHash")
	logassert.NoSecrets(t, capture.Records(t))
}

func TestGetBlocklistHandler_CaminhosDeRecusa(t *testing.T) {
	sessionBoom := errors.New("bl-no-session")
	portBoom := errors.New("bl-port-boom")

	cases := []struct {
		name             string
		arrange          func(*contractsfake.BlocklistManager)
		request          func() *http.Request
		want             int
		wantErrSubstring string
	}{
		{
			name:    "sem userinfo no contexto",
			request: func() *http.Request { return uhRequest(http.MethodGet, "/user/blocklist", "", nil) },
			want:    http.StatusUnauthorized, wantErrSubstring: errUnauthorized.Error(),
		},
		{
			name: "tipo errado no contexto",
			request: func() *http.Request {
				r := uhRequest(http.MethodGet, "/user/blocklist", "", nil)
				return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
			},
			want: http.StatusUnauthorized, wantErrSubstring: errUnauthorized.Error(),
		},
		{
			name: "session id vazio",
			request: func() *http.Request {
				return withUser(uhRequest(http.MethodGet, "/user/blocklist", "", nil), "")
			},
			want: http.StatusBadRequest, wantErrSubstring: errMissingSessionID.Error(),
		},
		{
			name: "sessao recusada pela porta",
			arrange: func(bm *contractsfake.BlocklistManager) {
				bm.EnsureSessionFunc = func(context.Context, string) error { return sessionBoom }
			},
			request: func() *http.Request {
				return withUser(uhRequest(http.MethodGet, "/user/blocklist", "", nil), "u-1")
			},
			want: http.StatusInternalServerError, wantErrSubstring: sessionBoom.Error(),
		},
		{
			name: "leitura da blocklist falha",
			arrange: func(bm *contractsfake.BlocklistManager) {
				bm.GetBlocklistFunc = func(context.Context, string) (domain.Blocklist, error) {
					return domain.Blocklist{}, portBoom
				}
			},
			request: func() *http.Request {
				return withUser(uhRequest(http.MethodGet, "/user/blocklist", "", nil), "u-1")
			},
			want: http.StatusInternalServerError, wantErrSubstring: portBoom.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bm := &contractsfake.BlocklistManager{}
			if tc.arrange != nil {
				tc.arrange(bm)
			}

			rec, capture := uhServe(blNewHandler(bm), tc.request())

			assertErrorEnvelope(t, rec, tc.want)
			logassert.OutcomeLogged(t, capture.Records(t), tc.wantErrSubstring)
		})
	}
}

// TestGetBlocklistHandler_NaoAlcancaAPortaSemSessao: as recusas de fronteira
// tem de acontecer ANTES de falar com o WhatsApp.
func TestGetBlocklistHandler_NaoAlcancaAPortaSemSessao(t *testing.T) {
	for _, r := range []*http.Request{
		uhRequest(http.MethodGet, "/user/blocklist", "", nil),
		withUser(uhRequest(http.MethodGet, "/user/blocklist", "", nil), ""),
	} {
		bm := &contractsfake.BlocklistManager{}
		rec := httptest.NewRecorder()
		blNewHandler(bm).ServeHTTP(rec, r)

		if len(bm.EnsureSessionCalls) != 0 || len(bm.GetBlocklistCalls) != 0 {
			t.Fatalf("recusa de fronteira alcancou a porta (status %d)", rec.Code)
		}
	}
}
