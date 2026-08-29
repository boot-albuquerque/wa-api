package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/justinas/alice"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/auth"
	"wa-api/pkg/infra/db"
	"wa-api/pkg/infra/noise/observability/applog"
	intstorage "wa-api/pkg/infra/storage"
	customhttp "wa-api/pkg/presentation/http"
	dtostorage "wa-api/pkg/presentation/http/dto/storage"
	"wa-api/pkg/presentation/http/handlers"
)

// CAP-29 — POST /s3/configure, GET /s3/config, DELETE /s3/config e
// POST /s3/test.
//
// Estes testes exercitam a ROTA REGISTRADA (registerCustomRoutes +
// gorilla/mux), e nao o handler cru: a ARMADILHA 2 deste repo e' exatamente
// isso. As duas familias de caminho — `/s3/...` e `/session/s3/...` — sao
// exercitadas, porque a segunda passa por um wrapper que despacha por METODO
// (wiring_routes.go:176), e um `switch` errado ali mandaria o DELETE para o
// handler de leitura sem que nenhum teste de handler percebesse.
//
// Nada aqui e' dublê do lado que importa: SQLite real com o schema de
// producao, o AES-GCM real de pkg/infra/auth, o appCtx.UserInfoCache real, e o
// storage.S3Manager REAL — que e' de onde ProcessMediaForS3
// (wiring_delegates.go:70) tira o cliente que sobe midia. Um dublê de manager
// nao teria como provar a revogacao.

// s3TestEncryptionKey tem 32 bytes porque AES-256 exige exatamente isso —
// aes.NewCipher recusa qualquer outro tamanho (pkg/infra/auth/hmac.go:46).
const s3TestEncryptionKey = "0123456789abcdef0123456789abcdef"

// s3TestPlainSecret e' a credencial de terceiro que nao pode aparecer em
// lugar nenhum a nao ser dentro do envelope.
const s3TestPlainSecret = "sUp3r-s3cr3t-s3-key-value"

// s3TestAccessKey e' a access key, que a leitura mascara.
const s3TestAccessKey = "AKIAEXAMPLEACCESSKEY"

// s3TestEndpoint e' um IP LITERAL da faixa de documentacao TEST-NET-3
// (RFC 5737), e as duas propriedades sao deliberadas:
//
//   - literal, porque egress.ValidateOutboundURL resolve DNS para nomes
//     (pkg/infra/egress/egress.go:142) e um IP curto-circuita em :132; um nome
//     sintetico faria o teste depender de resolucao de rede;
//   - 203.0.113.0/24, porque nao esta' em reservedCIDRs (egress.go:50) e
//     portanto passa a validacao, ao contrario de loopback ou RFC1918.
//
// O POST nao toca a rede: InitializeS3Client so' monta o cliente do SDK
// (pkg/infra/storage/s3.go:110). Quem toca e' o teste de CONEXAO, e esse
// aponta para o httptest.Server.
const s3TestEndpoint = "https://203.0.113.10"

type s3RouteFixture struct {
	db     *sqlx.DB
	router *mux.Router
	userID string
	logs   *strings.Builder
}

// newS3RouteFixture monta banco, cache, roteador e captura de log reais.
//
// appCtx e' global do pacote: o fixture o substitui por um novo e restaura no
// Cleanup, para que um teste nao veja o cache do outro. O userID e' unico por
// teste porque o storage.S3Manager e' um singleton de processo — dois testes
// com o mesmo id disputariam a mesma entrada do registro.
func newS3RouteFixture(t *testing.T) *s3RouteFixture {
	t.Helper()

	anterior := appCtx
	appCtx = NewAppContext()
	appCtx.GlobalEncryptionKey = s3TestEncryptionKey
	t.Cleanup(func() { appCtx = anterior })

	// O log global do zerolog e' para onde os adapters e o repositorio
	// escrevem; o adapter de applog leva o log dos use cases para o MESMO
	// buffer. Sem os dois, a checagem de vazamento cobriria metade do caminho.
	logs := &strings.Builder{}
	captured := zerolog.New(logs)
	logAnterior := zlog.Logger
	zlog.Logger = captured
	t.Cleanup(func() { zlog.Logger = logAnterior })

	database := newChatHistoryDB(t)
	logger := applog.NewZerologAdapter(captured)

	userID := "user-s3-" + strings.ReplaceAll(t.Name(), "/", "-")

	store := db.NewS3ConfigRepository(database)
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
		ChatHistory: &handlers.ChatHistoryHandlers{},
	}
	ch.Storage = &handlers.StorageHandlers{
		// Os quatro handlers sob teste, com as MESMAS dependencias que
		// wiring_handlers.go monta em producao.
		ConfigureS3: handlers.NewConfigureS3Handler(
			storage.NewConfigureS3UseCase(alwaysSessionGuard{}, store, s3SecretCipher{}, s3ClientManager{}, userInfoS3Cache{}, logger)),
		GetS3Config: handlers.NewGetS3ConfigHandler(
			storage.NewGetS3ConfigUseCase(alwaysSessionGuard{}, store, logger)),
		DeleteS3Config: handlers.NewDeleteS3ConfigHandler(
			storage.NewDeleteS3ConfigUseCase(alwaysSessionGuard{}, store, s3ClientManager{}, userInfoS3Cache{}, logger)),
		TestS3Connection: handlers.NewTestS3ConnectionHandler(
			storage.NewTestS3ConnectionUseCase(alwaysSessionGuard{}, store, s3SecretCipher{}, s3ClientManager{}, logger)),
	}

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v := Values{M: map[string]string{"Id": userID}}
			next.ServeHTTP(w, r.WithContext(
				context.WithValue(r.Context(), appport.UserInfoKey, v)))
		})
	}

	router := mux.NewRouter()
	registerCustomRoutes(router, alice.New(inject), ch)
	f := &s3RouteFixture{db: database, router: router, userID: userID, logs: logs}
	f.seedUser(t)
	// O singleton de processo nao pode levar estado de um teste para o outro.
	t.Cleanup(func() { intstorage.GetS3Manager().RemoveClient(userID) })
	return f
}

// seedUser grava a linha de users sem S3 configurado — o estado inicial de
// quem nunca configurou.
func (f *s3RouteFixture) seedUser(t *testing.T) {
	t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(
		`INSERT INTO users (id, name, token, token_hash) VALUES (?, ?, ?, ?)`),
		f.userID, "tenant s3", "token-"+f.userID, "hash-"+f.userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

// seedCacheEntry poe o usuario no appCtx.UserInfoCache como
// ensureUserInfoCached o poria. Sem entrada previa o adapter e' no-op por
// desenho, e o teste do cache nao teria o que observar.
func (f *s3RouteFixture) seedCacheEntry() {
	appCtx.UserInfoCache.Set(f.userID, Values{M: map[string]string{
		"Id":                       f.userID,
		"Webhook":                  "https://example.invalid/hook",
		userInfoS3EnabledField:     userInfoS3EnabledFalse,
		userInfoMediaDeliveryField: domain.MediaDeliveryBase64,
	}}, cache.NoExpiration)
}

// cachedS3 devolve os dois campos que a configuracao publica.
func (f *s3RouteFixture) cachedS3(t *testing.T) (enabled, mediaDelivery string) {
	t.Helper()
	cached, found := appCtx.UserInfoCache.Get(f.userID)
	if !found {
		t.Fatal("a entrada do usuario sumiu do UserInfoCache")
	}
	v, ok := cached.(Values)
	if !ok {
		t.Fatalf("a entrada do cache mudou de tipo: %T", cached)
	}
	return v.Get(userInfoS3EnabledField), v.Get(userInfoMediaDeliveryField)
}

// cachedEntryText serializa a entrada inteira do cache, para a busca negativa
// por segredo — o campo certo nao basta se o valor vaza noutro.
func (f *s3RouteFixture) cachedEntryText(t *testing.T) string {
	t.Helper()
	cached, found := appCtx.UserInfoCache.Get(f.userID)
	if !found {
		return ""
	}
	v, ok := cached.(Values)
	if !ok {
		t.Fatalf("a entrada do cache mudou de tipo: %T", cached)
	}
	return fmt.Sprintf("%v", v.M)
}

// storedS3 le as colunas de S3 diretamente — a fonte de verdade, sem passar
// pelo codigo sob teste.
func (f *s3RouteFixture) storedS3(t *testing.T) (enabled bool, endpoint, region, bucket, accessKey, secretKey, publicURL, mediaDelivery string, pathStyle bool, retentionDays int) {
	t.Helper()
	row := f.db.QueryRowx(f.db.Rebind(
		`SELECT s3_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key,
			s3_secret_key, s3_public_url, media_delivery, s3_path_style, s3_retention_days
		 FROM users WHERE id = ?`), f.userID)
	if err := row.Scan(&enabled, &endpoint, &region, &bucket, &accessKey,
		&secretKey, &publicURL, &mediaDelivery, &pathStyle, &retentionDays); err != nil {
		t.Fatalf("ler as colunas de s3: %v", err)
	}
	return
}

// storedSecret le apenas users.s3_secret_key.
func (f *s3RouteFixture) storedSecret(t *testing.T) string {
	t.Helper()
	var secret string
	if err := f.db.QueryRowx(f.db.Rebind(`SELECT s3_secret_key FROM users WHERE id = ?`), f.userID).
		Scan(&secret); err != nil {
		t.Fatalf("ler s3_secret_key: %v", err)
	}
	return secret
}

// writeRawSecret grava um valor CRU em s3_secret_key, sem passar pelo
// cifrador: e' assim que uma instalacao escrita pelo binario historico chega
// ao codigo novo.
func (f *s3RouteFixture) writeRawSecret(t *testing.T, raw string) {
	t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(`UPDATE users SET s3_secret_key = ? WHERE id = ?`),
		raw, f.userID); err != nil {
		t.Fatalf("gravar segredo cru: %v", err)
	}
}

// enableS3Row habilita o S3 do usuario apontando para endpoint, com o segredo
// no envelope REAL — nao o do dublê.
func (f *s3RouteFixture) enableS3Row(t *testing.T, endpoint string) {
	t.Helper()
	envelope, err := auth.EncryptS3Secret(s3TestPlainSecret, s3TestEncryptionKey)
	if err != nil {
		t.Fatalf("cifrar o segredo do preparo: %v", err)
	}
	if _, err := f.db.Exec(f.db.Rebind(
		`UPDATE users SET s3_enabled = ?, s3_endpoint = ?, s3_region = ?, s3_bucket = ?,
			s3_access_key = ?, s3_secret_key = ?, s3_path_style = ?, media_delivery = ?,
			s3_retention_days = ? WHERE id = ?`),
		true, endpoint, "us-east-1", "mybucket", s3TestAccessKey, envelope, true,
		domain.MediaDeliveryBase64, 30, f.userID); err != nil {
		t.Fatalf("habilitar s3: %v", err)
	}
}

func (f *s3RouteFixture) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

// configureBody monta o corpo do POST com os dez campos do contrato.
func configureBody(enabled bool, mediaDelivery string) string {
	return fmt.Sprintf(`{"enabled":%t,"endpoint":%q,"region":"us-east-1","bucket":"mybucket",`+
		`"access_key":%q,"secret_key":%q,"path_style":true,"public_url":"https://cdn.example.com",`+
		`"media_delivery":%q,"retention_days":7}`,
		enabled, s3TestEndpoint, s3TestAccessKey, s3TestPlainSecret, mediaDelivery)
}

// s3RoutePaths sao as DUAS familias de caminho que chegam aos mesmos
// handlers, com a tripla (configure, config, test) de cada uma.
var s3RoutePaths = []struct {
	name      string
	configure string
	config    string
	test      string
}{
	{"caminho direto", "/s3/configure", "/s3/config", "/s3/test"},
	{"caminho /session", "/session/s3/config", "/session/s3/config", "/session/s3/test"},
}

// TESTE 1 (rota) — POST grava o segredo NO ENVELOPE, e ele decifra de volta
// para o texto original.
//
// A ida-e-volta e' o que morde: "e' diferente do texto plano" passaria com um
// base64 puro, com um hash, ou com qualquer coisa que nao devolva a credencial
// para quem precisa dela (ADR-0009).
func TestS3Route_PostGravaEnvelopeQueDecifraDeVolta(t *testing.T) {
	for _, rp := range s3RoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newS3RouteFixture(t)
			f.seedCacheEntry()

			rec := f.do(t, http.MethodPost, rp.configure, configureBody(true, domain.MediaDeliveryBoth))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}

			enabled, endpoint, region, bucket, accessKey, secret, publicURL,
				mediaDelivery, pathStyle, retentionDays := f.storedS3(t)

			// As DEZ colunas do UPDATE historico, uma a uma: gravar nove e
			// esquecer uma devolveria 200 igual.
			if !enabled {
				t.Error("s3_enabled = false depois de configurar com enabled:true")
			}
			if endpoint != s3TestEndpoint {
				t.Errorf("s3_endpoint = %q, quero %q", endpoint, s3TestEndpoint)
			}
			if region != "us-east-1" {
				t.Errorf("s3_region = %q, quero us-east-1", region)
			}
			if bucket != "mybucket" {
				t.Errorf("s3_bucket = %q, quero mybucket", bucket)
			}
			if accessKey != s3TestAccessKey {
				t.Errorf("s3_access_key = %q, quero %q", accessKey, s3TestAccessKey)
			}
			if publicURL != "https://cdn.example.com" {
				t.Errorf("s3_public_url = %q", publicURL)
			}
			if mediaDelivery != domain.MediaDeliveryBoth {
				t.Errorf("media_delivery = %q, quero %q", mediaDelivery, domain.MediaDeliveryBoth)
			}
			if !pathStyle {
				t.Error("s3_path_style = false, quero true")
			}
			if retentionDays != 7 {
				t.Errorf("s3_retention_days = %d, quero 7", retentionDays)
			}

			if secret == "" {
				t.Fatal("a rota respondeu 200 e NAO gravou o segredo — o stub da F151 voltou")
			}
			if secret == s3TestPlainSecret {
				t.Fatal("users.s3_secret_key guarda a credencial EM CLARO (ADR-0009)")
			}
			if !strings.HasPrefix(secret, auth.S3SecretEnvelopePrefix) {
				t.Fatalf("o valor gravado (%q) nao tem o envelope %q", secret, auth.S3SecretEnvelopePrefix)
			}
			decifrado, err := auth.DecryptS3Secret(secret, []byte(s3TestEncryptionKey))
			if err != nil {
				t.Fatalf("o valor gravado nao e' o envelope AES-GCM da chave global: %v", err)
			}
			if decifrado != s3TestPlainSecret {
				t.Fatalf("decifrado = %q, quero %q", decifrado, s3TestPlainSecret)
			}

			// O cliente do registro REAL nasceu, porque enabled=true.
			if _, _, ok := intstorage.GetS3Manager().GetClient(f.userID); !ok {
				t.Error("o cliente nao foi registrado no S3Manager depois de habilitar")
			}

			// E o cache recebeu os dois campos que os leitores consomem —
			// e NENHUMA credencial.
			cachedEnabled, cachedDelivery := f.cachedS3(t)
			if cachedEnabled != userInfoS3EnabledTrue {
				t.Errorf("S3Enabled = %q no cache, quero %q", cachedEnabled, userInfoS3EnabledTrue)
			}
			if cachedDelivery != domain.MediaDeliveryBoth {
				t.Errorf("MediaDelivery = %q no cache, quero %q", cachedDelivery, domain.MediaDeliveryBoth)
			}
			assertNoSecretIn(t, "a entrada do UserInfoCache", f.cachedEntryText(t), secret)
		})
	}
}

// TESTE 2 (rota) — media_delivery invalido: 400 e NADA gravado.
func TestS3Route_PostMediaDeliveryInvalido_400SemGravar(t *testing.T) {
	f := newS3RouteFixture(t)
	f.seedCacheEntry()

	rec := f.do(t, http.MethodPost, "/s3/configure", configureBody(true, "carrier-pigeon"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if got := f.storedSecret(t); got != "" {
		t.Fatalf("a recusa gravou %q em s3_secret_key", got)
	}
	if enabled, _, _, _, _, _, _, _, _, _ := f.storedS3(t); enabled {
		t.Fatal("a recusa habilitou o S3")
	}
	if cachedEnabled, _ := f.cachedS3(t); cachedEnabled != userInfoS3EnabledFalse {
		t.Fatalf("a recusa publicou S3Enabled = %q no cache", cachedEnabled)
	}
}

// TESTE 3 (rota) — media_delivery vazio vira o default "base64", e nao fica
// vazio na coluna: o leitor de midia compara com strings exatas
// (eventhandler_message.go:47), e "" nunca casaria com nenhuma.
func TestS3Route_PostMediaDeliveryVazio_ViraBase64(t *testing.T) {
	f := newS3RouteFixture(t)
	f.seedCacheEntry()

	rec := f.do(t, http.MethodPost, "/s3/configure", configureBody(true, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if _, _, _, _, _, _, _, mediaDelivery, _, _ := f.storedS3(t); mediaDelivery != domain.MediaDeliveryBase64 {
		t.Fatalf("media_delivery = %q, quero %q", mediaDelivery, domain.MediaDeliveryBase64)
	}
	if _, cachedDelivery := f.cachedS3(t); cachedDelivery != domain.MediaDeliveryBase64 {
		t.Fatalf("MediaDelivery = %q no cache, quero %q", cachedDelivery, domain.MediaDeliveryBase64)
	}
}

// TESTE 4 (rota) — sem chave de encriptacao global a cifra falha: 500, e NADA
// gravado. E' o caminho que poria a credencial em claro na coluna se a ordem
// invertesse (cifrar DEPOIS de gravar).
func TestS3Route_PostSemChaveDeEncriptacao_500SemGravar(t *testing.T) {
	f := newS3RouteFixture(t)
	f.seedCacheEntry()
	appCtx.GlobalEncryptionKey = ""

	rec := f.do(t, http.MethodPost, "/s3/configure", configureBody(true, domain.MediaDeliveryS3))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if got := f.storedSecret(t); got != "" {
		t.Fatalf("gravou %q depois de a cifra falhar", got)
	}
	if enabled, _, _, _, _, _, _, _, _, _ := f.storedS3(t); enabled {
		t.Fatal("habilitou o S3 depois de a cifra falhar")
	}
	if cachedEnabled, _ := f.cachedS3(t); cachedEnabled != userInfoS3EnabledFalse {
		t.Fatalf("publicou S3Enabled = %q no cache depois de a cifra falhar", cachedEnabled)
	}
	if _, _, ok := intstorage.GetS3Manager().GetClient(f.userID); ok {
		t.Fatal("registrou o cliente depois de a cifra falhar")
	}
	assertNoSecretIn(t, "o log", f.logs.String(), "")
}

// TESTE 5 (rota) — a leitura mascara a access key e NAO devolve o segredo em
// forma nenhuma. A busca e' no corpo INTEIRO, e nao so' no campo: um campo
// novo na resposta e' exatamente como um segredo volta a vazar.
func TestS3Route_GetMascaraEnaoVazaOSegredo(t *testing.T) {
	for _, rp := range s3RoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newS3RouteFixture(t)
			f.seedCacheEntry()

			if rec := f.do(t, http.MethodPost, rp.configure, configureBody(true, domain.MediaDeliveryBoth)); rec.Code != http.StatusOK {
				t.Fatalf("preparo: POST devolveu %d (%s)", rec.Code, rec.Body.String())
			}
			envelope := f.storedSecret(t)

			rec := f.do(t, http.MethodGet, rp.config, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}

			view := s3ViewFromEnvelope(t, rec)
			if view.AccessKey != domain.MaskedS3AccessKey {
				t.Fatalf("access_key = %q, quero %q", view.AccessKey, domain.MaskedS3AccessKey)
			}
			// O resto da configuracao TEM de vir, ou a mascara estaria
			// escondendo o fato de a leitura nao ler nada.
			if !view.Enabled || view.Bucket != "mybucket" || view.Region != "us-east-1" {
				t.Fatalf("a leitura nao devolveu a configuracao gravada: %+v", view)
			}
			if view.MediaDelivery != domain.MediaDeliveryBoth || view.RetentionDays != 7 {
				t.Fatalf("a leitura nao devolveu media_delivery/retention: %+v", view)
			}

			assertNoSecretIn(t, "a resposta do GET", rec.Body.String(), envelope)
			if strings.Contains(rec.Body.String(), s3TestAccessKey) {
				t.Fatalf("a access key REAL vazou na resposta: %s", rec.Body.String())
			}
		})
	}
}

// TESTE 6 (rota) — a REVOGACAO. Banco limpo **e** cliente fora do S3Manager.
//
// A assercao do manager e' a que morde: o cliente vive no registro de processo
// e serve todo upload de midia (wiring_delegates.go:70). Uma revogacao que
// limpasse so' o banco devolveria exatamente o mesmo 200 com a credencial
// continuando a subir arquivo ate' o processo reiniciar (HOUSEKEEP F157).
func TestS3Route_DeleteRevogaBancoEClienteEmMemoria(t *testing.T) {
	for _, rp := range s3RoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newS3RouteFixture(t)
			f.seedCacheEntry()

			if rec := f.do(t, http.MethodPost, rp.configure, configureBody(true, domain.MediaDeliveryS3)); rec.Code != http.StatusOK {
				t.Fatalf("preparo: POST devolveu %d (%s)", rec.Code, rec.Body.String())
			}
			if _, _, ok := intstorage.GetS3Manager().GetClient(f.userID); !ok {
				t.Fatal("preparo: o cliente nao nasceu, entao a revogacao nao teria o que remover")
			}

			rec := f.do(t, http.MethodDelete, rp.config, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}

			enabled, endpoint, region, bucket, accessKey, secret, publicURL,
				mediaDelivery, pathStyle, retentionDays := f.storedS3(t)
			if enabled || endpoint != "" || region != "" || bucket != "" ||
				accessKey != "" || secret != "" || publicURL != "" {
				t.Fatalf("as colunas de S3 continuam preenchidas apos a revogacao: "+
					"enabled=%t endpoint=%q region=%q bucket=%q access=%q secret=%q public=%q",
					enabled, endpoint, region, bucket, accessKey, secret, publicURL)
			}
			// Os tres defaults do UPDATE historico (`41bc8e2^:handlers.go:6465`).
			if mediaDelivery != domain.MediaDeliveryBase64 || !pathStyle || retentionDays != 30 {
				t.Errorf("os defaults do estado limpo nao foram restaurados: media_delivery=%q path_style=%t retention=%d",
					mediaDelivery, pathStyle, retentionDays)
			}

			if _, _, ok := intstorage.GetS3Manager().GetClient(f.userID); ok {
				t.Fatal("o cliente CONTINUA no S3Manager apos a revogacao: a credencial revogada segue subindo midia do usuario")
			}
			if cachedEnabled, cachedDelivery := f.cachedS3(t); cachedEnabled != userInfoS3EnabledFalse ||
				cachedDelivery != domain.MediaDeliveryBase64 {
				t.Fatalf("o cache nao refletiu a revogacao: S3Enabled=%q MediaDelivery=%q", cachedEnabled, cachedDelivery)
			}
		})
	}
}

// TESTE 7 (rota) — falha de banco na revogacao: 500 e o cliente NAO e'
// removido.
//
// A ordem e' o contrato. Remover o cliente antes da gravacao deixaria a
// revogacao pela metade: o proximo EnsureClientFromDB o reconstruiria da linha
// que nunca mudou, e o operador teria recebido 500 com a credencial ainda
// ativa E o cliente derrubado — o pior dos dois estados.
//
// A falha e' produzida fechando o banco: o UPDATE volta com erro real do
// driver, e nao com um erro fabricado que so' existe no teste.
func TestS3Route_DeleteComBancoQuebrado_500SemRemoverOCliente(t *testing.T) {
	f := newS3RouteFixture(t)
	f.seedCacheEntry()

	if rec := f.do(t, http.MethodPost, "/s3/configure", configureBody(true, domain.MediaDeliveryS3)); rec.Code != http.StatusOK {
		t.Fatalf("preparo: POST devolveu %d (%s)", rec.Code, rec.Body.String())
	}
	if _, _, ok := intstorage.GetS3Manager().GetClient(f.userID); !ok {
		t.Fatal("preparo: o cliente nao nasceu")
	}

	if err := f.db.Close(); err != nil {
		t.Fatalf("fechar o banco: %v", err)
	}

	rec := f.do(t, http.MethodDelete, "/s3/config", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if _, _, ok := intstorage.GetS3Manager().GetClient(f.userID); !ok {
		t.Fatal("o cliente foi removido com a revogacao FALHADA: o banco continua com a credencial e o upload parou sem ninguem ter pedido")
	}
}

// TESTE 8 (rota) — teste de conexao com S3 desabilitado: 400 com a mensagem
// historica, e a rede nao e' tocada.
func TestS3Route_TestComS3Desabilitado_400(t *testing.T) {
	for _, rp := range s3RoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newS3RouteFixture(t)

			rec := f.do(t, http.MethodPost, rp.test, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, quero 400 (corpo: %s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "S3 is not enabled for this user") {
				t.Fatalf("a mensagem historica sumiu do corpo: %s", rec.Body.String())
			}
			if _, _, ok := intstorage.GetS3Manager().GetClient(f.userID); ok {
				t.Error("a recusa registrou um cliente")
			}
		})
	}
}

// TESTE 9 (rota) — teste de conexao com o fake respondendo OK: 200 com Bucket
// e Region, e o ListObjectsV2 REAL chegou ao servidor.
func TestS3Route_TestComFakeOK_200ComBucketERegion(t *testing.T) {
	for _, rp := range s3RoutePaths {
		t.Run(rp.name, func(t *testing.T) {
			f := newS3RouteFixture(t)
			fake := newFakeS3Endpoint(t, false)
			f.enableS3Row(t, fake.URL)

			rec := f.do(t, http.MethodPost, rp.test, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			var result domain.S3TestResult
			decodeS3Data(t, rec, &result)
			if !result.Connected {
				t.Error("connected = false num teste bem-sucedido")
			}
			if result.Bucket != "mybucket" || result.Region != "us-east-1" {
				t.Fatalf("Bucket/Region = %q/%q, quero mybucket/us-east-1", result.Bucket, result.Region)
			}
			if fake.calls() == 0 {
				t.Fatal("o 200 saiu sem NENHUMA requisicao ao endpoint: o teste de conexao nao testou conexao")
			}
			assertNoSecretIn(t, "a resposta do teste de conexao", rec.Body.String(), f.storedSecret(t))
		})
	}
}

// TESTE 10 (rota) — o fake responde erro: 422 com o envelope canônico
// {code,error:{code,message}}, e nada de 200 nem de fallback silencioso.
//
// F276: até esta correção, a recusa do upstream subia CRUA e a rota
// respondia 500 com `error` como texto solto — o único jeito de saber SE a
// configuração estava boa era ler o log do servidor, não a resposta HTTP.
// Este teste ficava VERDE nesse estado (era, literalmente, o comportamento
// que ele afirmava); a mudança correta é a de baixo, e não reverter esta
// asserção.
func TestS3Route_TestComFakeErro_422ComEnvelopeCanonico(t *testing.T) {
	f := newS3RouteFixture(t)
	fake := newFakeS3Endpoint(t, true)
	f.enableS3Row(t, fake.URL)

	rec := f.do(t, http.MethodPost, "/s3/test", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quero 422 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "successful") {
		t.Fatalf("a falha de conexao foi reportada como sucesso: %s", rec.Body.String())
	}
	if fake.calls() == 0 {
		t.Fatal("o 422 saiu sem tocar o endpoint: a falha veio de outro lugar que nao a conexao")
	}

	var env struct {
		Success bool `json:"success"`
		Code    int  `json:"code"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("corpo nao e' JSON: %v (%s)", err, rec.Body.String())
	}
	if env.Success {
		t.Fatal("success=true numa resposta de erro")
	}
	if env.Code != http.StatusUnprocessableEntity {
		t.Fatalf("envelope.code = %d, quero 422", env.Code)
	}
	if env.Error.Code != "upstream_rejected" {
		t.Fatalf("error.code = %q, quero upstream_rejected", env.Error.Code)
	}
	if env.Error.Message == "" {
		t.Fatal("error.message vazio: a resposta deixou de dizer o diagnostico do upstream")
	}
	assertNoSecretIn(t, "a resposta do teste de conexao com falha", rec.Body.String(), f.storedSecret(t))
}

// TESTE 11 (rota) — LEGADO: linha com s3_secret_key SEM o envelope e' INVALIDA
// e NAO e' usada como texto claro.
//
// Este e' o teste do ADR-0009, e existe porque o proximo executor, ao ver
// `Decrypt` falhar numa instalacao antiga, "conserta" caindo para plaintext —
// que e' fallback silencioso em caminho de segredo. Se ele fizer isso, este
// teste vira vermelho: o fake responderia 200 e a rota devolveria sucesso.
//
// O fake responde OK de proposito: assim o UNICO motivo possivel para o 500 e'
// a recusa do valor legado, e nao uma falha de rede que mascararia o defeito.
func TestS3Route_TestComSegredoLegadoSemEnvelope_RecusaSemCairParaPlaintext(t *testing.T) {
	f := newS3RouteFixture(t)
	fake := newFakeS3Endpoint(t, false)
	f.enableS3Row(t, fake.URL)
	// Sobrescreve o envelope pelo texto claro, que e' o que o binario
	// historico gravava (`41bc8e2^:handlers.go:6243`).
	f.writeRawSecret(t, s3TestPlainSecret)

	rec := f.do(t, http.MethodPost, "/s3/test", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500: a linha legada foi ACEITA (corpo: %s)", rec.Code, rec.Body.String())
	}
	if fake.calls() != 0 {
		t.Fatal("a linha legada chegou a montar um cliente e a falar com o endpoint: o segredo em claro FOI usado como credencial")
	}
	assertNoSecretIn(t, "a resposta da recusa do legado", rec.Body.String(), "")
	assertNoSecretIn(t, "o log da recusa do legado", f.logs.String(), "")
}

// TESTE 12 (rota) — o segredo nao aparece em log em NENHUM dos quatro
// caminhos, nem no caminho de erro.
//
// A varredura e' sobre o buffer inteiro do zerolog global (onde escrevem o
// repositorio e os adapters) e do adapter de applog (onde escrevem os use
// cases): cobrir so' um dos dois deixaria metade do caminho sem medida.
func TestS3Route_SegredoNuncaVaiParaOLog(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, f *s3RouteFixture)
	}{
		{"POST", func(t *testing.T, f *s3RouteFixture) {
			f.do(t, http.MethodPost, "/s3/configure", configureBody(true, domain.MediaDeliveryBoth))
		}},
		{"POST com falha de cifra", func(t *testing.T, f *s3RouteFixture) {
			appCtx.GlobalEncryptionKey = ""
			f.do(t, http.MethodPost, "/s3/configure", configureBody(true, domain.MediaDeliveryBoth))
		}},
		{"GET", func(t *testing.T, f *s3RouteFixture) {
			f.do(t, http.MethodPost, "/s3/configure", configureBody(true, domain.MediaDeliveryBoth))
			f.do(t, http.MethodGet, "/s3/config", "")
		}},
		{"DELETE", func(t *testing.T, f *s3RouteFixture) {
			f.do(t, http.MethodPost, "/s3/configure", configureBody(true, domain.MediaDeliveryBoth))
			f.do(t, http.MethodDelete, "/s3/config", "")
		}},
		{"TEST com sucesso", func(t *testing.T, f *s3RouteFixture) {
			fake := newFakeS3Endpoint(t, false)
			f.enableS3Row(t, fake.URL)
			f.do(t, http.MethodPost, "/s3/test", "")
		}},
		{"TEST com falha de conexao", func(t *testing.T, f *s3RouteFixture) {
			fake := newFakeS3Endpoint(t, true)
			f.enableS3Row(t, fake.URL)
			f.do(t, http.MethodPost, "/s3/test", "")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newS3RouteFixture(t)
			f.seedCacheEntry()
			tc.run(t, f)

			if f.logs.Len() == 0 {
				t.Fatal("o caminho nao logou nada — a checagem de vazamento ficaria vacua")
			}
			assertNoSecretIn(t, "o log", f.logs.String(), f.storedSecretOrEmpty())
		})
	}
}

// storedSecretOrEmpty le s3_secret_key sem falhar quando a linha ou o banco
// nao estao mais la'.
func (f *s3RouteFixture) storedSecretOrEmpty() string {
	var secret string
	if err := f.db.QueryRowx(f.db.Rebind(`SELECT s3_secret_key FROM users WHERE id = ?`), f.userID).
		Scan(&secret); err != nil {
		return ""
	}
	return secret
}

// assertNoSecretIn procura o segredo em claro — e, quando informado, o
// envelope inteiro — dentro de texto. `where` nomeia o lugar para que a falha
// diga o que vazou onde.
func assertNoSecretIn(t *testing.T, where, texto, envelope string) {
	t.Helper()
	formas := []struct{ nome, valor string }{
		{"o segredo em texto plano", s3TestPlainSecret},
	}
	if envelope != "" {
		formas = append(formas, struct{ nome, valor string }{"o envelope cifrado", envelope})
	}
	for _, forma := range formas {
		if strings.Contains(texto, forma.valor) {
			t.Fatalf("%s vazou em %s: %s", forma.nome, where, texto)
		}
	}
}

// s3ViewFromEnvelope extrai `data` do envelope do ADR-002 como S3ConfigView.
func s3ViewFromEnvelope(t *testing.T, rec *httptest.ResponseRecorder) dtostorage.S3ConfigViewResponse {
	t.Helper()
	var view dtostorage.S3ConfigViewResponse
	decodeS3Data(t, rec, &view)
	return view
}

func decodeS3Data(t *testing.T, rec *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	env := decodeEnvelope(t, rec)
	if err := json.Unmarshal(env.Data, dest); err != nil {
		t.Fatalf("data nao decodifica em %T: %v (corpo: %s)", dest, err, rec.Body.String())
	}
}

// --- o dublê do endpoint de S3 -------------------------------------------

// fakeS3Endpoint e' um servidor S3-compativel em processo.
//
// O SDK v2 da AWS e' um struct concreto sem costura de injecao, mas aceita um
// BaseEndpoint: apontar para um httptest.Server exercita a construcao, a
// ASSINATURA e o parsing de XML REAIS, sem rede nem Docker. E' a mesma tecnica
// (e a mesma razao) do harness que ja' existe em
// pkg/infra/storage/s3_manager_test.go:19-24 e do dublê renderListXML de
// :188 — replicada aqui, e nao importada, porque aquele e' interno ao pacote
// pkg/infra/storage.
//
// TestConnection chama ListObjectsV2 (pkg/infra/storage/s3.go:381), que e' um
// GET; e' a unica operacao que este dublê precisa saber responder.
type fakeS3Endpoint struct {
	*httptest.Server
	mu    chan struct{}
	count *int
}

// newFakeS3Endpoint sobe o servidor. Com failList, ele responde 403 —
// escolhido em vez de 5xx porque o retryer padrao do SDK trata 5xx como
// retentavel, e cada assercao de erro viraria tres viagens mais backoff
// (a mesma escolha de pkg/infra/storage/s3_manager_test.go:100-103).
func newFakeS3Endpoint(t *testing.T, failList bool) *fakeS3Endpoint {
	t.Helper()
	count := 0
	f := &fakeS3Endpoint{mu: make(chan struct{}, 1), count: &count}
	f.mu <- struct{}{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-f.mu
		count++
		f.mu <- struct{}{}
		w.Header().Set("Content-Type", "application/xml")
		if failList {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
				`<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">` +
			`<Name>mybucket</Name><KeyCount>0</KeyCount><MaxKeys>1</MaxKeys>` +
			`<IsTruncated>false</IsTruncated></ListBucketResult>`))
	}))
	t.Cleanup(f.Server.Close)
	return f
}

// calls devolve quantas requisicoes chegaram ao endpoint.
func (f *fakeS3Endpoint) calls() int {
	<-f.mu
	n := *f.count
	f.mu <- struct{}{}
	return n
}
