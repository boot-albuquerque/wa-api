package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

// CAP-19 — os quatro eixos de fronteira do CAP-18, mais o eixo do txtID da
// F142, nas capabilities que NAO sao de envio.
//
//	1. missing session id     — autenticado, `Id` vazio => 400, porta intacta
//	2. corpo malformado       — JSON truncado => 400, porta intacta
//	3. sucesso nao loga       — caminho feliz sem registro warn/error
//	4. wrong type in context  — valor que nao satisfaz userInfo => 401, sem panico
//	5. txtID na porta         — o id que chega a porta e' o do CONTEXTO (F142)
//
// Por que este arquivo existe, medido e nao suposto:
//
//   - `delete` e `edit` ja' tinham os cinco pela rota registrada
//     (handler_message_mutation_test.go) e NAO estao aqui.
//   - `presence`, `markread`, `react` e as quatro capabilities de
//     handler_misc_test.go tinham os eixos 1, 2 e 3 — mas por
//     `handler.ServeHTTP` CRU (ipmServe recebe o handler, nao um roteador).
//     Cobertura por handler cru nao exercita a cadeia da rota (ARMADILHA 2),
//     e o preco disso e' visivel: handler_misc_test.go pede
//     "/chat/rejectcall" e "/chat/requestunavailablemessage", DUAS rotas que
//     nao existem (as reais sao "/call/reject" e
//     "/chat/request-unavailable-message", wiring_routes.go:158,160). Com
//     handler cru o caminho da requisicao e' decorativo, entao o engano
//     nunca falhou nada.
//   - `download` tinha os eixos 1 e 2 pela rota registrada, mas sem assercao
//     de CAUSA no eixo 1; o eixo 3 nao existia (o teste de sucesso nao
//     captura log) e o eixo 4 nao existia em forma nenhuma.
//
// O eixo 4 e' o de maior valor aqui: dez destas rotas passam por
// `sessionUser` (handler_session.go:38), que faz a assercao de tipo de UMA
// variavel e depende do `if info == nil` seguinte para nao entregar tipo
// errado adiante. Nada travava esse par ate' agora.
//
// Uma tabela unica, e nao dezessete copias: DUPLICACAO foi a causa raiz da
// F143.

// nonSendAxisOutcome e' o resultado de UMA requisicao pela rota registrada.
type nonSendAxisOutcome struct {
	rec *httptest.ResponseRecorder
	// recs e' a saida da mesma cadeia hlog que router.go instala.
	recs []logLine
	// portCalls conta as chamadas da porta de acao da capability.
	portCalls int
	// portTxtID e' o txtID que a porta RECEBEU na primeira chamada. Vazio
	// quando a porta nao foi alcancada.
	portTxtID string
}

// nonSendAxisCase e' UMA capability vista pelos cinco eixos.
type nonSendAxisCase struct {
	// nome e' o rotulo do subteste, sem barra, para que `go test -run` possa
	// isolar UMA capability — que e' o que o controle negativo precisa fazer.
	nome string
	// rota e' a rota REGISTRADA, copiada de wiring_routes.go. E' ela que o
	// roteador do caso registra e que a requisicao pede.
	rota string
	// validBody e' o corpo que produz 200 no caminho feliz.
	validBody string
	// decodeCause e' a substring que o campo `error` do registro tem de
	// carregar quando o corpo e' malformado.
	//
	// Ela DIVERGE entre familias de handler, e a divergencia e' medida:
	// handler_presence.go, handler_reaction.go e handler_misc.go logam o
	// erro-sentinela `errDecodePayload` ("could not decode payload"),
	// enquanto handler_download.go loga o erro CRU do json.Decoder — que
	// para o corpo truncado deste arquivo e' "unexpected EOF". E' a mesma
	// inconsistencia da F141 que o CAP-18 nomeou nas capabilities de envio;
	// uniformizar a assercao a esconderia.
	decodeCause string
	// serve faz UMA requisicao pela rota REGISTRADA com a mutacao dada, sob
	// captura de log, e devolve o que os eixos observam.
	serve func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome
}

// nonSendAxisServe e' o unico ponto do arquivo que monta requisicao: envolve
// o ROTEADOR na cadeia hlog de producao, faz o POST e devolve resposta e log.
func nonSendAxisServe(t *testing.T, h http.Handler, target, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

// nonSendAxisRouter registra UM handler na rota exata, com o metodo exato,
// como registerCustomRoutes faz.
func nonSendAxisRouter(path string, h http.Handler) http.Handler {
	r := mux.NewRouter()
	r.Handle(path, h).Methods(http.MethodPost)
	return r
}

const nonSendAxisMalformedBody = `{"Phone":"5511`

const (
	// nonSendAxisDecodeCauseRaw e' a causa das capabilities que logam o erro
	// cru do json.Decoder para o corpo truncado acima.
	nonSendAxisDecodeCauseRaw = "unexpected EOF"
	// nonSendAxisDecodeCauseSentinel e' a causa das que logam errDecodePayload.
	nonSendAxisDecodeCauseSentinel = "could not decode payload"
)

// nonSendAxisDownloadBody monta o payload de download com o MIME da
// capability, como handler_download_test.go faz.
func nonSendAxisDownloadBody(mime string) string {
	return `{"Url":"https://mmg.whatsapp.net/d/f/AbCdEf.enc",` +
		`"DirectPath":"/v/t62.7118-24/12345_678_90.enc",` +
		`"MediaKey":"` + base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03, 0x04}) + `",` +
		`"Mimetype":"` + mime + `",` +
		`"FileEncSHA256":"` + base64.StdEncoding.EncodeToString([]byte{0xaa, 0xbb}) + `",` +
		`"FileSHA256":"` + base64.StdEncoding.EncodeToString([]byte{0xcc, 0xdd}) + `",` +
		`"FileLength":4242}`
}

// nonSendAxisDownloadCase monta um dos cinco casos de download. As cinco
// rotas divergem em construtor, MIME e nada mais — mas continuam CINCO
// casos, com nome e rota proprios, e nao um caso parametrizado por indice.
func nonSendAxisDownloadCase(nome, rota, mime string, novo func(appport.MediaDownloader, appport.Logger) http.Handler) nonSendAxisCase {
	return nonSendAxisCase{
		nome:        nome,
		rota:        rota,
		validBody:   nonSendAxisDownloadBody(mime),
		decodeCause: nonSendAxisDecodeCauseRaw,
		serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(context.Context, string, domain.MediaDescriptor) ([]byte, error) {
					return []byte{0x00, 0x01, 'o', 'k'}, nil
				},
			}
			rec, recs := nonSendAxisServe(t, nonSendAxisRouter(rota, novo(md, silentLogger{})), rota, body, mut)
			out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(md.DownloadCalls)}
			if out.portCalls > 0 {
				out.portTxtID = md.DownloadCalls[0].TxtID
			}
			return out
		},
	}
}

func nonSendAxisCases() []nonSendAxisCase {
	return []nonSendAxisCase{
		{
			nome:        "SendPresence",
			rota:        "/user/presence",
			validBody:   `{"type":"available"}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				pc := &contractsfake.PresenceController{}
				h := NewSendPresenceHandler(message.NewSendPresenceUseCase(pc, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/user/presence", h), "/user/presence", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(pc.SendPresenceCalls)}
				if out.portCalls > 0 {
					out.portTxtID = pc.SendPresenceCalls[0].TxtID
				}
				return out
			},
		},
		{
			nome:        "SubscribePresence",
			rota:        "/user/presence/subscribe",
			validBody:   `{"Phone":"5511999999999"}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				pc := &contractsfake.PresenceController{}
				h := NewSubscribePresenceHandler(message.NewSubscribePresenceUseCase(pc, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/user/presence/subscribe", h), "/user/presence/subscribe", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(pc.SubscribePresenceCalls)}
				if out.portCalls > 0 {
					out.portTxtID = pc.SubscribePresenceCalls[0].TxtID
				}
				return out
			},
		},
		{
			nome:        "ChatPresence",
			rota:        "/chat/presence",
			validBody:   `{"Phone":"5511999999999","State":"composing"}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				pc := &contractsfake.PresenceController{}
				h := NewChatPresenceHandler(message.NewChatPresenceUseCase(pc, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/chat/presence", h), "/chat/presence", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(pc.SendChatPresenceCalls)}
				if out.portCalls > 0 {
					out.portTxtID = pc.SendChatPresenceCalls[0].TxtID
				}
				return out
			},
		},
		{
			nome:        "MarkRead",
			rota:        "/chat/markread",
			validBody:   `{"Id":["MSG1"],"ChatPhone":"5511999999999"}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				cm := &contractsfake.ChatMessenger{}
				h := NewMarkReadHandler(message.NewMarkReadUseCase(cm, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/chat/markread", h), "/chat/markread", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(cm.MarkReadCalls)}
				if out.portCalls > 0 {
					out.portTxtID = cm.MarkReadCalls[0].TxtID
				}
				return out
			},
		},
		{
			nome:        "React",
			rota:        "/chat/react",
			validBody:   `{"Phone":"5511999999999","Body":"ok","Id":"MSG1"}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				cm := &contractsfake.ChatMessenger{}
				h := NewReactHandler(message.NewReactUseCase(cm, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/chat/react", h), "/chat/react", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(cm.SendReactionCalls)}
				if out.portCalls > 0 {
					out.portTxtID = cm.SendReactionCalls[0].TxtID
				}
				return out
			},
		},
		{
			// A rota e' "/call/reject" (wiring_routes.go:158), e nao o
			// "/chat/rejectcall" que handler_misc_test.go pede — e que so'
			// passa la' porque o teste chama o handler cru.
			nome:        "RejectCall",
			rota:        "/call/reject",
			validBody:   `{"call_from":"5511999999999@s.whatsapp.net","call_id":"CALL1"}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				ops := &contractsfake.ChatOperations{}
				h := NewRejectCallHandler(chat.NewRejectCallUseCase(ops, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/call/reject", h), "/call/reject", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(ops.RejectCallCalls)}
				if out.portCalls > 0 {
					out.portTxtID = ops.RejectCallCalls[0].TxtID
				}
				return out
			},
		},
		{
			// Idem: a rota real e' "/chat/request-unavailable-message".
			nome:        "RequestUnavailableMessage",
			rota:        "/chat/request-unavailable-message",
			validBody:   `{"chat":"5511999999999@s.whatsapp.net","sender":"5511888888888@s.whatsapp.net","id":"MSG1"}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				ops := &contractsfake.ChatOperations{}
				h := NewRequestUnavailableMessageHandler(chat.NewRequestUnavailableMessageUseCase(ops, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/chat/request-unavailable-message", h),
					"/chat/request-unavailable-message", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(ops.RequestUnavailableMessageCalls)}
				if out.portCalls > 0 {
					out.portTxtID = ops.RequestUnavailableMessageCalls[0].TxtID
				}
				return out
			},
		},
		{
			nome:        "MuteChat",
			rota:        "/chat/mute",
			validBody:   `{"jid":"5511999999999@s.whatsapp.net","mute":true}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				ops := &contractsfake.ChatOperations{}
				h := NewMuteChatHandler(chat.NewMuteChatUseCase(ops, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/chat/mute", h), "/chat/mute", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(ops.MuteChatCalls)}
				if out.portCalls > 0 {
					out.portTxtID = ops.MuteChatCalls[0].TxtID
				}
				return out
			},
		},
		{
			nome:        "ArchiveChat",
			rota:        "/chat/archive",
			validBody:   `{"jid":"5511999999999@s.whatsapp.net","archive":true}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				ops := &contractsfake.ChatOperations{}
				h := NewArchiveChatHandler(chat.NewArchiveChatUseCase(ops, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/chat/archive", h), "/chat/archive", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(ops.ArchiveChatCalls)}
				if out.portCalls > 0 {
					out.portTxtID = ops.ArchiveChatCalls[0].TxtID
				}
				return out
			},
		},
		{
			nome:        "StarMessage",
			rota:        "/message/star",
			validBody:   `{"chat":"120363111111111111@g.us","sender":"5511999999999@s.whatsapp.net","message_id":"ABCDE12345","from_me":false,"star":true}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				ms := &contractsfake.MessageStarrer{}
				h := NewStarMessageHandler(chat.NewStarMessageUseCase(ms, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/message/star", h), "/message/star", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(ms.StarMessageCalls)}
				if out.portCalls > 0 {
					out.portTxtID = ms.StarMessageCalls[0].TxtID
				}
				return out
			},
		},
		{
			nome:        "PinChat",
			rota:        "/chat/pin",
			validBody:   `{"jid":"5511999999999@s.whatsapp.net","pin":true}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				cp := &contractsfake.ChatPinner{}
				h := NewPinChatHandler(chat.NewPinChatUseCase(cp, &contractsfake.JIDResolver{}, silentLogger{}))
				rec, recs := nonSendAxisServe(t, nonSendAxisRouter("/chat/pin", h), "/chat/pin", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(cp.PinChatCalls)}
				if out.portCalls > 0 {
					out.portTxtID = cp.PinChatCalls[0].TxtID
				}
				return out
			},
		},
		{
			// POST /user/privacy chega pelo mesmo HandlerFunc que
			// wiring_routes.go:161-167 registra para GET e POST; o
			// roteador do caso reproduz esse desvio por metodo em vez de
			// registrar o handler de POST diretamente.
			nome:        "SetPrivacySetting",
			rota:        "/user/privacy",
			validBody:   `{"privacy_setting":"groupadd","value":"contacts"}`,
			decodeCause: nonSendAxisDecodeCauseSentinel,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) nonSendAxisOutcome {
				pm := &contractsfake.PrivacyManager{}
				get := NewGetPrivacySettingsHandler(user.NewGetPrivacySettingsUseCase(pm, silentLogger{}))
				set := NewSetPrivacySettingHandler(user.NewSetPrivacySettingUseCase(pm, silentLogger{}))
				byMethod := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						get.ServeHTTP(w, r)
						return
					}
					set.ServeHTTP(w, r)
				})
				r := mux.NewRouter()
				r.Handle("/user/privacy", byMethod).Methods(http.MethodGet, http.MethodPost)

				rec, recs := nonSendAxisServe(t, r, "/user/privacy", body, mut)
				out := nonSendAxisOutcome{rec: rec, recs: recs, portCalls: len(pm.SetPrivacySettingCalls)}
				if out.portCalls > 0 {
					out.portTxtID = pm.SetPrivacySettingCalls[0].TxtID
				}
				return out
			},
		},
		nonSendAxisDownloadCase("DownloadImage", "/chat/downloadimage", "image/jpeg",
			func(md appport.MediaDownloader, l appport.Logger) http.Handler {
				return NewDownloadImageHandler(message.NewDownloadImageUseCase(md, l))
			}),
		nonSendAxisDownloadCase("DownloadVideo", "/chat/downloadvideo", "video/mp4",
			func(md appport.MediaDownloader, l appport.Logger) http.Handler {
				return NewDownloadVideoHandler(message.NewDownloadVideoUseCase(md, l))
			}),
		nonSendAxisDownloadCase("DownloadAudio", "/chat/downloadaudio", "audio/ogg",
			func(md appport.MediaDownloader, l appport.Logger) http.Handler {
				return NewDownloadAudioHandler(message.NewDownloadAudioUseCase(md, l))
			}),
		nonSendAxisDownloadCase("DownloadDocument", "/chat/downloaddocument", "application/pdf",
			func(md appport.MediaDownloader, l appport.Logger) http.Handler {
				return NewDownloadDocumentHandler(message.NewDownloadDocumentUseCase(md, l))
			}),
		nonSendAxisDownloadCase("DownloadSticker", "/chat/downloadsticker", "image/webp",
			func(md appport.MediaDownloader, l appport.Logger) http.Handler {
				return NewDownloadStickerHandler(message.NewDownloadStickerUseCase(md, l))
			}),
	}
}

// nonSendAxisCasesChecked devolve a tabela depois de verificar que ela ainda
// cobre as DEZASSETE capabilities non-send, na ordem, por NOME. Uma capability
// removida em silencio levaria os cinco eixos junto, sem nenhuma falha.
func nonSendAxisCasesChecked(t *testing.T) []nonSendAxisCase {
	t.Helper()
	casos := nonSendAxisCases()
	want := []string{
		"SendPresence", "SubscribePresence", "ChatPresence", "MarkRead", "React",
		"RejectCall", "RequestUnavailableMessage", "MuteChat", "ArchiveChat", "StarMessage",
		"PinChat", "SetPrivacySetting",
		"DownloadImage", "DownloadVideo", "DownloadAudio", "DownloadDocument", "DownloadSticker",
	}
	if len(casos) != len(want) {
		t.Fatalf("a tabela cobre %d capabilities, quero as %d do CAP-19 (%s)",
			len(casos), len(want), strings.Join(want, ", "))
	}
	for i, w := range want {
		if casos[i].nome != w {
			t.Fatalf("a tabela na posicao %d e' %q, quero %q", i, casos[i].nome, w)
		}
	}
	return casos
}

// nonSendAxisNoSessionID e' a requisicao AUTENTICADA cujo `Id` e' vazio:
// userinfo esta' no contexto, entao a recusa tem de ser 400 do cliente, e
// nao 401.
func nonSendAxisNoSessionID(r *http.Request) *http.Request { return withUser(r, "") }

// nonSendAxisWrongType poe no contexto um valor que NAO satisfaz userInfo. A
// chave e' tipada mas o valor e' `any`: sem o par assercao-de-tipo + `nil`
// que sessionUser faz, isto e' panico em producao.
func nonSendAxisWrongType(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
}

// TestNonSendCapabilities_MissingSessionID_ViaRegisteredRoute — eixo 1.
//
// O corpo e' VALIDO de proposito: se a guarda de sessao nao disparar, nada
// mais impede a acao e a porta e' alcancada. A CAUSA e' asseverada porque o
// envelope de erro deste repo e' o generico — sem o registro de saida, este
// 400 e' indistinguivel do 400 por corpo malformado.
func TestNonSendCapabilities_MissingSessionID_ViaRegisteredRoute(t *testing.T) {
	for _, caso := range nonSendAxisCasesChecked(t) {
		t.Run(caso.nome, func(t *testing.T) {
			out := caso.serve(t, caso.validBody, nonSendAxisNoSessionID)

			assertErrorEnvelope(t, out.rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, out.recs, "missing session id")
			if out.portCalls != 0 {
				t.Fatalf("%s: requisicao sem session id alcancou a porta %d vez(es)", caso.rota, out.portCalls)
			}
		})
	}
}

// TestNonSendCapabilities_MalformedBody_ViaRegisteredRoute — eixo 2.
//
// A requisicao e' AUTENTICADA de proposito: sem isso o 401 mascara o 400 e o
// teste nao mede o decode. A CAUSA distingue o decode da rejeicao do use
// case — o corpo truncado tambem produziria 400 por campo obrigatorio
// ausente se o decode fosse ignorado.
func TestNonSendCapabilities_MalformedBody_ViaRegisteredRoute(t *testing.T) {
	for _, caso := range nonSendAxisCasesChecked(t) {
		t.Run(caso.nome, func(t *testing.T) {
			out := caso.serve(t, nonSendAxisMalformedBody, msgAuthed)

			assertErrorEnvelope(t, out.rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, out.recs, caso.decodeCause)
			if out.portCalls != 0 {
				t.Fatalf("%s: corpo malformado alcancou a porta %d vez(es)", caso.rota, out.portCalls)
			}
		})
	}
}

// TestNonSendCapabilities_WrongTypeInContext_ViaRegisteredRoute — eixo 4.
func TestNonSendCapabilities_WrongTypeInContext_ViaRegisteredRoute(t *testing.T) {
	for _, caso := range nonSendAxisCasesChecked(t) {
		t.Run(caso.nome, func(t *testing.T) {
			out := caso.serve(t, caso.validBody, nonSendAxisWrongType)

			assertErrorEnvelope(t, out.rec, http.StatusUnauthorized)
			logassert.OutcomeLogged(t, out.recs, "unauthorized")
			if out.portCalls != 0 {
				t.Fatalf("%s: contexto com tipo errado alcancou a porta %d vez(es)", caso.rota, out.portCalls)
			}
		})
	}
}

// TestNonSendCapabilities_SuccessEmitsNoOutcomeLog — eixos 3 e 5.
//
// O eixo 3 e' o que os outros nao pegam: um handler que logasse TODO request
// em warn passaria em cada assercao de caminho de erro deste arquivo e ainda
// seria o ruido que a Fase 12 existe para evitar. A assercao e'
// `assertNoOutcomeLog` (logassert_test.go), que compara por NIVEL — a forma
// fraca `has("error")` deixaria passar um Warn de ruido, que nao traz campo
// "error" nenhum (licao do FIX-10 e da F143).
//
// O eixo 5 vive no mesmo subteste porque so' o caminho feliz alcanca a porta:
// o txtID que a porta recebe tem de ser o do CONTEXTO autenticado, e nao um
// campo do payload nem um valor inventado pelo handler (F142). O contador em
// 1 tambem trava o valor OPOSTO ao dos outros eixos — sem ele as assercoes de
// "nao alcancou" seriam vacuas, porque um contador que nunca se move passa em
// todas elas.
func TestNonSendCapabilities_SuccessEmitsNoOutcomeLog(t *testing.T) {
	for _, caso := range nonSendAxisCasesChecked(t) {
		t.Run(caso.nome, func(t *testing.T) {
			out := caso.serve(t, caso.validBody, msgAuthed)

			if out.rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, quero 200 (corpo: %s)", caso.rota, out.rec.Code, out.rec.Body.String())
			}
			if out.portCalls != 1 {
				t.Fatalf("%s: a porta foi chamada %d vez(es), quero 1", caso.rota, out.portCalls)
			}
			if out.portTxtID != "user-1" {
				t.Fatalf("%s: a porta recebeu txtID %q, quero %q — o Id do contexto autenticado",
					caso.rota, out.portTxtID, "user-1")
			}
			assertNoOutcomeLog(t, out.recs)
		})
	}
}
