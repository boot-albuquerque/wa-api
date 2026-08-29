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

	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/auth"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/wa-noise/observability/applog"
	"wa-api/pkg/presentation/http/handlers"
)

// CAP-28 — POST /admin/users e a chave HMAC do corpo (HOUSEKEEP F158).
//
// Estes testes exercitam a ROTA REGISTRADA (registerAdminRoutes +
// gorilla/mux) contra SQLite real com o schema de produção e o AES-GCM real
// de pkg/infra/auth. O dublê ficaria de fora justamente do que importa: a
// prova de que o que foi GRAVADO decifra de volta para o texto plano. Um
// cifrador dublê faria essa asserção passar com o defeito no lugar — que é
// como o defeito sobreviveu à suíte antiga, que só assegurava
// `len(rec.HmacKey) != 0` (ARMADILHA 2).

// addUserTestEncryptionKey tem 32 bytes porque AES-256 exige exatamente isso
// — aes.NewCipher recusa qualquer outro tamanho (pkg/infra/auth/hmac.go:46).
const addUserTestEncryptionKey = "0123456789abcdef0123456789abcdef"

// addUserTestPlainKey é a chave HMAC do usuário, com os 32 caracteres que
// domain.MinHmacKeyLength exige.
const addUserTestPlainKey = "chave-hmac-de-trinta-e-dois-carA"

// addUserTestShortKey tem 31 caracteres — um a menos que o piso.
const addUserTestShortKey = "chave-hmac-de-trinta-e-um-carAc"

const addUserTestToken = "token-cap28"

type addUserRouteFixture struct {
	db     *sqlx.DB
	router *mux.Router
	logs   *bytes.Buffer
}

// newAddUserRouteFixture monta banco, roteador e logger reais.
//
// appCtx é global do pacote: o fixture o substitui por um novo e restaura no
// Cleanup, para que um teste não veja a chave de encriptação do outro.
func newAddUserRouteFixture(t *testing.T) *addUserRouteFixture {
	t.Helper()

	anterior := appCtx
	appCtx = NewAppContext()
	appCtx.GlobalEncryptionKey = addUserTestEncryptionKey
	t.Cleanup(func() { appCtx = anterior })

	database := newChatHistoryDB(t)
	logs := &bytes.Buffer{}
	logger := applog.NewZerologAdapter(zerolog.New(logs))

	// As MESMAS dependências que wiring_handlers.go:217 monta em produção:
	// o repositório real e o cifrador real, não um dublê.
	addUserUC := user.NewAddUserUseCase(db.NewUserRepository(database), hmacKeyEncryptor{}, s3SecretCipher{}, logger, true)
	userHandlers := handlers.NewUserHandlers(
		nil, addUserUC, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	ch := &customHandlers{
		User: userHandlers,
		Misc: &handlers.MiscHandlers{},
	}

	router := mux.NewRouter()
	registerAdminRoutes(router.PathPrefix("/admin").Subrouter(), ch)
	return &addUserRouteFixture{db: database, router: router, logs: logs}
}

// postUser dispara POST /admin/users pela rota registrada.
func (f *addUserRouteFixture) postUser(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/users", strings.NewReader(body))
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// storedHmacKey devolve a coluna users.hmac_key do usuário do token dado, e
// se a linha existe.
//
// A busca é por token_hash, e não por token, porque a coluna `token` recebe
// VAZIO desde a F97 (pkg/infra/db/user_repository.go:73): procurar pelo texto
// claro não acharia linha nenhuma e faria o teste de "nada foi gravado"
// passar mesmo com o usuário criado.
func (f *addUserRouteFixture) storedHmacKey(t *testing.T, token string) ([]byte, bool) {
	t.Helper()
	var keys [][]byte
	if err := f.db.Select(&keys, f.db.Rebind(`SELECT hmac_key FROM users WHERE token_hash = ?`), domain.HashToken(token)); err != nil {
		t.Fatalf("select hmac_key: %v", err)
	}
	if len(keys) == 0 {
		return nil, false
	}
	if len(keys) > 1 {
		t.Fatalf("linhas para o token %q = %d, queria no máximo 1", token, len(keys))
	}
	return keys[0], true
}

func TestAdminAddUser_ChaveHmacGravadaCifradaEDecifraDeVolta(t *testing.T) {
	f := newAddUserRouteFixture(t)

	rec := f.postUser(t, `{"name":"alice","token":"`+addUserTestToken+`","hmac_key":"`+addUserTestPlainKey+`","engine":"wa_noise"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, queria %d (corpo: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	stored, ok := f.storedHmacKey(t, addUserTestToken)
	if !ok {
		t.Fatal("usuário não foi criado")
	}
	if len(stored) == 0 {
		t.Fatal("hmac_key gravada vazia")
	}
	// Ida: o que está na coluna NÃO é o texto plano.
	if bytes.Equal(stored, []byte(addUserTestPlainKey)) {
		t.Error("hmac_key gravada em CLARO — é o texto plano do corpo")
	}
	// Volta: e é AES-GCM que decifra para exatamente o texto plano. Só a ida
	// não bastaria: qualquer transformação (base64, hash) passaria nela e
	// deixaria o webhook sem assinatura do mesmo jeito.
	plain, err := auth.DecryptHMACKey(stored, []byte(addUserTestEncryptionKey))
	if err != nil {
		t.Fatalf("DecryptHMACKey sobre o valor gravado: %v", err)
	}
	if plain != addUserTestPlainKey {
		t.Errorf("decifrado = %q, queria %q", plain, addUserTestPlainKey)
	}

	// O segredo não volta na resposta nem entra no log.
	if strings.Contains(rec.Body.String(), addUserTestPlainKey) {
		t.Errorf("a chave em claro apareceu na resposta: %s", rec.Body.String())
	}
	if strings.Contains(f.logs.String(), addUserTestPlainKey) {
		t.Errorf("a chave em claro apareceu no log: %s", f.logs.String())
	}

	var envelope struct {
		Data struct {
			HmacConfigured bool `json:"hmac_configured"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if !envelope.Data.HmacConfigured {
		t.Error("hmac_configured = false, queria true")
	}
}

func TestAdminAddUser_CifraFalhaNaoCriaUsuario(t *testing.T) {
	f := newAddUserRouteFixture(t)
	// Chave de encriptação global ausente é a falha REAL de EncryptHMACKey
	// (pkg/infra/auth/hmac.go:39), não um erro inventado por dublê.
	appCtx.GlobalEncryptionKey = ""

	rec := f.postUser(t, `{"name":"alice","token":"`+addUserTestToken+`","hmac_key":"`+addUserTestPlainKey+`","engine":"wa_noise"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, queria %d (corpo: %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	// Falha FECHADA: nada gravado. Criar o usuário sem chave seria pior que
	// recusar — o operador veria 200 e um usuário sem HMAC nenhum.
	if _, ok := f.storedHmacKey(t, addUserTestToken); ok {
		t.Error("usuário criado mesmo com a cifra falhando")
	}
	if strings.Contains(f.logs.String(), addUserTestPlainKey) {
		t.Errorf("a chave em claro apareceu no log de erro: %s", f.logs.String())
	}
}

func TestAdminAddUser_ChaveHmacCurtaERecusada(t *testing.T) {
	f := newAddUserRouteFixture(t)

	rec := f.postUser(t, `{"name":"alice","token":"`+addUserTestToken+`","hmac_key":"`+addUserTestShortKey+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, queria %d (corpo: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if _, ok := f.storedHmacKey(t, addUserTestToken); ok {
		t.Error("usuário criado com chave HMAC abaixo do piso")
	}
}

func TestAdminAddUser_SemChaveHmacCriaUsuario(t *testing.T) {
	f := newAddUserRouteFixture(t)

	rec := f.postUser(t, `{"name":"alice","token":"`+addUserTestToken+`","engine":"wa_noise"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, queria %d (corpo: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	stored, ok := f.storedHmacKey(t, addUserTestToken)
	if !ok {
		t.Fatal("usuário não foi criado")
	}
	if len(stored) != 0 {
		t.Errorf("hmac_key = %v, queria vazia", stored)
	}
}
