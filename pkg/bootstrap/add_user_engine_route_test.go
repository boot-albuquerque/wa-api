package bootstrap

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/noise/observability/applog"
	"wa-api/pkg/presentation/http/handlers"
)

// Engine obrigatório na criação (itens 4-5) e imutável na edição (itens
// 8, 61) do prompt arquitetural — exercitado pela ROTA REGISTRADA
// (registerAdminRoutes + gorilla/mux) contra SQLite real, como o resto da
// suíte CAP-28 (ver add_user_hmac_route_test.go).

const engineRouteToken = "token-engine-route"

type engineRouteFixture struct {
	db     *sqlx.DB
	router *mux.Router
	logs   *bytes.Buffer
}

func newEngineRouteFixture(t *testing.T) *engineRouteFixture {
	t.Helper()

	anterior := appCtx
	appCtx = NewAppContext()
	appCtx.GlobalEncryptionKey = addUserTestEncryptionKey
	t.Cleanup(func() { appCtx = anterior })

	database := newChatHistoryDB(t)
	logs := &bytes.Buffer{}
	logger := applog.NewZerologAdapter(zerolog.New(logs))

	repo := db.NewUserRepository(database)
	addUserUC := user.NewAddUserUseCase(repo, hmacKeyEncryptor{}, s3SecretCipher{}, logger, true)
	editUserUC := user.NewEditUserUseCase(repo, s3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, logger)
	listUsersUC := user.NewListUsersUseCase(repo, logger, &contractsfake.SessionStatusReader{})

	userHandlers := handlers.NewUserHandlers(
		listUsersUC, addUserUC, editUserUC, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	ch := &customHandlers{
		User: userHandlers,
		Misc: &handlers.MiscHandlers{},
	}

	router := mux.NewRouter()
	registerAdminRoutes(router.PathPrefix("/admin").Subrouter(), ch)
	return &engineRouteFixture{db: database, router: router, logs: logs}
}

func (f *engineRouteFixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f *engineRouteFixture) postUser(t *testing.T, body string) *httptest.ResponseRecorder {
	return f.do(t, http.MethodPost, "/admin/users", body)
}

// errorCode extrai error.code do envelope ADR-002.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v (corpo: %s)", err, rec.Body.String())
	}
	return envelope.Error.Code
}

// TestAdminAddUser_EngineAusenteNuloVazioOuInvalidoE400 trava os itens 4-5:
// nenhum destes corpos cria o usuário.
func TestAdminAddUser_EngineAusenteNuloVazioOuInvalidoE400(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "ausente", body: `{"name":"alice","token":"` + engineRouteToken + `"}`},
		{name: "nulo", body: `{"name":"alice","token":"` + engineRouteToken + `","engine":null}`},
		{name: "vazio", body: `{"name":"alice","token":"` + engineRouteToken + `","engine":""}`},
		{name: "inválido", body: `{"name":"alice","token":"` + engineRouteToken + `","engine":"postgres"}`},
		{name: "legacy_unknown", body: `{"name":"alice","token":"` + engineRouteToken + `","engine":"legacy_unknown"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEngineRouteFixture(t)
			rec := f.postUser(t, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, queria 400 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if code := errorCode(t, rec); code != "invalid_engine" {
				t.Errorf("error.code = %q, queria %q", code, "invalid_engine")
			}
			var count int
			if err := f.db.Get(&count, `SELECT COUNT(*) FROM users`); err != nil {
				t.Fatalf("count users: %v", err)
			}
			if count != 0 {
				t.Errorf("usuários gravados = %d, queria 0", count)
			}
		})
	}
}

// TestAdminAddUser_EngineValidoCriaEListagemMostraOMotor trava o caminho de
// SUCESSO (item 3/9): noise e headless criam a conta, e GET
// /admin/users devolve o campo "engine".
func TestAdminAddUser_EngineValidoCriaEListagemMostraOMotor(t *testing.T) {
	for _, engine := range []string{"noise", "headless"} {
		t.Run(engine, func(t *testing.T) {
			f := newEngineRouteFixture(t)
			rec := f.postUser(t, `{"name":"alice","token":"`+engineRouteToken+`","engine":"`+engine+`"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, queria 200 (corpo: %s)", rec.Code, rec.Body.String())
			}

			listRec := f.do(t, http.MethodGet, "/admin/users", "")
			if listRec.Code != http.StatusOK {
				t.Fatalf("GET /admin/users status = %d (corpo: %s)", listRec.Code, listRec.Body.String())
			}
			var envelope struct {
				Data []struct {
					Engine string `json:"engine"`
				} `json:"data"`
			}
			if err := json.Unmarshal(listRec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode: %v (corpo: %s)", err, listRec.Body.String())
			}
			if len(envelope.Data) != 1 {
				t.Fatalf("usuários listados = %d, queria 1", len(envelope.Data))
			}
			if envelope.Data[0].Engine != engine {
				t.Errorf("engine na listagem = %q, queria %q", envelope.Data[0].Engine, engine)
			}
		})
	}
}

// TestAdminEditUser_EngineDivergenteE409SemTocarOBanco trava os itens 8/61:
// PUT tentando mudar o engine é recusado, e a coluna no banco continua com o
// valor original — não é só o status HTTP que prova isso.
func TestAdminEditUser_EngineDivergenteE409SemTocarOBanco(t *testing.T) {
	f := newEngineRouteFixture(t)
	createRec := f.postUser(t, `{"name":"alice","token":"`+engineRouteToken+`","engine":"noise"}`)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status = %d (corpo: %s)", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	editRec := f.do(t, http.MethodPut, "/admin/users/"+created.Data.ID, `{"engine":"headless"}`)
	if editRec.Code != http.StatusConflict {
		t.Fatalf("edit status = %d, queria 409 (corpo: %s)", editRec.Code, editRec.Body.String())
	}
	if code := errorCode(t, editRec); code != "engine_immutable" {
		t.Errorf("error.code = %q, queria %q", code, "engine_immutable")
	}

	var stored string
	if err := f.db.Get(&stored, `SELECT engine FROM users WHERE id = ?`, created.Data.ID); err != nil {
		t.Fatalf("select engine: %v", err)
	}
	if stored != "noise" {
		t.Errorf("engine no banco = %q, queria %q — a tentativa recusada não pode ter tocado a coluna", stored, "noise")
	}
}
