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

func storageCases() []storageCase {
	log := silentLogger{}
	return []storageCase{
		{
			name: "ConfigureS3",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureS3Handler(storage.NewConfigureS3UseCase(sg, log))
			},
			method:    http.MethodPost,
			path:      "/storage/s3/configure",
			body:      `{"enabled":true,"bucket":"b","region":"r"}`,
			readsBody: true,
		},
		{
			name: "GetS3Config",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewGetS3ConfigHandler(storage.NewGetS3ConfigUseCase(sg, log))
			},
			method: http.MethodGet,
			path:   "/storage/s3/config",
		},
		{
			name: "TestS3Connection",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewTestS3ConnectionHandler(storage.NewTestS3ConnectionUseCase(sg, log))
			},
			method:    http.MethodPost,
			path:      "/storage/s3/test",
			body:      `{"endpoint":"e","region":"r","bucket":"b","access_key":"ak","secret_key":"sk"}`,
			readsBody: true,
		},
		{
			name: "DeleteS3Config",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewDeleteS3ConfigHandler(storage.NewDeleteS3ConfigUseCase(sg, log))
			},
			method: http.MethodDelete,
			path:   "/storage/s3/config",
		},
		{
			name: "ConfigureHmac",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureHmacHandler(storage.NewConfigureHmacUseCase(sg, log))
			},
			method:    http.MethodPost,
			path:      "/storage/hmac/configure",
			body:      `{"enabled":true,"key":"k","secret":"s"}`,
			readsBody: true,
		},
		{
			name: "GetHmacConfig",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewGetHmacConfigHandler(storage.NewGetHmacConfigUseCase(sg, log))
			},
			method: http.MethodGet,
			path:   "/storage/hmac/config",
		},
		{
			name: "DeleteHmacConfig",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewDeleteHmacConfigHandler(storage.NewDeleteHmacConfigUseCase(sg, log))
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

// TestStorageHandlers_SessionFailure_500: a sessao do whatsmeow falta. E' falha
// de dependencia, entao vai em error — e a causa do use case chega ao log.
func TestStorageHandlers_SessionFailure_500(t *testing.T) {
	sessionErr := errors.New("no whatsmeow session for user")
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
func TestStorageHandlers_UseCaseRejection_500(t *testing.T) {
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
				return NewConfigureS3Handler(storage.NewConfigureS3UseCase(sg, log))
			},
			method:  http.MethodPost,
			path:    "/storage/s3/configure",
			body:    `{"enabled":true,"media_delivery":"carrier-pigeon"}`,
			wantErr: "media_delivery",
		},
		{
			name: "TestS3Connection/campos obrigatorios ausentes",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewTestS3ConnectionHandler(storage.NewTestS3ConnectionUseCase(sg, log))
			},
			method:  http.MethodPost,
			path:    "/storage/s3/test",
			body:    `{"endpoint":"e"}`,
			wantErr: "missing required S3 configuration fields",
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

			assertErrorEnvelope(t, rec, http.StatusInternalServerError)
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
				return NewConfigureS3Handler(storage.NewConfigureS3UseCase(sg, log))
			},
			path: "/storage/s3/configure",
			body: `{"enabled":true,"access_key":"` + logassertAdminToken +
				`","secret_key":"` + logassertGlobalEncryptionKey + `"}`,
		},
		{
			name: "ConfigureHmac",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewConfigureHmacHandler(storage.NewConfigureHmacUseCase(sg, log))
			},
			path: "/storage/hmac/configure",
			body: `{"enabled":true,"key":"k","secret":"` + logassertGlobalHMACKey + `"}`,
		},
		{
			name: "TestS3Connection",
			build: func(sg appport.SessionGuard) http.Handler {
				return NewTestS3ConnectionHandler(storage.NewTestS3ConnectionUseCase(sg, log))
			},
			path: "/storage/s3/test",
			body: `{"secret_key":"` + logassertGlobalHMACKey + `"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Sessao quebrada: e' o caminho que MAIS loga, logo o que mais
			// pode vazar.
			sg := storageSession(errors.New("no whatsmeow session for user"))
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
