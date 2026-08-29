package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	"wa-api/pkg/presentation/http/contracttest"
)

// Teste de contrato da superfície de GESTÃO de conversa da família mensagens:
// presença, marcar como lida, temporizador efémero, estrela e descarga.
//
// O que ele afirma, e que os testes por rota que já existiam NÃO afirmavam:
// que a chave servida é snake_case minúsculo. O de descarga em particular
// decodificava com `json:"Mimetype"`, e o `encoding/json` do Go casa chaves
// SEM distinguir maiúsculas na descodificação — a suíte inteira continuaria
// verde com o nome errado no fio.

// mgmtContractCase é uma rota migrada, com o seu router registado e o corpo
// que a exercita com sucesso.
type mgmtContractCase struct {
	nome string
	rota string
	// proibidas são as chaves que a migração tinha de FAZER DESAPARECER.
	proibidas []string
	serve     func(t *testing.T) *httptest.ResponseRecorder
}

func mgmtPost(t *testing.T, h http.Handler, rota, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, rota, strings.NewReader(corpo))
	h.ServeHTTP(rec, msgAuthed(req))
	return rec
}

// mgmtRouter monta UMA rota como wiring_routes.go a monta.
func mgmtRouter(rota string, h http.Handler) *mux.Router {
	r := mux.NewRouter()
	r.Handle(rota, h).Methods(http.MethodPost)
	return r
}

func mgmtContractCases() []mgmtContractCase {
	log := silentLogger{}
	return []mgmtContractCase{
		{
			nome:      "presence",
			rota:      "/user/presence",
			proibidas: []string{"Details"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				d := newPresenceDeps()
				return mgmtPost(t, mgmtRouter("/user/presence",
					NewSendPresenceHandler(message.NewSendPresenceUseCase(d.presence, log))),
					"/user/presence", `{"type":"available"}`)
			},
		},
		{
			nome:      "chatpresence",
			rota:      "/chat/presence",
			proibidas: []string{"Details"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				d := newPresenceDeps()
				return mgmtPost(t, mgmtRouter("/chat/presence",
					NewChatPresenceHandler(message.NewChatPresenceUseCase(d.presence, d.jids, log))),
					"/chat/presence", `{"phone":"5511999999999","state":"composing"}`)
			},
		},
		{
			nome:      "markread",
			rota:      "/chat/markread",
			proibidas: []string{"Details"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				d := newPresenceDeps()
				return mgmtPost(t, mgmtRouter("/chat/markread",
					NewMarkReadHandler(message.NewMarkReadUseCase(d.messenger, d.jids, log))),
					"/chat/markread", `{"id":["MSG1"],"chat_phone":"5511999999999"}`)
			},
		},
		{
			nome:      "ephemeral",
			rota:      "/chat/ephemeral",
			proibidas: []string{"Details"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				ops, jids, logger := disappearingFakes()
				return mgmtPost(t, mgmtRouter("/chat/ephemeral", disappearingHandler(ops, jids, logger)),
					"/chat/ephemeral", `{"chat":"5511999999999@s.whatsapp.net","duration":"24h"}`)
			},
		},
		{
			nome:      "star",
			rota:      "/message/star",
			proibidas: []string{"Success", "Message"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				h := NewStarMessageHandler(chat.NewStarMessageUseCase(
					&contractsfake.MessageStarrer{}, &contractsfake.JIDResolver{}, log))
				return mgmtPost(t, mgmtRouter("/message/star", h), "/message/star",
					`{"chat":"120363111111111111@g.us","sender":"5511999999999@s.whatsapp.net",`+
						`"message_id":"ABCDE12345","from_me":false,"star":true}`)
			},
		},
		{
			// HOUSEKEEP F323: /chat/mute decoded domain.MuteChatRequest
			// directly and returned *domain.MuteChatResult with no DTO
			// indirection. The wire keys already matched — this case proves
			// the DTO layer (dtomessage.MuteChatRequest/PresentMuteChat) is
			// now what the real registered route exercises.
			nome:      "mute",
			rota:      "/chat/mute",
			proibidas: []string{"Success", "Message", "Jid", "Mute", "MuteDuration"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				ops := &contractsfake.ChatOperations{}
				jids := &contractsfake.JIDResolver{}
				h := NewMuteChatHandler(chat.NewMuteChatUseCase(ops, jids, log))
				return mgmtPost(t, mgmtRouter("/chat/mute", h), "/chat/mute",
					`{"jid":"5511999999999@s.whatsapp.net","mute":true}`)
			},
		},
		{
			// HOUSEKEEP F323, same class of defect as "mute".
			nome:      "archive",
			rota:      "/chat/archive",
			proibidas: []string{"Success", "Message", "Jid", "Archive"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				ops := &contractsfake.ChatOperations{}
				jids := &contractsfake.JIDResolver{}
				h := NewArchiveChatHandler(chat.NewArchiveChatUseCase(ops, jids, log))
				return mgmtPost(t, mgmtRouter("/chat/archive", h), "/chat/archive",
					`{"jid":"5511999999999@s.whatsapp.net","archive":true}`)
			},
		},
		{
			// HOUSEKEEP F323, same class of defect as "mute".
			nome:      "pin",
			rota:      "/chat/pin",
			proibidas: []string{"Success", "Message", "Jid", "Pin"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				pinner := &contractsfake.ChatPinner{}
				jids := &contractsfake.JIDResolver{}
				h := NewPinChatHandler(chat.NewPinChatUseCase(pinner, jids, log))
				return mgmtPost(t, mgmtRouter("/chat/pin", h), "/chat/pin",
					`{"jid":"5511999999999@s.whatsapp.net","pin":true}`)
			},
		},
		{
			// HOUSEKEEP F323, same class of defect as "mute".
			nome:      "requestunavailablemessage",
			rota:      "/chat/request-unavailable-message",
			proibidas: []string{"Success", "Message", "RequestID", "Chat", "Sender", "MessageID", "Timestamp"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				ops := &contractsfake.ChatOperations{}
				jids := &contractsfake.JIDResolver{}
				h := NewRequestUnavailableMessageHandler(chat.NewRequestUnavailableMessageUseCase(ops, jids, log))
				return mgmtPost(t, mgmtRouter("/chat/request-unavailable-message", h),
					"/chat/request-unavailable-message",
					`{"chat":"5511999999999@s.whatsapp.net","sender":"5511888888888@s.whatsapp.net","id":"MSG1"}`)
			},
		},
		{
			nome:      "downloadimage",
			rota:      "/chats/download/image",
			proibidas: []string{"Mimetype", "Data"},
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				c := downloadRouteCases()[0]
				md := &contractsfake.MediaDownloader{
					DownloadFunc: func(context.Context, string, domain.MediaDescriptor) ([]byte, error) {
						return []byte{0x01, 0x02, 0x03}, nil
					},
				}
				return mgmtPost(t, c.router(md), c.route(), c.body())
			},
		},
	}
}

// TestChatMgmt_ContratoPublico_NomesCanonicos é a afirmação principal: toda
// chave do corpo servido, recursivamente, é snake_case minúsculo.
func TestChatMgmt_ContratoPublico_NomesCanonicos(t *testing.T) {
	for _, caso := range mgmtContractCases() {
		t.Run(caso.nome, func(t *testing.T) {
			rec := caso.serve(t)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, quero 200; corpo: %s", caso.rota, rec.Code, rec.Body.String())
			}
			contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
		})
	}
}

// TestChatMgmt_ContratoPublico_ChavesAntigasSumiram é a segunda metade: "a
// chave nova existe" não prova migração, porque um struct pode carregar as
// duas — e no caso da descarga o decodificador do Go nem sequer distinguia.
func TestChatMgmt_ContratoPublico_ChavesAntigasSumiram(t *testing.T) {
	for _, caso := range mgmtContractCases() {
		t.Run(caso.nome, func(t *testing.T) {
			rec := caso.serve(t)
			contracttest.AssertNoKeys(t, rec.Body.Bytes(), caso.proibidas...)
		})
	}
}
