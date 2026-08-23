package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"wa-api/pkg/infra/db"
)

// F191, camada HTTP. As rotas são testadas pelo ROUTER REGISTRADO e não pelo
// handler cru: o router deste projeto é o gorilla/mux, e um handler montado
// sem padrão de rota nunca exercita a extração do `{id}` — foi assim que a F81
// sobreviveu (armadilha nº9 do ARMADILHAS.md).

func labelTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite", t.TempDir()+"/labels.db"+db.SQLitePragmas)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return database
}

// labelRouter monta as DUAS rotas como o wiring as regista.
func labelRouter(repo *db.LabelRepository) *mux.Router {
	h := NewLabelHandlers(repo)
	r := mux.NewRouter()
	r.Handle("/labels", h.ListLabels).Methods(http.MethodGet)
	r.Handle("/labels/{id}/chats", h.ListLabelChat).Methods(http.MethodGet)
	return r
}

func labelGet(t *testing.T, r *mux.Router, path, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if userID != "" {
		req = withUser(req, userID)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestListLabels_DevolveAsGravadas(t *testing.T) {
	database := labelTestDB(t)
	repo := db.NewLabelRepository(database)
	agora := time.Now().UTC().Truncate(time.Second)

	if err := repo.UpsertLabel(context.Background(), "u1",
		db.Label{LabelID: "L1", Name: "Clientes", Color: 2, UpdatedAt: agora}); err != nil {
		t.Fatalf("UpsertLabel: %v", err)
	}
	// De OUTRO utilizador: não pode aparecer. Vazar etiqueta entre sessões
	// seria pior que não ter a rota.
	if err := repo.UpsertLabel(context.Background(), "u2",
		db.Label{LabelID: "L9", Name: "Alheia", UpdatedAt: agora}); err != nil {
		t.Fatalf("UpsertLabel: %v", err)
	}

	rec := labelGet(t, labelRouter(repo), "/labels", "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}

	var env struct {
		Data []db.Label `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("resposta não é o envelope: %v (%s)", err, rec.Body.String())
	}
	if len(env.Data) != 1 || env.Data[0].LabelID != "L1" || env.Data[0].Name != "Clientes" {
		t.Fatalf("data = %+v, quero só a etiqueta de u1", env.Data)
	}
}

// TestListLabels_VaziaSaiComoLista: `null` rebenta um `for...of` no cliente;
// `[]` não.
func TestListLabels_VaziaSaiComoLista(t *testing.T) {
	rec := labelGet(t, labelRouter(db.NewLabelRepository(labelTestDB(t))), "/labels", "u1")

	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if string(env.Data) != "[]" {
		t.Fatalf("data = %s, quero [] — null rebenta o for...of do cliente", env.Data)
	}
}

// TestListLabelChats_UsaOIdDaRota é o teste da armadilha nº9: com o handler
// montado sem padrão, `mux.Vars` devolveria vazio e a rota responderia a lista
// de outra etiqueta — ou de nenhuma — com 200.
func TestListLabelChats_UsaOIdDaRota(t *testing.T) {
	database := labelTestDB(t)
	repo := db.NewLabelRepository(database)
	agora := time.Now().UTC()

	for _, c := range []db.LabelChat{
		{LabelID: "L1", ChatJID: "aaa@s.whatsapp.net", Labeled: true, UpdatedAt: agora},
		{LabelID: "L2", ChatJID: "bbb@s.whatsapp.net", Labeled: true, UpdatedAt: agora},
	} {
		if err := repo.SetChatLabel(context.Background(), "u1", c); err != nil {
			t.Fatalf("SetChatLabel: %v", err)
		}
	}

	rec := labelGet(t, labelRouter(repo), "/labels/L2/chats", "u1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, quero 200 (corpo %s)", rec.Code, rec.Body.String())
	}

	var env struct {
		Data []db.LabelChat `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if len(env.Data) != 1 || env.Data[0].ChatJID != "bbb@s.whatsapp.net" {
		t.Fatalf("data = %+v, quero só a conversa de L2 — o {id} da rota não foi lido", env.Data)
	}
}

func TestListLabels_SemAutenticacaoRecusa(t *testing.T) {
	rec := labelGet(t, labelRouter(db.NewLabelRepository(labelTestDB(t))), "/labels", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, quero 401", rec.Code)
	}
}

// labelRouterQuebrado monta as rotas sobre um banco SEM as tabelas de
// etiqueta: a falha do repositório é REAL, do driver, e não a minha ideia de
// como ele falharia (armadilha nº1).
func labelRouterQuebrado(t *testing.T) *mux.Router {
	t.Helper()
	database := labelTestDB(t)
	for _, tbl := range []string{"wa_label_messages", "wa_label_chats", "wa_labels"} {
		if _, err := database.Exec("DROP TABLE IF EXISTS " + tbl); err != nil {
			t.Fatalf("drop %s: %v", tbl, err)
		}
	}
	return labelRouter(db.NewLabelRepository(database))
}

// TestListLabels_FalhaDoBancoE500ENaoListaVazia trava a distinção que importa:
// uma avaria NÃO pode sair como `{"data":[]}`. O cliente leria "não há
// etiquetas" e pararia de tentar — que é o mesmo defeito da F182, noutra rota.
func TestListLabels_FalhaDoBancoE500ENaoListaVazia(t *testing.T) {
	rec := labelGet(t, labelRouterQuebrado(t), "/labels", "u1")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, quero 500 (corpo %s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("avaria saiu como lista vazia: %s", rec.Body.String())
	}
}

func TestListLabelChats_FalhaDoBancoE500(t *testing.T) {
	rec := labelGet(t, labelRouterQuebrado(t), "/labels/L1/chats", "u1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, quero 500 (corpo %s)", rec.Code, rec.Body.String())
	}
}

// TestListLabelChats_SemAutenticacaoRecusa: o outro ramo de saída do handler.
func TestListLabelChats_SemAutenticacaoRecusa(t *testing.T) {
	rec := labelGet(t, labelRouter(db.NewLabelRepository(labelTestDB(t))), "/labels/L1/chats", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, quero 401", rec.Code)
	}
}
