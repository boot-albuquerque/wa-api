package bootstrap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/justinas/alice"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/auth"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/wa-noise/observability/applog"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/handlers"
)

// CAP-27 — POST /hmac/configure, GET /hmac/config, DELETE /hmac/config.
//
// Estes testes exercitam a ROTA REGISTRADA (registerCustomRoutes +
// gorilla/mux), e nao o handler cru: a ARMADILHA 2 deste repo e' exatamente
// isso. As duas familias de caminho — `/hmac/config` e `/session/hmac/config`
// — sao exercitadas, porque a segunda passa por um wrapper que despacha por
// METODO (wiring_routes.go:187), e um `switch` errado ali mandaria o DELETE
// para o handler de leitura sem que nenhum teste de handler percebesse.
//
// Nada aqui e' dublê do lado que importa: SQLite real com o schema de
// producao, o AES-GCM real de pkg/infra/auth, e o appCtx.UserInfoCache real —
// que e' de onde lifecycle_webhook.go:178 tira a chave para assinar cada
// webhook por usuario. Um dublê de cache nao teria como provar a revogacao.

// hmacTestEncryptionKey tem 32 bytes porque AES-256 exige exatamente isso —
// aes.NewCipher recusa qualquer outro tamanho (pkg/infra/auth/hmac.go:46).
const hmacTestEncryptionKey = "0123456789abcdef0123456789abcdef"

// hmacTestPlainKey e' a chave HMAC do usuario, com os 32 caracteres que
// domain.MinHmacKeyLength exige.
const hmacTestPlainKey = "chave-hmac-de-trinta-e-dois-carA"

// hmacTestUserID e' o dono da chave em todos os casos abaixo.
const hmacTestUserID = "user-hmac-1"

type hmacRouteFixture struct {
	db     *sqlx.DB
	router *mux.Router
}

// newHmacRouteFixture monta banco, cache e roteador reais.
//
// appCtx e' global do pacote: o fixture o substitui por um novo e restaura no
// Cleanup, para que um teste nao veja o cache do outro.
func newHmacRouteFixture(t *testing.T) *hmacRouteFixture {
	t.Helper()

	anterior := appCtx
	appCtx = NewAppContext()
	appCtx.GlobalEncryptionKey = hmacTestEncryptionKey
	t.Cleanup(func() { appCtx = anterior })

	database := newChatHistoryDB(t)
	logger := applog.NewZerologAdapter(zerolog.Nop())

	store := db.NewHmacConfigRepository(database)
	ch := &customHandlers{
		Profile:     &customhttp.ProfileHandler{},
		ProfileFull: &customhttp.ProfileFullHandler{},
		Message:     &MessageHandlers{},
		Session:     &SessionHandlers{},
		Webhook:     &WebhookHandlers{},
		User:        &handlers.UserHandlers{},
		Group:       &handlers.GroupHandlers{},
		Misc:        &handlers.MiscHandlers{},
		Blocklist:   &handlers.BlocklistHandlers{},
		Download:    &handlers.DownloadHandlers{},
		Presence:    &handlers.PresenceHandlers{},
		Reaction:    &handlers.ReactionHandlers{},
		Contact:     &handlers.ContactHandlers{},
		GroupMgmt:   &handlers.GroupManagementHandlers{},
		Newsletter:  &handlers.NewsletterHandlers{},
		Label:       &handlers.LabelHandlers{},
		ChatHistory: &handlers.ChatHistoryHandlers{},

		// Os tres handlers sob teste, com as MESMAS dependencias que
		// wiring_handlers.go monta em producao.
		Storage: &handlers.StorageHandlers{
			ConfigureHmac: handlers.NewConfigureHmacHandler(
				storage.NewConfigureHmacUseCase(alwaysSessionGuard{}, store, hmacKeyEncryptor{}, userInfoHmacCache{}, logger)),
			GetHmacConfig: handlers.NewGetHmacConfigHandler(
				storage.NewGetHmacConfigUseCase(alwaysSessionGuard{}, store, logger)),
			DeleteHmacConfig: handlers.NewDeleteHmacConfigHandler(
				storage.NewDeleteHmacConfigUseCase(alwaysSessionGuard{}, store, userInfoHmacCache{}, logger)),
		},
	}

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v := Values{M: map[string]string{"Id": hmacTestUserID}}
			next.ServeHTTP(w, r.WithContext(
				context.WithValue(r.Context(), appport.UserInfoKey, v)))
		})
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), ch)
	f := &hmacRouteFixture{db: database, router: router}
	f.seedUser(t)
	return f
}

// seedUser grava a linha de users sem chave HMAC — o estado inicial de quem
// nunca configurou.
func (f *hmacRouteFixture) seedUser(t *testing.T) {
	t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(
		`INSERT INTO users (id, name, token, token_hash) VALUES (?, ?, ?, ?)`),
		hmacTestUserID, "tenant hmac", "token-hmac", "hash-hmac"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

// seedCacheEntry poe o usuario no appCtx.UserInfoCache como
// ensureUserInfoCached o poria. Sem entrada previa o adapter e' no-op por
// desenho, e o teste da revogacao nao teria o que observar.
func (f *hmacRouteFixture) seedCacheEntry(hmacKeyEncrypted string) {
	appCtx.UserInfoCache.Set(hmacTestUserID, Values{M: map[string]string{
		"Id":                 hmacTestUserID,
		"Webhook":            "https://example.invalid/hook",
		userInfoHmacKeyField: hmacKeyEncrypted,
	}}, cache.NoExpiration)
}

// cachedHmac devolve os dois campos do cache que a revogacao tem de zerar.
func (f *hmacRouteFixture) cachedHmac(t *testing.T) (encoded, hasHmac string) {
	t.Helper()
	cached, found := appCtx.UserInfoCache.Get(hmacTestUserID)
	if !found {
		t.Fatal("a entrada do usuario sumiu do UserInfoCache")
	}
	v, ok := cached.(Values)
	if !ok {
		t.Fatalf("a entrada do cache mudou de tipo: %T", cached)
	}
	return v.Get(userInfoHmacKeyField), v.Get(userInfoHasHmacField)
}

// storedHmacKey le a coluna users.hmac_key diretamente — a fonte de verdade,
// sem passar pelo codigo sob teste.
func (f *hmacRouteFixture) storedHmacKey(t *testing.T) []byte {
	t.Helper()
	var chave []byte
	if err := f.db.QueryRowx(f.db.Rebind(`SELECT hmac_key FROM users WHERE id = ?`), hmacTestUserID).Scan(&chave); err != nil {
		t.Fatalf("ler hmac_key: %v", err)
	}
	return chave
}

func (f *hmacRouteFixture) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

// hmacRoutePaths sao as DUAS familias de caminho que chegam aos mesmos
// handlers, com o par (configure, config) de cada uma.
var hmacRoutePaths = []struct {
	name      string
	configure string
	config    string
}{
	{"caminho direto", "/hmac/configure", "/hmac/config"},
	{"caminho /session", "/session/hmac/config", "/session/hmac/config"},
}

// TESTE 1 (rota) — POST grava a chave CIFRADA no banco e publica no cache.
// O valor no banco nao e' o texto plano, e decifra de volta para ele.
func TestHmacRoute_PostGravaCifradoEPublicaNoCache(t *testing.T) {
	for _, rp := range hmacRoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newHmacRouteFixture(t)
			f.seedCacheEntry("")

			rec := f.do(t, http.MethodPost, rp.configure, `{"hmac_key":"`+hmacTestPlainKey+`"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}

			gravado := f.storedHmacKey(t)
			if len(gravado) == 0 {
				t.Fatal("a rota respondeu 200 e NAO gravou nada — o stub da F151 voltou")
			}
			if string(gravado) == hmacTestPlainKey {
				t.Fatal("users.hmac_key guarda a chave EM CLARO")
			}
			decifrada, err := auth.DecryptHMACKey(gravado, []byte(hmacTestEncryptionKey))
			if err != nil {
				t.Fatalf("o valor gravado nao e' AES-GCM da chave global: %v", err)
			}
			if decifrada != hmacTestPlainKey {
				t.Fatalf("decifrado = %q, quero %q", decifrada, hmacTestPlainKey)
			}

			encoded, hasHmac := f.cachedHmac(t)
			if encoded != base64.StdEncoding.EncodeToString(gravado) {
				t.Fatalf("o cache tem %q; quero o base64 do que foi ao banco", encoded)
			}
			if hasHmac != userInfoHasHmacTrue {
				t.Errorf("HasHmac = %q, quero %q", hasHmac, userInfoHasHmacTrue)
			}
		})
	}
}

// TESTE 2 (rota) — chave curta: 400 e NADA gravado.
func TestHmacRoute_PostChaveCurta_400SemGravar(t *testing.T) {
	f := newHmacRouteFixture(t)
	f.seedCacheEntry("")

	rec := f.do(t, http.MethodPost, "/hmac/configure", `{"hmac_key":"curta-demais"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if got := f.storedHmacKey(t); len(got) != 0 {
		t.Fatalf("a recusa gravou %q no banco", got)
	}
	if encoded, _ := f.cachedHmac(t); encoded != "" {
		t.Fatalf("a recusa publicou %q no cache", encoded)
	}
}

// TESTE 3 (rota) — sem chave de encriptacao global a cifra falha: 500, e nada
// gravado. E' o caminho que poria texto claro na coluna se a ordem invertesse.
func TestHmacRoute_PostSemChaveDeEncriptacao_500SemGravar(t *testing.T) {
	f := newHmacRouteFixture(t)
	f.seedCacheEntry("")
	appCtx.GlobalEncryptionKey = ""

	rec := f.do(t, http.MethodPost, "/hmac/configure", `{"hmac_key":"`+hmacTestPlainKey+`"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if got := f.storedHmacKey(t); len(got) != 0 {
		t.Fatalf("gravou %q depois de a cifra falhar", got)
	}
	if encoded, _ := f.cachedHmac(t); encoded != "" {
		t.Fatalf("publicou %q no cache depois de a cifra falhar", encoded)
	}
}

// TESTE 5 e 6 (rota) — a leitura devolve mascara, e o corpo INTEIRO nao
// contem o segredo em nenhuma forma.
func TestHmacRoute_GetMascaraEnaoVaza(t *testing.T) {
	for _, rp := range hmacRoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newHmacRouteFixture(t)
			f.seedCacheEntry("")

			semChave := f.do(t, http.MethodGet, rp.config, "")
			if semChave.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", semChave.Code, semChave.Body.String())
			}
			if got := hmacKeyFromEnvelope(t, semChave); got != "" {
				t.Fatalf("hmac_key = %q sem chave configurada, quero \"\"", got)
			}

			if rec := f.do(t, http.MethodPost, rp.configure, `{"hmac_key":"`+hmacTestPlainKey+`"}`); rec.Code != http.StatusOK {
				t.Fatalf("preparo: POST devolveu %d (%s)", rec.Code, rec.Body.String())
			}
			cifrada := f.storedHmacKey(t)

			comChave := f.do(t, http.MethodGet, rp.config, "")
			if comChave.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", comChave.Code, comChave.Body.String())
			}
			if got := hmacKeyFromEnvelope(t, comChave); got != domain.MaskedHmacKey {
				t.Fatalf("hmac_key = %q, quero %q", got, domain.MaskedHmacKey)
			}

			// Teste NEGATIVO de vazamento, no corpo INTEIRO e nao so' no
			// campo: a chave em claro, a forma cifrada crua e o base64 dela.
			corpo := comChave.Body.String()
			for _, forma := range []struct{ nome, valor string }{
				{"texto plano", hmacTestPlainKey},
				{"cifrado cru", string(cifrada)},
				{"cifrado em base64", base64.StdEncoding.EncodeToString(cifrada)},
			} {
				if strings.Contains(corpo, forma.valor) {
					t.Fatalf("a chave vazou na resposta como %s: %s", forma.nome, corpo)
				}
			}
		})
	}
}

// TESTE 7 (rota) — a REVOGACAO. Banco NULL **e** cache limpo.
//
// A assercao de cache e' a que morde: appCtx.UserInfoCache e'
// cache.NoExpiration, e uma revogacao que limpasse so' o banco devolveria
// exatamente o mesmo 200 com a chave continuando a assinar os webhooks do
// usuario ate' o processo reiniciar (HOUSEKEEP F157).
func TestHmacRoute_DeleteRevogaBancoECache(t *testing.T) {
	for _, rp := range hmacRoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newHmacRouteFixture(t)
			f.seedCacheEntry("")

			if rec := f.do(t, http.MethodPost, rp.configure, `{"hmac_key":"`+hmacTestPlainKey+`"}`); rec.Code != http.StatusOK {
				t.Fatalf("preparo: POST devolveu %d (%s)", rec.Code, rec.Body.String())
			}
			if encoded, _ := f.cachedHmac(t); encoded == "" {
				t.Fatal("preparo: o cache nasceu vazio, entao a revogacao nao teria o que limpar")
			}

			rec := f.do(t, http.MethodDelete, rp.config, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}

			if got := f.storedHmacKey(t); len(got) != 0 {
				t.Fatalf("users.hmac_key continua %q apos a revogacao", got)
			}
			encoded, hasHmac := f.cachedHmac(t)
			if encoded != "" {
				t.Fatalf("HmacKeyEncrypted = %q no cache apos a revogacao: a chave revogada continua assinando os webhooks do usuario", encoded)
			}
			if hasHmac != userInfoHasHmacFalse {
				t.Fatalf("HasHmac = %q apos a revogacao, quero %q", hasHmac, userInfoHasHmacFalse)
			}

			// E a leitura depois da revogacao confirma pelo fio.
			if got := hmacKeyFromEnvelope(t, f.do(t, http.MethodGet, rp.config, "")); got != "" {
				t.Fatalf("GET apos revogar devolveu hmac_key = %q", got)
			}
		})
	}
}

// TESTE 8 (rota) — falha de banco na revogacao: 500 e cache NAO limpo.
//
// A falha e' produzida fechando o banco: o UPDATE volta com erro real do
// driver, e nao com um erro fabricado que so' existe no teste.
func TestHmacRoute_DeleteComBancoQuebrado_500SemLimparOCache(t *testing.T) {
	f := newHmacRouteFixture(t)
	f.seedCacheEntry("")

	if rec := f.do(t, http.MethodPost, "/hmac/configure", `{"hmac_key":"`+hmacTestPlainKey+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("preparo: POST devolveu %d (%s)", rec.Code, rec.Body.String())
	}
	antes, _ := f.cachedHmac(t)
	if antes == "" {
		t.Fatal("preparo: o cache nasceu vazio")
	}

	if err := f.db.Close(); err != nil {
		t.Fatalf("fechar o banco: %v", err)
	}

	rec := f.do(t, http.MethodDelete, "/hmac/config", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500 (corpo: %s)", rec.Code, rec.Body.String())
	}

	depois, hasHmac := f.cachedHmac(t)
	if depois != antes {
		t.Fatalf("o cache foi limpo com a revogacao FALHADA (%q -> %q): o restart traria a chave de volta em silencio", antes, depois)
	}
	if hasHmac == userInfoHasHmacFalse {
		t.Fatal("HasHmac virou false com a revogacao FALHADA")
	}
}

// hmacKeyFromEnvelope extrai `data.hmac_key` do envelope do ADR-002.
func hmacKeyFromEnvelope(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	env := decodeEnvelope(t, rec)
	var view domain.HmacConfigView
	if err := json.Unmarshal(env.Data, &view); err != nil {
		t.Fatalf("data nao e' HmacConfigView: %v (corpo: %s)", err, rec.Body.String())
	}
	return view.HmacKey
}
