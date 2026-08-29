package bootstrap

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
	"github.com/justinas/alice"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/egress"
	"wa-api/pkg/infra/wa-noise/observability/applog"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/handlers"
)

// CAP-30 — POST /session/history · /webhook/history e POST /session/proxy ·
// /proxy/set.
//
// Estes testes exercitam a ROTA REGISTRADA (registerCustomRoutes +
// gorilla/mux), e as DUAS familias de caminho de cada rota, porque e' a
// ARMADILHA 2 deste repo.
//
// A cadeia de middleware e' a de PRODUCAO: `authAlice` (o mesmo
// middleware.AuthAlice que o router.go instala) sobre o `userinfocache` real.
// Isso importa mais do que parece — o gate de historico da F128 se semeia do
// que ESSE cache guarda, e um dublê de autenticacao que montasse o Values a
// mao teria provado a publicacao no cache errado sem que nada acusasse. O
// requisito e' o mesmo que o resto do repo aplica a dublê: ele nao pode ser
// mais permissivo, e aqui a solucao foi nao ter dublê nenhum.
//
// Nada de S3, HMAC ou cifra aparece aqui: as duas rotas deste bloco nao tocam
// segredo.

const (
	sessionCfgUserID = "user-session-cfg"
	sessionCfgToken  = "token-session-cfg"
)

type sessionCfgFixture struct {
	db     *sqlx.DB
	router *mux.Router

	// revalidacoes conta quantas vezes o gate de /chats/history foi ao banco
	// revalidar `users.history`. E' a metrica da F128: com o cache publicado
	// na escrita, a primeira leitura seguinte NAO revalida.
	revalidacoes *int

	// conectado decide o que SessionStatus reporta para o SetProxy.
	conectado *bool
}

// contadorDeRevalidacao embrulha o repositorio REAL de historico e so' conta
// as chamadas de HistoryLimit. Delegar (em vez de dublar) e' deliberado: o
// que esta' sob observacao e' QUANTAS vezes o gate revalida, nao o que a
// revalidacao devolve.
type contadorDeRevalidacao struct {
	appport.ChatHistoryReader
	chamadas *int
}

func (c contadorDeRevalidacao) HistoryLimit(ctx context.Context, userID string) (int, error) {
	*c.chamadas++
	return c.ChatHistoryReader.HistoryLimit(ctx, userID)
}

// statusDeSessao implementa appport.SessionStatusReader lendo um ponteiro que
// o teste controla. Imita a regra REAL do adapter de producao
// (pkg/infra/wa-noise/runtime/session/guard.go:72): o PRIMEIRO valor e'
// client.IsConnected(), e e' esse que a guarda historica consultava.
type statusDeSessao struct{ conectado *bool }

func (s statusDeSessao) SessionStatus(context.Context, string) (bool, bool) {
	return *s.conectado, *s.conectado
}

func newSessionCfgFixture(t *testing.T) *sessionCfgFixture {
	t.Helper()

	anteriorCtx := appCtx
	appCtx = NewAppContext()
	t.Cleanup(func() { appCtx = anteriorCtx })

	// O cache de autenticacao e' global do pacote, como em producao. Trocar e
	// restaurar mantem um teste invisivel para o outro.
	anteriorCache := userinfocache
	userinfocache = cache.New(5*time.Minute, 10*time.Minute)
	t.Cleanup(func() { userinfocache = anteriorCache })

	database := newChatHistoryDB(t)
	logger := applog.NewZerologAdapter(zerolog.Nop())

	revalidacoes := 0
	conectado := false

	store := db.NewSessionConfigRepository(database)
	historyRepo := contadorDeRevalidacao{
		ChatHistoryReader: db.NewChatHistoryRepository(database),
		chamadas:          &revalidacoes,
	}

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
		Community:   &handlers.CommunityHandlers{},
		Newsletter:  &handlers.NewsletterHandlers{},
		Label:       &handlers.LabelHandlers{},

		// Os dois handlers sob teste, com as MESMAS dependencias que
		// wiring_handlers.go monta em producao.
		Storage: &handlers.StorageHandlers{
			SetHistory: handlers.NewSetHistoryHandler(
				storage.NewSetHistoryUseCase(alwaysSessionGuard{}, store, userInfoSessionCache{}, logger)),
			// A LEITURA da mesma configuracao (CAP-32), com o MESMO
			// repositorio da escrita: e' o que permite ao teste do
			// round-trip provar que a rota le' a coluna que a outra grava.
			GetHistory: handlers.NewGetHistoryHandler(
				storage.NewGetHistoryUseCase(alwaysSessionGuard{}, store, logger)),
			SetProxy: handlers.NewSetProxyHandler(
				storage.NewSetProxyUseCase(statusDeSessao{&conectado}, store, userInfoSessionCache{}, true,
					egress.SystemResolver(), logger)),
		},
		// O consumidor do que a escrita publica: o gate da F128.
		ChatHistory: &handlers.ChatHistoryHandlers{
			GetChatHistory: handlers.NewGetChatHistoryHandler(
				chat.NewGetChatHistoryUseCase(historyRepo, logger)),
		},
	}

	f := &sessionCfgFixture{db: database, router: mux.NewRouter(), revalidacoes: &revalidacoes, conectado: &conectado}
	f.seedUser(t)
	registerCustomRoutes(f.router, alice.New(authAlice(database.DB, userinfocache, nil)), ch)
	return f
}

// seedUser grava a linha de users como POST /admin/users a grava: com o hash
// do token, que e' por onde AuthAlice casa (auth.go:171).
func (f *sessionCfgFixture) seedUser(t *testing.T) {
	t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(
		`INSERT INTO users (id, name, token, token_hash, history, proxy_url) VALUES (?, ?, ?, ?, ?, ?)`),
		sessionCfgUserID, "tenant session cfg", sessionCfgToken, domain.HashToken(sessionCfgToken), 0, ""); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

// seedCacheEntry poe o usuario no appCtx.UserInfoCache como
// ensureUserInfoCached o poria. Sem entrada previa o adapter e' no-op por
// desenho, e o teste da publicacao nao teria o que observar naquele cache.
func (f *sessionCfgFixture) seedCacheEntry() {
	appCtx.UserInfoCache.Set(sessionCfgUserID, Values{M: map[string]string{
		"Id":                 sessionCfgUserID,
		"Token":              sessionCfgToken,
		userInfoHistoryField: "0",
		userInfoProxyField:   "",
	}}, cache.NoExpiration)
}

func (f *sessionCfgFixture) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("token", sessionCfgToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// storedHistory e storedProxy leem as colunas direto — a fonte de verdade, sem
// passar pelo codigo sob teste.
func (f *sessionCfgFixture) storedHistory(t *testing.T) int {
	t.Helper()
	var history int
	if err := f.db.QueryRowx(f.db.Rebind(
		`SELECT COALESCE(history, 0) FROM users WHERE id = ?`), sessionCfgUserID).Scan(&history); err != nil {
		t.Fatalf("ler history: %v", err)
	}
	return history
}

func (f *sessionCfgFixture) storedProxy(t *testing.T) (string, bool) {
	t.Helper()
	var (
		url      string
		useProxy bool
	)
	if err := f.db.QueryRowx(f.db.Rebind(
		`SELECT COALESCE(proxy_url, ''), COALESCE(webhook_use_proxy, true) FROM users WHERE id = ?`),
		sessionCfgUserID).Scan(&url, &useProxy); err != nil {
		t.Fatalf("ler proxy: %v", err)
	}
	return url, useProxy
}

// cachedByUserID le o campo na entrada do appCtx.UserInfoCache (chaveada por
// usuario) — o cache que saveMessageHistory consulta.
func (f *sessionCfgFixture) cachedByUserID(t *testing.T, field string) string {
	t.Helper()
	cached, found := appCtx.UserInfoCache.Get(sessionCfgUserID)
	if !found {
		t.Fatal("a entrada do usuario sumiu do appCtx.UserInfoCache")
	}
	v, ok := cached.(Values)
	if !ok {
		t.Fatalf("a entrada do cache mudou de tipo: %T", cached)
	}
	return v.Get(field)
}

// cachedByToken le o campo na entrada do cache de AUTENTICACAO (chaveada por
// token) — o cache de onde o gate de /chats/history tira a semente.
func (f *sessionCfgFixture) cachedByToken(t *testing.T, field string) (string, time.Time) {
	t.Helper()
	cached, expiracao, found := userinfocache.GetWithExpiration(sessionCfgToken)
	if !found {
		t.Fatal("a entrada do token sumiu do cache de autenticacao")
	}
	v, ok := cached.(Values)
	if !ok {
		t.Fatalf("a entrada do cache mudou de tipo: %T", cached)
	}
	return v.Get(field), expiracao
}

// As DUAS familias de caminho de cada rota.
var (
	historyRoutePaths = []struct{ name, path string }{
		{"caminho /webhook", "/webhook/history"},
		{"caminho /session", "/session/history"},
	}
	proxyRoutePaths = []struct{ name, path string }{
		{"caminho /proxy", "/proxy/set"},
		{"caminho /session", "/session/proxy"},
	}
)

// TESTE 1 (rota) — POST grava no banco E publica nos DOIS caches de userinfo.
//
// Os dois, e nao um: appCtx.UserInfoCache (por usuario, NoExpiration) e' o que
// saveMessageHistory le' para decidir se uma mensagem recebida chega a ser
// persistida; o cache de autenticacao (por token, com TTL) e' o que semeia o
// gate de /chats/history. Publicar so' num deles deixaria metade do defeito de
// pe', e qual metade dependeria de qual cache a pessoa lembrou.
func TestSessionConfigRoute_SetHistoryGravaEPublicaNosDoisCaches(t *testing.T) {
	for _, rp := range historyRoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newSessionCfgFixture(t)
			f.seedCacheEntry()

			// Uma requisicao antes da escrita, so' para que AuthAlice crie a
			// entrada do token no cache de autenticacao — e' o estado real de
			// quem ja' fez ao menos uma chamada autenticada.
			f.do(t, http.MethodGet, "/chats/history?chat_jid=index", "")

			rec := f.do(t, http.MethodPost, rp.path, `{"history":50}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}

			if got := f.storedHistory(t); got != 50 {
				t.Fatalf("users.history = %d, quero 50 — a rota respondeu 200 e NAO gravou (o stub da F151 voltou)", got)
			}
			if got := f.cachedByUserID(t, userInfoHistoryField); got != "50" {
				t.Errorf("appCtx.UserInfoCache[History] = %q, quero \"50\": saveMessageHistory continua vendo o valor velho", got)
			}
			valor, expiracao := f.cachedByToken(t, userInfoHistoryField)
			if valor != "50" {
				t.Fatalf("cache de autenticacao[History] = %q, quero \"50\": o gate de /chats/history vai continuar "+
					"revalidando no banco uma vez por requisicao ate' o TTL expirar (F128)", valor)
			}
			if expiracao.IsZero() {
				t.Error("a entrada de autenticacao foi republicada SEM expiracao: o token passaria a autenticar " +
					"para sempre, sem o userCacheTTL que o limita")
			}
		})
	}
}

// TESTE 1b (rota) — a CONSEQUENCIA, que e' o que a F128 mede: depois da
// escrita, a primeira leitura de /chats/history NAO revalida no banco.
//
// Este e' o teste que morde se a publicacao no cache de autenticacao sumir.
// Nao assere o cache: assere o comportamento que a F128 descreve.
func TestSessionConfigRoute_SetHistoryFechaAF128_OGateNaoRevalida(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()

	// Antes: com history=0 no banco e no cache, o gate revalida e recusa.
	if rec := f.do(t, http.MethodGet, "/chats/history?chat_jid=index", ""); rec.Code == http.StatusOK {
		t.Fatalf("com history=0 a leitura devia ser recusada, veio %d (%s)", rec.Code, rec.Body.String())
	}
	if *f.revalidacoes != 1 {
		t.Fatalf("revalidacoes antes da escrita = %d, quero 1 — a linha de base do gate mudou", *f.revalidacoes)
	}

	if rec := f.do(t, http.MethodPost, "/session/history", `{"history":50}`); rec.Code != http.StatusOK {
		t.Fatalf("POST /session/history = %d (%s)", rec.Code, rec.Body.String())
	}

	antes := *f.revalidacoes
	rec := f.do(t, http.MethodGet, "/chats/history?chat_jid=index", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("depois de ligar o historico a leitura devia passar, veio %d (%s)", rec.Code, rec.Body.String())
	}
	if depois := *f.revalidacoes; depois != antes {
		t.Fatalf("o gate revalidou no banco %d vez(es) DEPOIS da escrita: a escrita nao publicou o valor "+
			"fresco no cache que o gate le', e a F128 continua aberta", depois-antes)
	}
}

// TESTE 2 (rota) — history negativo: 400, nada gravado, cache intocado.
func TestSessionConfigRoute_SetHistoryNegativo_400SemGravar(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()

	rec := f.do(t, http.MethodPost, "/session/history", `{"history":-1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if got := f.storedHistory(t); got != 0 {
		t.Errorf("a recusa gravou history = %d", got)
	}
	if got := f.cachedByUserID(t, userInfoHistoryField); got != "0" {
		t.Errorf("a recusa publicou %q no cache", got)
	}
}

// TESTE 3 (rota) — falha de gravacao: 500 e cache NAO tocado. O erro vem do
// driver real (banco fechado), nao de um erro fabricado.
func TestSessionConfigRoute_SetHistoryFalhaDeBanco_500SemTocarOCache(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()
	f.do(t, http.MethodGet, "/chats/history?chat_jid=index", "")

	if err := f.db.Close(); err != nil {
		t.Fatalf("fechar o banco: %v", err)
	}

	rec := f.do(t, http.MethodPost, "/session/history", `{"history":50}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if got := f.cachedByUserID(t, userInfoHistoryField); got != "0" {
		t.Errorf("publicou %q no cache por usuario depois de o banco falhar", got)
	}
	if valor, _ := f.cachedByToken(t, userInfoHistoryField); valor != "0" {
		t.Errorf("publicou %q no cache de autenticacao depois de o banco falhar", valor)
	}
}

// TESTE 4 (rota) — cliente CONECTADO: 400 e o banco NAO e' tocado. A guarda
// roda antes da escrita; move-la para depois devolveria 400 do mesmo jeito,
// com o proxy ja' gravado.
func TestSessionConfigRoute_SetProxyConectado_400SemGravar(t *testing.T) {
	for _, rp := range proxyRoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newSessionCfgFixture(t)
			f.seedCacheEntry()
			*f.conectado = true

			rec := f.do(t, http.MethodPost, rp.path, `{"enable":true,"proxy_url":"http://203.0.113.10:3128"}`)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if url, _ := f.storedProxy(t); url != "" {
				t.Fatalf("a guarda de sessao conectada roda DEPOIS da escrita: users.proxy_url = %q", url)
			}
			if got := f.cachedByUserID(t, userInfoProxyField); got != "" {
				t.Errorf("a recusa publicou %q no cache", got)
			}
		})
	}
}

// TESTES 5, 6 e 7 (rota) — os esquemas aceitos gravam e publicam; os demais
// sao 400 sem gravar.
func TestSessionConfigRoute_SetProxyEsquemas(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "http grava", url: "http://203.0.113.10:3128"},
		{name: "socks5 grava", url: "socks5://user:pass@203.0.113.10:1080"},
		{name: "https e' recusado", url: "https://203.0.113.10:3128", wantErr: true},
		{name: "ftp e' recusado", url: "ftp://203.0.113.10:21", wantErr: true},
	}
	for _, rp := range proxyRoutePaths {
		for _, tc := range cases {
			t.Run(rp.name+"/"+tc.name, func(t *testing.T) {
				f := newSessionCfgFixture(t)
				f.seedCacheEntry()
				f.do(t, http.MethodGet, "/chats/history?chat_jid=index", "")

				rec := f.do(t, http.MethodPost, rp.path, `{"enable":true,"proxy_url":"`+tc.url+`"}`)

				if tc.wantErr {
					if rec.Code != http.StatusBadRequest {
						t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
					}
					if corpo := rec.Body.String(); !strings.Contains(corpo, "only HTTP and SOCKS5 proxies are supported") {
						t.Errorf("corpo = %s, quero a mensagem historica de esquema nao suportado", corpo)
					}
					if url, _ := f.storedProxy(t); url != "" {
						t.Errorf("a recusa gravou proxy_url = %q", url)
					}
					return
				}

				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
				}
				if url, _ := f.storedProxy(t); url != tc.url {
					t.Fatalf("users.proxy_url = %q, quero %q — a rota respondeu 200 e NAO gravou", url, tc.url)
				}
				if got := f.cachedByUserID(t, userInfoProxyField); got != tc.url {
					t.Errorf("appCtx.UserInfoCache[Proxy] = %q, quero %q", got, tc.url)
				}
				if valor, _ := f.cachedByToken(t, userInfoProxyField); valor != tc.url {
					t.Errorf("cache de autenticacao[Proxy] = %q, quero %q", valor, tc.url)
				}

				// O corpo ecoa a configuracao GRAVADA, que e' o que distingue
				// "gravei o que voce pediu" de "respondi 200".
				var envelope struct {
					Data struct {
						ProxyURL string `json:"proxy_url"`
					} `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("decodificar envelope: %v (corpo: %s)", err, rec.Body.String())
				}
				if envelope.Data.ProxyURL != tc.url {
					t.Errorf("resposta ProxyURL = %q, quero %q", envelope.Data.ProxyURL, tc.url)
				}
			})
		}
	}
}

// TESTE 8 (rota) — habilitar sem proxy_url: 400, nada gravado.
func TestSessionConfigRoute_SetProxySemURL_400SemGravar(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()

	rec := f.do(t, http.MethodPost, "/session/proxy", `{"enable":true}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if corpo := rec.Body.String(); !strings.Contains(corpo, "missing proxy_url in payload") {
		t.Errorf("corpo = %s, quero a mensagem historica", corpo)
	}
	if url, _ := f.storedProxy(t); url != "" {
		t.Errorf("a recusa gravou proxy_url = %q", url)
	}
}

// TESTE 9 (rota) — URL malformada: 400, nada gravado.
func TestSessionConfigRoute_SetProxyURLMalformada_400SemGravar(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()

	rec := f.do(t, http.MethodPost, "/session/proxy", `{"enable":true,"proxy_url":"://x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if corpo := rec.Body.String(); !strings.Contains(corpo, "invalid proxy URL format") {
		t.Errorf("corpo = %s, quero a mensagem historica", corpo)
	}
	if url, _ := f.storedProxy(t); url != "" {
		t.Errorf("a recusa gravou proxy_url = %q", url)
	}
}

// TESTE 10 (rota) — desabilitar zera proxy_url no banco e o "Proxy" nos dois
// caches.
func TestSessionConfigRoute_SetProxyDesabilita_ZeraBancoECaches(t *testing.T) {
	for _, rp := range proxyRoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newSessionCfgFixture(t)
			f.seedCacheEntry()
			f.do(t, http.MethodGet, "/chats/history?chat_jid=index", "")

			const url = "socks5://203.0.113.10:1080"
			if rec := f.do(t, http.MethodPost, rp.path, `{"enable":true,"proxy_url":"`+url+`"}`); rec.Code != http.StatusOK {
				t.Fatalf("preparo: POST devolveu %d (%s)", rec.Code, rec.Body.String())
			}
			if got, _ := f.storedProxy(t); got != url {
				t.Fatalf("preparo: users.proxy_url = %q, quero %q", got, url)
			}

			rec := f.do(t, http.MethodPost, rp.path, `{"enable":false}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if got, _ := f.storedProxy(t); got != "" {
				t.Fatalf("users.proxy_url = %q depois de desabilitar, quero vazio", got)
			}
			if got := f.cachedByUserID(t, userInfoProxyField); got != "" {
				t.Errorf("appCtx.UserInfoCache[Proxy] = %q depois de desabilitar", got)
			}
			if valor, _ := f.cachedByToken(t, userInfoProxyField); valor != "" {
				t.Errorf("cache de autenticacao[Proxy] = %q depois de desabilitar", valor)
			}
		})
	}
}

// TESTE 11 (rota) — falha de gravacao do proxy: 500 e cache NAO tocado.
func TestSessionConfigRoute_SetProxyFalhaDeBanco_500SemTocarOCache(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()
	f.do(t, http.MethodGet, "/chats/history?chat_jid=index", "")

	if err := f.db.Close(); err != nil {
		t.Fatalf("fechar o banco: %v", err)
	}

	rec := f.do(t, http.MethodPost, "/session/proxy", `{"enable":true,"proxy_url":"http://203.0.113.10:3128"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if got := f.cachedByUserID(t, userInfoProxyField); got != "" {
		t.Errorf("publicou %q no cache por usuario depois de o banco falhar", got)
	}
	if valor, _ := f.cachedByToken(t, userInfoProxyField); valor != "" {
		t.Errorf("publicou %q no cache de autenticacao depois de o banco falhar", valor)
	}
}

// CAP-31 (rota) — a guarda de endereco reservado pela ROTA REGISTRADA, nas
// DUAS familias de caminho, e com o resolvedor de PRODUCAO
// (egress.SystemResolver(), montado no fixture): nao ha' duble de DNS aqui, e
// nao ha' E/S de rede porque os hosts sao IP LITERAL, que curto-circuitam a
// resolucao em egress.go:197.
//
// O modo vem do proprio corpo, e os dois valores sao obrigatorios: so' as
// recusas provariam que a guarda existe, nao que o gatilho dela e' o modo.
func TestSessionConfigRoute_SetProxyEnderecoReservado(t *testing.T) {
	const loopback = "http://127.0.0.1:3128"
	const reservado = "socks5://10.0.0.1:1080"

	cases := []struct {
		name    string
		corpo   string
		url     string
		wantErr bool
	}{
		{
			name:    "webhook pelo proxy + loopback e' 400",
			corpo:   `{"enable":true,"webhook_use_proxy":true,"proxy_url":"` + loopback + `"}`,
			wantErr: true,
		},
		{
			name:    "webhook pelo proxy + faixa reservada e' 400",
			corpo:   `{"enable":true,"webhook_use_proxy":true,"proxy_url":"` + reservado + `"}`,
			wantErr: true,
		},
		{
			name:  "webhook fora do proxy + loopback GRAVA",
			corpo: `{"enable":true,"webhook_use_proxy":false,"proxy_url":"` + loopback + `"}`,
			url:   loopback,
		},
		{
			name:  "webhook pelo proxy + IP publico GRAVA",
			corpo: `{"enable":true,"webhook_use_proxy":true,"proxy_url":"http://203.0.113.10:3128"}`,
			url:   "http://203.0.113.10:3128",
		},
	}
	for _, rp := range proxyRoutePaths {
		for _, tc := range cases {
			t.Run(rp.name+"/"+tc.name, func(t *testing.T) {
				f := newSessionCfgFixture(t)
				f.seedCacheEntry()
				f.do(t, http.MethodGet, "/chats/history?chat_jid=index", "")

				rec := f.do(t, http.MethodPost, rp.path, tc.corpo)

				if tc.wantErr {
					if rec.Code != http.StatusBadRequest {
						t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
					}
					if url, _ := f.storedProxy(t); url != "" {
						t.Fatalf("a recusa gravou users.proxy_url = %q", url)
					}
					if got := f.cachedByUserID(t, userInfoProxyField); got != "" {
						t.Errorf("a recusa publicou %q no cache por usuario", got)
					}
					if valor, _ := f.cachedByToken(t, userInfoProxyField); valor != "" {
						t.Errorf("a recusa publicou %q no cache de autenticacao", valor)
					}
					return
				}

				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
				}
				if url, _ := f.storedProxy(t); url != tc.url {
					t.Fatalf("users.proxy_url = %q, quero %q", url, tc.url)
				}
				if got := f.cachedByUserID(t, userInfoProxyField); got != tc.url {
					t.Errorf("appCtx.UserInfoCache[Proxy] = %q, quero %q", got, tc.url)
				}
			})
		}
	}
}
