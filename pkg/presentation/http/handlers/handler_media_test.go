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

// Os cinco handlers de /chat/send de midia (handler_media.go: image, document,
// audio; handler_media_ext.go: sticker, video) repetem o mesmo corpo de
// fronteira: le o userinfo TIPADO do contexto, exige Id, decodifica, executa.
// Sao quatro caminhos de saida >=400 por handler, e todos os quatro tem de
// logar a causa — o log de fronteira do router sabe QUE a requisicao saiu 400,
// nao POR QUE.

// mediaUserInfo e' o valor guardado sob appport.UserInfoKey pelo middleware.
type mediaUserInfo struct{ id string }

func (u mediaUserInfo) Get(key string) string {
	if key == "Id" {
		return u.id
	}
	return ""
}

var _ userInfo = mediaUserInfo{}

// mediaCase descreve um dos cinco handlers de envio de midia.
type mediaCase struct {
	name       string
	route      string
	newHandler func(mc appport.MessageComposer, l appport.Logger) http.Handler
	// validBody decodifica e passa por toda a validacao do use case.
	validBody string
	// incompleteBody decodifica, mas o use case recusa por campo faltante.
	incompleteBody string
	// secretBody carrega os segredos da F9.4 no payload.
	secretBody string
}

func mediaCases() []mediaCase {
	return []mediaCase{
		{
			name:  "image",
			route: "/chat/send/image",
			newHandler: func(mc appport.MessageComposer, l appport.Logger) http.Handler {
				return NewSendImageHandler(message.NewSendImageUseCase(mc, l))
			},
			validBody:      `{"Phone":"5511999999999","Image":"data:image/jpeg;base64,AAAA"}`,
			incompleteBody: `{"Image":"data:image/jpeg;base64,AAAA"}`,
			secretBody:     `{"Phone":"` + logassertAdminToken + `","Image":"` + logassertGlobalEncryptionKey + `"}`,
		},
		{
			name:  "document",
			route: "/chat/send/document",
			newHandler: func(mc appport.MessageComposer, l appport.Logger) http.Handler {
				return NewSendDocumentHandler(message.NewSendDocumentUseCase(mc, l))
			},
			validBody:      `{"Phone":"5511999999999","Document":"data:application/pdf;base64,AAAA","FileName":"a.pdf"}`,
			incompleteBody: `{"Phone":"5511999999999","Document":"data:application/pdf;base64,AAAA"}`,
			secretBody:     `{"Phone":"` + logassertAdminToken + `","Document":"` + logassertGlobalEncryptionKey + `","FileName":"a.pdf"}`,
		},
		{
			name:  "audio",
			route: "/chat/send/audio",
			newHandler: func(mc appport.MessageComposer, l appport.Logger) http.Handler {
				return NewSendAudioHandler(message.NewSendAudioUseCase(mc, l))
			},
			validBody:      `{"Phone":"5511999999999","Audio":"data:audio/ogg;base64,AAAA"}`,
			incompleteBody: `{"Phone":"5511999999999"}`,
			secretBody:     `{"Phone":"` + logassertAdminToken + `","Audio":"` + logassertGlobalEncryptionKey + `"}`,
		},
		{
			name:  "sticker",
			route: "/chat/send/sticker",
			newHandler: func(mc appport.MessageComposer, l appport.Logger) http.Handler {
				return NewSendStickerHandler(message.NewSendStickerUseCase(mc, l))
			},
			validBody:      `{"Phone":"5511999999999","Sticker":"data:image/webp;base64,AAAA"}`,
			incompleteBody: `{"Phone":"5511999999999"}`,
			secretBody:     `{"Phone":"` + logassertAdminToken + `","Sticker":"` + logassertGlobalEncryptionKey + `"}`,
		},
		{
			name:  "video",
			route: "/chat/send/video",
			newHandler: func(mc appport.MessageComposer, l appport.Logger) http.Handler {
				return NewSendVideoHandler(message.NewSendVideoUseCase(mc, l))
			},
			validBody:      `{"Phone":"5511999999999","Video":"data:video/mp4;base64,AAAA"}`,
			incompleteBody: `{"Phone":"5511999999999"}`,
			secretBody:     `{"Phone":"` + logassertAdminToken + `","Video":"` + logassertGlobalEncryptionKey + `"}`,
		},
	}
}

// mediaRequest monta a requisicao; info nil = middleware de auth nao rodou.
func mediaRequest(route, body string, info any) *http.Request {
	r := httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
	if info != nil {
		r = r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, info))
	}
	return r
}

func serveMedia(h http.Handler, r *http.Request) (*httptest.ResponseRecorder, *logCapture) {
	wrapped, capture := logassert.Wrap(h)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, r)
	return rec, capture
}

// TestMediaSendHandlers_Success: payload completo e sessao valida devolvem 200
// e o ID gerado pela porta.
func TestMediaSendHandlers_Success(t *testing.T) {
	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			rec, _ := serveMedia(h, mediaRequest(tc.route, tc.validBody, mediaUserInfo{id: "42"}))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if len(mc.EnsureSessionCalls) != 1 {
				t.Fatalf("EnsureSession chamado %d vezes", len(mc.EnsureSessionCalls))
			}
			if !strings.Contains(rec.Body.String(), contractsfake.DefaultMessageID) {
				t.Fatalf("resposta nao carrega o ID gerado pela porta: %s", rec.Body.String())
			}
		})
	}
}

// TestMediaSendHandlers_NoUserInfo_401: sem userinfo no contexto, 401 com log
// de causa e sem tocar a porta.
func TestMediaSendHandlers_NoUserInfo_401(t *testing.T) {
	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			rec, capture := serveMedia(h, mediaRequest(tc.route, tc.validBody, nil))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, esperado 401", rec.Code)
			}
			if len(mc.EnsureSessionCalls) != 0 {
				t.Fatal("handler tocou a porta antes de decidir que nao ha' usuario")
			}
			got := logassert.OutcomeLogged(t, capture.Records(t), errUnauthorized.Error())
			if got.str("level") != "warn" {
				t.Fatalf("401 e' rejeicao de cliente: nivel %q", got.str("level"))
			}
		})
	}
}

// TestMediaSendHandlers_WrongContextValue_401: um valor que NAO implementa
// userInfo (o caso da chave crua string, ou de um tipo trocado) tem de virar
// 401, nao panico.
func TestMediaSendHandlers_WrongContextValue_401(t *testing.T) {
	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			rec, capture := serveMedia(h, mediaRequest(tc.route, tc.validBody, "nao-implementa-userInfo"))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, esperado 401", rec.Code)
			}
			logassert.OutcomeLogged(t, capture.Records(t), errUnauthorized.Error())
		})
	}
}

// TestMediaSendHandlers_EmptySessionID_400: userinfo presente, Id vazio.
func TestMediaSendHandlers_EmptySessionID_400(t *testing.T) {
	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			rec, capture := serveMedia(h, mediaRequest(tc.route, tc.validBody, mediaUserInfo{id: ""}))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, esperado 400", rec.Code)
			}
			if len(mc.EnsureSessionCalls) != 0 {
				t.Fatal("handler tocou a porta sem Id de sessao")
			}
			got := logassert.OutcomeLogged(t, capture.Records(t), errMissingSessionID.Error())
			if got.str("level") != "warn" {
				t.Fatalf("400 de sessao ausente e' rejeicao de cliente: nivel %q", got.str("level"))
			}
		})
	}
}

// TestMediaSendHandlers_MalformedPayload_400 exercita o ramo de decode.
func TestMediaSendHandlers_MalformedPayload_400(t *testing.T) {
	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			rec, capture := serveMedia(h, mediaRequest(tc.route, `{"Phone":`, mediaUserInfo{id: "42"}))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, esperado 400", rec.Code)
			}
			if len(mc.EnsureSessionCalls) != 0 {
				t.Fatal("handler tocou a porta com payload ilegivel")
			}
			got := logassert.OutcomeLogged(t, capture.Records(t))
			if got.str("level") != "warn" {
				t.Fatalf("payload ilegivel e' rejeicao de cliente: nivel %q", got.str("level"))
			}
		})
	}
}

// TestMediaSendHandlers_IncompletePayload_500: decodifica, mas o use case
// recusa por campo obrigatorio ausente.
func TestMediaSendHandlers_IncompletePayload_500(t *testing.T) {
	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			rec, capture := serveMedia(h, mediaRequest(tc.route, tc.incompleteBody, mediaUserInfo{id: "42"}))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, esperado 500", rec.Code)
			}
			logassert.OutcomeLogged(t, capture.Records(t))
		})
	}
}

// TestMediaSendHandlers_SessionFailure_500: a porta recusa a sessao.
func TestMediaSendHandlers_SessionFailure_500(t *testing.T) {
	const cause = "media-session-refused"

	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{SessionGuard: contractsfake.FailSession(errors.New(cause))}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			rec, capture := serveMedia(h, mediaRequest(tc.route, tc.validBody, mediaUserInfo{id: "42"}))

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

// TestMediaSendHandlers_MessageIDFailure_500: a sessao vale, mas a geracao do
// ID falha. E' o segundo ramo de erro do use case, e sem ele o 500 ficaria
// coberto por um unico caminho.
func TestMediaSendHandlers_MessageIDFailure_500(t *testing.T) {
	const cause = "media-message-id-refused"

	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{
				NewMessageIDFunc: func(context.Context, string) (string, error) {
					return "", errors.New(cause)
				},
			}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			rec, capture := serveMedia(h, mediaRequest(tc.route, tc.validBody, mediaUserInfo{id: "42"}))

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, esperado 500", rec.Code)
			}
			got := logassert.OutcomeLogged(t, capture.Records(t), cause)
			if got.str("level") != "error" {
				t.Fatalf("falha de geracao de ID tem de ser error, foi %q", got.str("level"))
			}
		})
	}
}

// TestMediaSendHandlers_NoSecretLeak planta os tres segredos da F9.4 no
// payload, no Id de sessao e no cabecalho, e exige que o log de saida nao os
// carregue — a regressao permanente.
func TestMediaSendHandlers_NoSecretLeak(t *testing.T) {
	for _, tc := range mediaCases() {
		t.Run(tc.name, func(t *testing.T) {
			mc := &contractsfake.MessageComposer{
				SessionGuard: contractsfake.FailSession(errors.New("media-session-refused")),
			}
			h := tc.newHandler(mc, &contractsfake.Logger{})

			r := mediaRequest(tc.route, tc.secretBody, mediaUserInfo{id: logassertGlobalHMACKey})
			r.Header.Set("Authorization", logassertAdminToken)

			rec, capture := serveMedia(h, r)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, esperado 500", rec.Code)
			}
			logassert.NoSecrets(t, capture.Records(t))
		})
	}
}
