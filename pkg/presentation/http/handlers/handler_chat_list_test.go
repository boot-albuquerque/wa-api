package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// GET /chat/list lê limit e offset da query string. Valor não numérico é
// tratado como AUSENTE, e não como erro: a rota é de leitura, e recusar a
// chamada inteira por um parâmetro decorativo seria desproporcional.

func rotaLista(t *testing.T, quantos int) http.Handler {
	t.Helper()
	atividade := map[string]time.Time{}
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	for i := 0; i < quantos; i++ {
		atividade[string(rune('a'+i))+"@lid"] = base.Add(time.Duration(i) * time.Minute)
	}
	ar := &contractsfake.ChatActivityReader{
		GetLastActivityByUserFunc: func(context.Context, string) (map[string]time.Time, error) {
			return atividade, nil
		},
	}
	uc := user.NewListChatsUseCase(ar, &contractsfake.ContactDirectory{}, &contractsfake.ContactDirectory{}, &contractsfake.GroupDirectory{}, &contractsfake.Logger{})
	h := NewUserHandlers(nil, nil, nil, nil, nil, nil, nil, nil, uc, nil, nil)

	r := mux.NewRouter()
	r.Handle("/chat/list", h.ListChats()).Methods(http.MethodGet)
	return r
}

func pagina(t *testing.T, url string, quantos int) domain.ChatListPage {
	t.Helper()
	req := withUser(httptest.NewRequest(http.MethodGet, url, nil), "user-1")
	rec := httptest.NewRecorder()
	rotaLista(t, quantos).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool                `json:"success"`
		Data    domain.ChatListPage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta nao e' o envelope do ADR-002: %v", err)
	}
	if !env.Success {
		t.Fatalf("success=false: %s", rec.Body.String())
	}
	return env.Data
}

func TestListChats_PadraoSemParametros(t *testing.T) {
	page := pagina(t, "/chat/list", 3)

	if page.Limit != user.LimitPadrao || page.Offset != 0 {
		t.Errorf("limit=%d offset=%d, quero %d e 0", page.Limit, page.Offset, user.LimitPadrao)
	}
	if page.Total != 3 || len(page.Chats) != 3 {
		t.Errorf("total=%d chats=%d, quero 3 e 3", page.Total, len(page.Chats))
	}
}

func TestListChats_LimitEOffsetDaQuery(t *testing.T) {
	page := pagina(t, "/chat/list?limit=2&offset=1", 5)

	if page.Limit != 2 || page.Offset != 1 {
		t.Errorf("limit=%d offset=%d, quero 2 e 1", page.Limit, page.Offset)
	}
	if len(page.Chats) != 2 {
		t.Errorf("veio %d conversas, quero 2", len(page.Chats))
	}
	if page.Total != 5 {
		t.Errorf("total=%d, quero 5 — o total e' da lista inteira", page.Total)
	}
}

// TestListChats_QueryInvalidaNaoEhErro: `?limit=abc` recebe o padrão, não um
// 400. Um cliente que erra um parâmetro opcional ainda consegue ler a lista.
func TestListChats_QueryInvalidaNaoEhErro(t *testing.T) {
	page := pagina(t, "/chat/list?limit=abc&offset=xyz", 3)

	if page.Limit != user.LimitPadrao || page.Offset != 0 {
		t.Errorf("limit=%d offset=%d, quero o padrao %d e 0", page.Limit, page.Offset, user.LimitPadrao)
	}
}

func TestListChats_SemUserInfo_401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/chat/list", nil)
	rec := httptest.NewRecorder()
	rotaLista(t, 3).ServeHTTP(rec, req)

	assertErrorEnvelope(t, rec, http.StatusUnauthorized)
}
