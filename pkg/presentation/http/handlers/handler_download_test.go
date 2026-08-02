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
	"wa-api/pkg/application/usecase/message"
)

// Os cinco handlers de /chat/download* sao o mesmo corpo repetido cinco vezes:
// sessionUser, decode, Execute, responde. Testa-los em tabela e' o que faz uma
// divergencia entre as cinco copias aparecer — e' exatamente o tipo de bug que
// a duplicacao produz.

// downloadUserInfo e' o valor que o middleware de auth guarda no contexto.
type downloadUserInfo struct{ id string }

func (u downloadUserInfo) Get(key string) string {
	if key == "Id" {
		return u.id
	}
	return ""
}

// downloadCase descreve um dos cinco handlers: como construi-lo sobre as
// portas fake e qual o corpo valido da sua requisicao.
type downloadCase struct {
	name       string
	newHandler func(sg appport.SessionGuard, l appport.Logger) http.Handler
}

func downloadCases() []downloadCase {
	return []downloadCase{
		{"image", func(sg appport.SessionGuard, l appport.Logger) http.Handler {
			return NewDownloadImageHandler(message.NewDownloadImageUseCase(sg, l))
		}},
		{"video", func(sg appport.SessionGuard, l appport.Logger) http.Handler {
			return NewDownloadVideoHandler(message.NewDownloadVideoUseCase(sg, l))
		}},
		{"audio", func(sg appport.SessionGuard, l appport.Logger) http.Handler {
			return NewDownloadAudioHandler(message.NewDownloadAudioUseCase(sg, l))
		}},
		{"document", func(sg appport.SessionGuard, l appport.Logger) http.Handler {
			return NewDownloadDocumentHandler(message.NewDownloadDocumentUseCase(sg, l))
		}},
		{"sticker", func(sg appport.SessionGuard, l appport.Logger) http.Handler {
			return NewDownloadStickerHandler(message.NewDownloadStickerUseCase(sg, l))
		}},
	}
}

const downloadValidBody = `{"Url":"https://mmg.whatsapp.net/d/f/abc.enc","Mimetype":"image/jpeg"}`

// downloadRequest monta a requisicao com (ou sem) userinfo no contexto.
func downloadRequest(body string, info any) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/chat/downloadimage", strings.NewReader(body))
	if info != nil {
		r = r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, info))
	}
	return r
}

// serveDownload executa o handler sob a mesma cadeia hlog de producao.
func serveDownload(h http.Handler, r *http.Request) (*httptest.ResponseRecorder, *logCapture) {
	wrapped, capture := logassert.Wrap(h)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, r)
	return rec, capture
}

// TestDownloadHandlers_Success: sessao valida e payload bem formado devolvem
// 200 e alcancam o use case exatamente uma vez.
func TestDownloadHandlers_Success(t *testing.T) {
	for _, tc := range downloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := &contractsfake.SessionGuard{}
			h := tc.newHandler(sg, &contractsfake.Logger{})

			rec, _ := serveDownload(h, downloadRequest(downloadValidBody, downloadUserInfo{id: "42"}))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if len(sg.EnsureSessionCalls) != 1 {
				t.Fatalf("EnsureSession chamado %d vezes", len(sg.EnsureSessionCalls))
			}
			if got := sg.EnsureSessionCalls[0].TxtID; got != "42" {
				t.Fatalf("use case recebeu txtID %q, esperado o Id do contexto", got)
			}
		})
	}
}

// TestDownloadHandlers_NoUserInfo_401: sem o middleware de auth, 401 e o use
// case NAO e' alcancado.
func TestDownloadHandlers_NoUserInfo_401(t *testing.T) {
	for _, tc := range downloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := &contractsfake.SessionGuard{}
			h := tc.newHandler(sg, &contractsfake.Logger{})

			rec, _ := serveDownload(h, downloadRequest(downloadValidBody, nil))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, esperado 401", rec.Code)
			}
			if len(sg.EnsureSessionCalls) != 0 {
				t.Fatal("handler falou com a sessao DEPOIS de decidir que nao ha' usuario")
			}
		})
	}
}

// TestDownloadHandlers_EmptySessionID_400: userinfo presente mas sem Id.
func TestDownloadHandlers_EmptySessionID_400(t *testing.T) {
	for _, tc := range downloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := &contractsfake.SessionGuard{}
			h := tc.newHandler(sg, &contractsfake.Logger{})

			rec, _ := serveDownload(h, downloadRequest(downloadValidBody, downloadUserInfo{id: ""}))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, esperado 400", rec.Code)
			}
			if len(sg.EnsureSessionCalls) != 0 {
				t.Fatal("handler falou com a sessao mesmo sem Id")
			}
		})
	}
}

// TestDownloadHandlers_MalformedPayload_400 exercita o ramo de decode. Co-gate
// D: o 400 tem de sair com registro warn/error carregando a causa e o req_id.
func TestDownloadHandlers_MalformedPayload_400(t *testing.T) {
	for _, tc := range downloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := &contractsfake.SessionGuard{}
			h := tc.newHandler(sg, &contractsfake.Logger{})

			rec, capture := serveDownload(h, downloadRequest(`{"Url":`, downloadUserInfo{id: "42"}))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, esperado 400", rec.Code)
			}
			if len(sg.EnsureSessionCalls) != 0 {
				t.Fatal("handler falou com a sessao com payload ilegivel")
			}
			got := logassert.OutcomeLogged(t, capture.Records(t))
			if got.str("level") != "warn" {
				t.Fatalf("payload ilegivel e' rejeicao de cliente: nivel %q", got.str("level"))
			}
		})
	}
}

// TestDownloadHandlers_SessionFailure_500: a sessao recusa, o use case propaga,
// e o handler responde 500 logando a causa em error.
func TestDownloadHandlers_SessionFailure_500(t *testing.T) {
	const cause = "download-session-refused"

	for _, tc := range downloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := contractsfake.FailSession(errors.New(cause))
			h := tc.newHandler(&sg, &contractsfake.Logger{})

			rec, capture := serveDownload(h, downloadRequest(downloadValidBody, downloadUserInfo{id: "42"}))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, esperado 500", rec.Code)
			}
			got := logassert.OutcomeLogged(t, capture.Records(t), cause)
			if got.str("level") != "error" {
				t.Fatalf("falha real de sessao tem de ser error, foi %q", got.str("level"))
			}
		})
	}
}

// TestDownloadHandlers_MissingURL_500: o payload decodifica, mas o use case
// recusa por falta de Url. Ramo de erro distinto do de sessao.
func TestDownloadHandlers_MissingURL_500(t *testing.T) {
	for _, tc := range downloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := &contractsfake.SessionGuard{}
			h := tc.newHandler(sg, &contractsfake.Logger{})

			rec, capture := serveDownload(h, downloadRequest(`{"Mimetype":"image/jpeg"}`, downloadUserInfo{id: "42"}))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, esperado 500", rec.Code)
			}
			if len(sg.EnsureSessionCalls) != 0 {
				t.Fatal("use case checou sessao antes de validar o payload")
			}
			logassert.OutcomeLogged(t, capture.Records(t))
		})
	}
}

// TestDownloadHandlers_NoSecretLeak planta os tres segredos da F9.4 no caminho
// do handler — corpo e cabecalho — e exige que o log de saida nao os carregue.
func TestDownloadHandlers_NoSecretLeak(t *testing.T) {
	for _, tc := range downloadCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := contractsfake.FailSession(errors.New("download-session-refused"))
			h := tc.newHandler(&sg, &contractsfake.Logger{})

			body := `{"Url":"https://example.invalid/` + logassertGlobalEncryptionKey + `"}`
			r := downloadRequest(body, downloadUserInfo{id: logassertGlobalHMACKey})
			r.Header.Set("Authorization", logassertAdminToken)

			rec, capture := serveDownload(h, r)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, esperado 500", rec.Code)
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}
