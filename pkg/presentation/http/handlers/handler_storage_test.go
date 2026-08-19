package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/storage"
)

// Os 10 handlers de /storage tem a mesma forma: le o userinfo, exige o Id,
// (opcionalmente) decodifica o corpo, chama o use case. Sao quatro respostas
// >=400 por handler — 34 caminhos de saida no arquivo — e antes desta fase
// nenhum deles logava a causa: o registro de fronteira de router.go dizia
// "status 400" sem dizer POR QUE.
//
// Toda assercao negativa aqui passa pelo co-gate D (logassert), entao um
// caminho que perca o log volta vermelho mesmo que o status continue certo.

// storageSession devolve o fake de sessao que os use cases de storage exigem,
// gravando as chamadas para que toda assercao negativa possa provar que a
// porta NAO foi alcancada.
func storageSession(err error) *contractsfake.SessionGuard {
	return &contractsfake.SessionGuard{
		EnsureSessionFunc: func(context.Context, string) error { return err },
	}
}

// storageCase descreve um handler de /storage e o corpo minimo que ele aceita.
type storageCase struct {
	name  string
	build func(sg appport.SessionGuard) http.Handler
	// method e path da rota real.
	method string
	path   string
	// body e' um corpo que atravessa a validacao do use case sem tocar a rede
	// (endpoint/URL vazios: egress.ValidateOutboundURL nao e' chamado).
	body string
	// readsBody indica se o handler decodifica JSON — so' esses podem ser
	// testados com corpo malformado.
	readsBody bool
}

// testHmacKey tem os 32 caracteres que domain.MinHmacKeyLength exige — abaixo
// disso o use case recusa com 400, e o caso de sucesso da tabela viraria uma
// recusa sem que ninguem percebesse.
const testHmacKey = "0123456789abcdef0123456789abcdef"

// newTestConfigureHmacUC monta o use case de escrita com os dubles das tres
// portas novas. O store e' o de contractsfake, que imita a regra REAL do
// adapter de producao (pkg/infra/db/hmac_config_repository.go).
func newTestConfigureHmacUC(sg appport.SessionGuard, log appport.Logger) *storage.ConfigureHmacUseCase {
	return storage.NewConfigureHmacUseCase(sg, &contractsfake.HmacKeyStore{}, &contractsfake.HmacKeyEncryptor{}, &contractsfake.UserInfoHmacCache{}, log)
}

// Os quatro use cases de S3 com os dubles das quatro portas novas. O store e'
// o de contractsfake, que imita a regra REAL do adapter de producao
// (pkg/infra/db/s3_config_repository.go).
func newTestConfigureS3UC(sg appport.SessionGuard, log appport.Logger) *storage.ConfigureS3UseCase {
	return storage.NewConfigureS3UseCase(sg, &contractsfake.S3ConfigStore{}, &contractsfake.S3SecretCipher{},
		&contractsfake.S3ClientManager{}, &contractsfake.UserInfoS3Cache{}, log)
}

func newTestGetS3ConfigUC(sg appport.SessionGuard, log appport.Logger) *storage.GetS3ConfigUseCase {
	return storage.NewGetS3ConfigUseCase(sg, &contractsfake.S3ConfigStore{}, log)
}

func newTestDeleteS3ConfigUC(sg appport.SessionGuard, log appport.Logger) *storage.DeleteS3ConfigUseCase {
	return storage.NewDeleteS3ConfigUseCase(sg, &contractsfake.S3ConfigStore{}, &contractsfake.S3ClientManager{},
		&contractsfake.UserInfoS3Cache{}, log)
}

// newTestTestS3ConnectionUC recebe o store ja' semeado: o caminho de sucesso
// deste use case exige uma configuracao HABILITADA no banco, e um store vazio
// o transformaria silenciosamente na recusa 400.
func newTestTestS3ConnectionUC(sg appport.SessionGuard, log appport.Logger, store *contractsfake.S3ConfigStore) *storage.TestS3ConnectionUseCase {
	return storage.NewTestS3ConnectionUseCase(sg, store, &contractsfake.S3SecretCipher{}, &contractsfake.S3ClientManager{}, log)
}

// enabledS3Store devolve um store com S3 habilitado para o usuario "42" da
// tabela, com o segredo no envelope do dublê — a forma que o cifrador real
// produziria (ADR-0009).
func enabledS3Store() *contractsfake.S3ConfigStore {
	return &contractsfake.S3ConfigStore{Stored: map[string]appport.S3ConfigRecord{
		"42": {
			Enabled:       true,
			Region:        "us-east-1",
			Bucket:        "b",
			AccessKey:     "ak",
			SecretKey:     contractsfake.FakeS3EnvelopePrefix + "sk",
			MediaDelivery: "base64",
		},
	}}
}

func storageCases() []storageCase {
	log := silentLogger{}
	return []storageCase{
		{
			name: "ConfigureS3",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureS3Handler(newTestConfigureS3UC(sg, log))
			},
			method:    http.MethodPost,
			path:      "/storage/s3/configure",
			body:      `{"enabled":true,"bucket":"b","region":"r"}`,
			readsBody: true,
		},
		{
			name: "GetS3Config",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewGetS3ConfigHandler(newTestGetS3ConfigUC(sg, log))
			},
			method: http.MethodGet,
			path:   "/storage/s3/config",
		},
		{
			name: "TestS3Connection",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewTestS3ConnectionHandler(newTestTestS3ConnectionUC(sg, log, enabledS3Store()))
			},
			method: http.MethodPost,
			path:   "/storage/s3/test",
			// Sem `readsBody`: o teste de conexao NAO decodifica corpo — ele
			// testa a configuracao GRAVADA (`41bc8e2^:handlers.go:6372`).
		},
		{
			name: "DeleteS3Config",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewDeleteS3ConfigHandler(newTestDeleteS3ConfigUC(sg, log))
			},
			method: http.MethodDelete,
			path:   "/storage/s3/config",
		},
		{
			name: "ConfigureHmac",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureHmacHandler(newTestConfigureHmacUC(sg, log))
			},
			method:    http.MethodPost,
			path:      "/storage/hmac/configure",
			body:      `{"hmac_key":"` + testHmacKey + `"}`,
			readsBody: true,
		},
		{
			name: "GetHmacConfig",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewGetHmacConfigHandler(storage.NewGetHmacConfigUseCase(sg, &contractsfake.HmacKeyStore{}, log))
			},
			method: http.MethodGet,
			path:   "/storage/hmac/config",
		},
		{
			name: "DeleteHmacConfig",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewDeleteHmacConfigHandler(storage.NewDeleteHmacConfigUseCase(sg, &contractsfake.HmacKeyStore{}, &contractsfake.UserInfoHmacCache{}, log))
			},
			method: http.MethodDelete,
			path:   "/storage/hmac/config",
		},
		{
			name: "SetProxy",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewSetProxyHandler(storage.NewSetProxyUseCase(sg, log))
			},
			method:    http.MethodPost,
			path:      "/storage/proxy",
			body:      `{"enabled":false}`,
			readsBody: true,
		},
		{
			name: "SetHistory",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewSetHistoryHandler(storage.NewSetHistoryUseCase(sg, log))
			},
			method:    http.MethodPost,
			path:      "/storage/history",
			body:      `{"history":30}`,
			readsBody: true,
		},
		{
			name: "GetHistory",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewGetHistoryHandler(storage.NewGetHistoryUseCase(sg, log))
			},
			method: http.MethodGet,
			path:   "/storage/history",
		},
	}
}

// serveStorage roda o handler dentro da mesma cadeia hlog que router.go
// instala, e devolve a resposta com os registros de log da requisicao.
func serveStorage(t *testing.T, h http.Handler, req *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(h)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	return rec, capture.Records(t)
}

// TestStorageHandlers_Success: com sessao e corpo validos, os 10 devolvem 200
// e o envelope de sucesso — e nao logam nada em warn/error.
func TestStorageHandlers_Success(t *testing.T) {
	for _, tc := range storageCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := storageSession(nil)
			req := withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)), "42")

			rec, recs := serveStorage(t, tc.build(sg), req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (corpo: %s)", rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)
			if !env.Success || env.Code != http.StatusOK {
				t.Fatalf("envelope de sucesso mal formado: %s", rec.Body.String())
			}
			if len(env.Data) == 0 {
				t.Fatal("resposta 200 sem data")
			}
			if len(sg.EnsureSessionCalls) != 1 {
				t.Fatalf("EnsureSession chamado %d vez(es), queria 1", len(sg.EnsureSessionCalls))
			}
			logassert.NoSecrets(t, recs)
		})
	}
}

// TestStorageHandlers_Unauthenticated_401: sem o valor que o middleware
// injeta, nenhum handler alcanca o use case — e a rejeicao vai para o log com
// a causa.
func TestStorageHandlers_Unauthenticated_401(t *testing.T) {
	for _, tc := range storageCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := storageSession(nil)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))

			rec, recs := serveStorage(t, tc.build(sg), req)

			assertErrorEnvelope(t, rec, http.StatusUnauthorized)
			if len(sg.EnsureSessionCalls) != 0 {
				t.Fatalf("requisicao nao autenticada alcancou a sessao %d vez(es)", len(sg.EnsureSessionCalls))
			}
			logassert.OutcomeLogged(t, recs, errUnauthorized.Error())
		})
	}
}

// TestStorageHandlers_MissingSessionID_400: userinfo presente mas sem Id — a
// rejeicao e' do cliente (warn), nao falha do servidor.
func TestStorageHandlers_MissingSessionID_400(t *testing.T) {
	for _, tc := range storageCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := storageSession(nil)
			req := withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)), "")

			rec, recs := serveStorage(t, tc.build(sg), req)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			if len(sg.EnsureSessionCalls) != 0 {
				t.Fatalf("requisicao sem session id alcancou a sessao %d vez(es)", len(sg.EnsureSessionCalls))
			}
			got := logassert.OutcomeLogged(t, recs, errMissingSessionID.Error())
			if got.str("level") != "warn" {
				t.Fatalf("rejeicao de cliente logada em %q — queria warn", got.str("level"))
			}
		})
	}
}

// TestStorageHandlers_MalformedBody_400: corpo que nao e' JSON para nos cinco
// handlers que decodificam.
func TestStorageHandlers_MalformedBody_400(t *testing.T) {
	for _, tc := range storageCases() {
		if !tc.readsBody {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			sg := storageSession(nil)
			req := withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader("{nao-e-json")), "42")

			rec, recs := serveStorage(t, tc.build(sg), req)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			if len(sg.EnsureSessionCalls) != 0 {
				t.Fatalf("corpo malformado alcancou a sessao %d vez(es)", len(sg.EnsureSessionCalls))
			}
			got := logassert.OutcomeLogged(t, recs)
			if got.str("error") == "" {
				t.Fatal("o erro de decodificacao foi logado sem causa")
			}
		})
	}
}

// TestStorageHandlers_BodyIsNotReadByHandlersThatDoNotDecode: os cinco
// handlers sem corpo nao podem rejeitar por causa dele. Sem esta assercao, um
// `readsBody` errado na tabela deixaria o caso acima vacuo.
func TestStorageHandlers_BodyIsNotReadByHandlersThatDoNotDecode(t *testing.T) {
	for _, tc := range storageCases() {
		if tc.readsBody {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			sg := storageSession(nil)
			req := withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader("{nao-e-json")), "42")

			rec, _ := serveStorage(t, tc.build(sg), req)

			if rec.Code != http.StatusOK {
				t.Fatalf("handler sem corpo rejeitou corpo malformado: status %d", rec.Code)
			}
		})
	}
}

// TestStorageHandlers_SessionFailure_500: a sessao do wa-noise falta. E' falha
// de dependencia, entao vai em error — e a causa do use case chega ao log.
func TestStorageHandlers_SessionFailure_500(t *testing.T) {
	sessionErr := errors.New("no wanoise session for user")
	for _, tc := range storageCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := storageSession(sessionErr)
			req := withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)), "42")

			rec, recs := serveStorage(t, tc.build(sg), req)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			if len(sg.EnsureSessionCalls) != 1 {
				t.Fatalf("EnsureSession chamado %d vez(es), queria 1", len(sg.EnsureSessionCalls))
			}
			got := logassert.OutcomeLogged(t, recs, sessionErr.Error())
			if got.str("level") != "error" {
				t.Fatalf("falha de dependencia logada em %q — queria error", got.str("level"))
			}
			if got.str("user") != "42" {
				t.Fatalf("o registro nao correlaciona com o usuario: %s", got.Raw)
			}
		})
	}
}

// TestStorageHandlers_UseCaseRejection_500: a sessao existe, mas o use case
// recusa o conteudo. E' o unico caminho em que a causa nasce no dominio, e
// nao na fronteira — e o handler tem de leva-la ao log do mesmo jeito.
// 400 desde a F66: campo obrigatorio ausente ou valor invalido no payload e
// erro do CLIENTE. Estes testes exigiam 500 — dois com "_500" no proprio
// nome —, fixando o defeito que a F66 corrige.
func TestStorageHandlers_UseCaseRejection_400(t *testing.T) {
	log := silentLogger{}
	cases := []struct {
		name    string
		build   func(sg appport.SessionGuard) http.Handler
		method  string
		path    string
		body    string
		wantErr string
	}{
		{
			name: "ConfigureS3/media_delivery invalido",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureS3Handler(newTestConfigureS3UC(sg, log))
			},
			method:  http.MethodPost,
			path:    "/storage/s3/configure",
			body:    `{"enabled":true,"media_delivery":"carrier-pigeon"}`,
			wantErr: "media_delivery",
		},
		{
			// A recusa deste use case deixou de ser "campo obrigatorio
			// ausente no corpo" e passou a ser "nao ha' configuracao
			// habilitada gravada" — porque ele deixou de ler o corpo. O
			// store VAZIO e' o estado de quem nunca configurou.
			name: "TestS3Connection/S3 nao habilitado",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewTestS3ConnectionHandler(newTestTestS3ConnectionUC(sg, log, &contractsfake.S3ConfigStore{}))
			},
			method:  http.MethodPost,
			path:    "/storage/s3/test",
			body:    ``,
			wantErr: "S3 is not enabled for this user",
		},
		{
			name: "SetProxy/habilitado sem URL",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewSetProxyHandler(storage.NewSetProxyUseCase(sg, log))
			},
			method:  http.MethodPost,
			path:    "/storage/proxy",
			body:    `{"enabled":true}`,
			wantErr: "proxy URL is required",
		},
		{
			name: "ConfigureHmac/chave curta",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureHmacHandler(newTestConfigureHmacUC(sg, log))
			},
			method:  http.MethodPost,
			path:    "/storage/hmac/configure",
			body:    `{"hmac_key":"curta"}`,
			wantErr: "HMAC key must be at least 32 characters long",
		},
		{
			name: "SetHistory/valor negativo",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewSetHistoryHandler(storage.NewSetHistoryUseCase(sg, log))
			},
			method:  http.MethodPost,
			path:    "/storage/history",
			body:    `{"history":-1}`,
			wantErr: "history value cannot be negative",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := withUser(httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)), "42")

			rec, recs := serveStorage(t, tc.build(storageSession(nil)), req)

			assertErrorEnvelope(t, rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, recs, tc.wantErr)
		})
	}
}

// TestStorageHandlers_PayloadSecretsNeverReachTheLog e' a clausula (d) do
// co-gate D exercitada contra codigo real: /storage/s3 e /storage/hmac sao os
// dois handlers do repositorio que recebem material de chave no CORPO. Se
// alguem trocar `.Err(err)` por um dump do payload, este teste acusa.
func TestStorageHandlers_PayloadSecretsNeverReachTheLog(t *testing.T) {
	log := silentLogger{}
	cases := []struct {
		name  string
		build func(sg appport.SessionGuard) http.Handler
		path  string
		body  string
	}{
		{
			name: "ConfigureS3",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureS3Handler(newTestConfigureS3UC(sg, log))
			},
			path: "/storage/s3/configure",
			body: `{"enabled":true,"access_key":"` + logassertAdminToken +
				`","secret_key":"` + logassertGlobalEncryptionKey + `"}`,
		},
		{
			name: "ConfigureHmac",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureHmacHandler(newTestConfigureHmacUC(sg, log))
			},
			path: "/storage/hmac/configure",
			body: `{"hmac_key":"` + logassertGlobalHMACKey + `"}`,
		},
		{
			name: "TestS3Connection",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewTestS3ConnectionHandler(newTestTestS3ConnectionUC(sg, log, enabledS3Store()))
			},
			path: "/storage/s3/test",
			body: `{"secret_key":"` + logassertGlobalHMACKey + `"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Sessao quebrada: e' o caminho que MAIS loga, logo o que mais
			// pode vazar.
			sg := storageSession(errors.New("no wanoise session for user"))
			req := withUser(httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)), "42")

			rec, recs := serveStorage(t, tc.build(sg), req)

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
			if len(recs) == 0 {
				t.Fatal("o caminho de saida nao logou — a checagem de vazamento fica vacua")
			}
			logassert.NoSecrets(t, recs)
		})
	}
}
