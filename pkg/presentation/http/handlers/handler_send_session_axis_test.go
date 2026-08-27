package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
)

// CAP-16 — O EIXO DO txtID nas DEZ capabilities de envio que ficaram sem ele.
//
// O txtID e' a identidade da SESSAO. Se o handler entregar a' porta um valor
// que nao seja o do contexto autenticado, a mensagem sai pela CONTA ERRADA —
// mesma familia da F125 (vazamento entre tenants do ramo `index`), nao higiene
// de teste.
//
// A EVAL-15 mediu o buraco em send/template: trocar `txtID` por uma constante
// no handler deixava `./pkg/...` INTEIRO verde. O CAP-15 fechou template
// (TestSendTemplate_AuthenticatedSessionReachesPort); send/buttons e send/list
// ja' tinham o eixo pela tabela de handler_message_test.go — e quando cada um
// saiu dessa tabela (send/buttons no CAP-21, send/list no CAP-22), o eixo foi
// junto: TestSendButtons_AuthenticatedSessionReachesPort e
// TestSendList_AuthenticatedSessionReachesPort. As DEZ que sobravam sao as
// deste arquivo:
//
//	text, image, audio, video, document, sticker, location, contact, poll, pollvote
//
// DOIS pontos por capability, nao um: a guarda de sessao (EnsureSession) e o
// metodo de envio (SendText, SendImage, ...) sao chamadas DISTINTAS da porta e
// um defeito pode atingir so' uma. Travar so' um lado deixa o outro livre.
//
// O id de cada caso e' um sentinela DIFERENTE do "user-1" que msgAuthed
// injeta, e diferente entre capabilities. Com "user-1" um handler que
// ignorasse o contexto e usasse uma constante passaria por coincidencia; aqui
// so' passa quem le' o contexto autenticado da propria requisicao.
//
// Por que UMA tabela e nao nove copias: as nove divergem apenas em QUAIS
// dubles montam o roteador e em QUAL slice de chamadas guarda o TxtID. Cada
// closure resolve essas duas divergencias dentro de si e devolve a mesma
// forma — dois slices de string —, entao a assercao, que e' o que este arquivo
// mede, e' escrita uma vez so'. Os roteadores e os corpos sao os ja'
// existentes de cada arquivo de teste, nao replicas.

// sessionAxisCase e' UMA capability de envio vista pelo eixo do txtID.
type sessionAxisCase struct {
	// nome e' o rotulo do subteste. Sem barra, para que `go test -run` possa
	// isolar UMA capability — que e' o que o controle negativo precisa fazer,
	// nove vezes.
	nome string
	rota string
	// sessionID e' o sentinela desta capability. Distinto de "user-1".
	sessionID string
	// serve monta o roteador da capability com dubles proprios, faz o POST
	// autenticado com sessionID no contexto, e devolve a resposta junto com os
	// txtIDs que chegaram aos DOIS pontos da porta.
	serve func(t *testing.T, sessionID string) (rec *httptest.ResponseRecorder, ensure []string, send []string)
}

// sessionAxisEnsureTxtIDs extrai os txtIDs vistos pela guarda de sessao.
func sessionAxisEnsureTxtIDs(g contractsfake.SessionGuard) []string {
	out := make([]string, 0, len(g.EnsureSessionCalls))
	for _, c := range g.EnsureSessionCalls {
		out = append(out, c.TxtID)
	}
	return out
}

func sessionAxisCases() []sessionAxisCase {
	return []sessionAxisCase{
		{
			nome:      "text",
			rota:      "POST /chat/send/text",
			sessionID: "send-text-session-4c81de",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				tm := &contractsfake.TextMessenger{}
				rec := sessionAxisPost(t, sendTextRouter(tm, &contractsfake.JIDResolver{}),
					"/chat/send/text", `{"phone":"5511999999999","body":"ola"}`, sessionID)
				send := make([]string, 0, len(tm.SendTextCalls))
				for _, c := range tm.SendTextCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(tm.SessionGuard), send
			},
		},
		{
			nome:      "image",
			rota:      "POST /chat/send/image",
			sessionID: "send-image-session-9a27bf",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				mm := &contractsfake.MediaMessenger{}
				rec := sessionAxisPost(t, sendImageRouter(mm, &contractsfake.JIDResolver{}, defaultSendImageFetcher()),
					"/chat/send/image", `{"phone":"5511999999999","image":"`+sendImageTestURL+`","caption":"legenda"}`, sessionID)
				send := make([]string, 0, len(mm.SendImageCalls))
				for _, c := range mm.SendImageCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(mm.SessionGuard), send
			},
		},
		{
			nome:      "audio",
			rota:      "POST /chat/send/audio",
			sessionID: "send-audio-session-1f5c30",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				mm := &contractsfake.MediaMessenger{}
				rec := sessionAxisPost(t, sendAudioRouter(mm, &contractsfake.JIDResolver{}, defaultSendAudioFetcher()),
					"/chat/send/audio", `{"phone":"5511999999999","audio":"`+sendAudioTestURL+`"}`, sessionID)
				send := make([]string, 0, len(mm.SendAudioCalls))
				for _, c := range mm.SendAudioCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(mm.SessionGuard), send
			},
		},
		{
			nome:      "video",
			rota:      "POST /chat/send/video",
			sessionID: "send-video-session-6d3b92",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				mm := &contractsfake.MediaMessenger{}
				rec := sessionAxisPost(t, sendVideoRouter(mm, &contractsfake.JIDResolver{}, defaultSendVideoFetcher()),
					"/chat/send/video", `{"phone":"5511999999999","video":"`+sendVideoTestURL+`"}`, sessionID)
				send := make([]string, 0, len(mm.SendVideoCalls))
				for _, c := range mm.SendVideoCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(mm.SessionGuard), send
			},
		},
		{
			nome:      "document",
			rota:      "POST /chat/send/document",
			sessionID: "send-document-session-2e74ac",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				mm := &contractsfake.MediaMessenger{}
				rec := sessionAxisPost(t, sendDocumentRouter(mm, &contractsfake.JIDResolver{}, defaultSendDocumentFetcher()),
					"/chat/send/document",
					`{"phone":"5511999999999","document":"`+sendDocumentTestURL+`","file_name":"relatorio.pdf"}`, sessionID)
				send := make([]string, 0, len(mm.SendDocumentCalls))
				for _, c := range mm.SendDocumentCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(mm.SessionGuard), send
			},
		},
		{
			nome:      "sticker",
			rota:      "POST /chat/send/sticker",
			sessionID: "send-sticker-session-8b0f65",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				mm := &contractsfake.MediaMessenger{}
				rec := sessionAxisPost(t, sendStickerRouter(mm, &contractsfake.JIDResolver{},
					defaultSendStickerFetcher(), defaultSendStickerProcessor()),
					"/chat/send/sticker", `{"phone":"5511999999999","sticker":"`+sendStickerTestURL+`"}`, sessionID)
				send := make([]string, 0, len(mm.SendStickerCalls))
				for _, c := range mm.SendStickerCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(mm.SessionGuard), send
			},
		},
		{
			nome:      "location",
			rota:      "POST /chat/send/location",
			sessionID: "send-location-session-5c9e18",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				sm := &contractsfake.SimpleMessenger{}
				rec := sessionAxisPost(t, sendLocationRouter(sm, &contractsfake.JIDResolver{}),
					"/chat/send/location",
					`{"phone":"5511999999999","name":"Praca da Se","latitude":-23.5505,"longitude":-46.6333}`, sessionID)
				send := make([]string, 0, len(sm.SendLocationCalls))
				for _, c := range sm.SendLocationCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(sm.SessionGuard), send
			},
		},
		{
			nome:      "contact",
			rota:      "POST /chat/send/contact",
			sessionID: "send-contact-session-3a6d47",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				sm := &contractsfake.SimpleMessenger{}
				rec := sessionAxisPost(t, sendContactRouter(sm, &contractsfake.JIDResolver{}),
					"/chat/send/contact",
					`{"phone":"5511999999999","name":"Alice","vcard":"BEGIN:VCARD\nVERSION:3.0\nFN:Alice\nEND:VCARD"}`, sessionID)
				send := make([]string, 0, len(sm.SendContactCalls))
				for _, c := range sm.SendContactCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(sm.SessionGuard), send
			},
		},
		{
			nome:      "poll",
			rota:      "POST /chat/send/poll",
			sessionID: "send-poll-session-7e2140",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				sm := &contractsfake.SimpleMessenger{}
				rec := sessionAxisPost(t, sendPollRouter(sm, &contractsfake.JIDResolver{}),
					"/chat/send/poll",
					`{"group":"120363313346913103@g.us","header":"Que horas almocamos?","options":["12h","13h"]}`, sessionID)
				send := make([]string, 0, len(sm.SendPollCalls))
				for _, c := range sm.SendPollCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(sm.SessionGuard), send
			},
		},
		{
			nome:      "pollvote",
			rota:      "POST /chat/send/pollvote",
			sessionID: "send-pollvote-session-c4a829",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				cm := &contractsfake.ChatMessenger{}
				rec := sessionAxisPost(t, sendPollVoteRouter(cm, &contractsfake.JIDResolver{}),
					"/chat/send/pollvote",
					`{"phone":"120363313346913103@g.us","sender":"5511999999999@s.whatsapp.net","poll_message_id":"3EB0POLL1","poll_message_timestamp":1755500100,"options":["12h"]}`, sessionID)
				send := make([]string, 0, len(cm.SendPollVoteCalls))
				for _, c := range cm.SendPollVoteCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(cm.SessionGuard), send
			},
		},
		{
			nome:      "forward",
			rota:      "POST /chat/send/forward",
			sessionID: "send-forward-session-d7f192",
			serve: func(t *testing.T, sessionID string) (*httptest.ResponseRecorder, []string, []string) {
				tm := &contractsfake.TextMessenger{}
				rec := sessionAxisPost(t, sendForwardRouter(tm, &contractsfake.JIDResolver{}),
					"/chat/send/forward",
					`{"phone":"5511999999999","body":"forwarded text"}`, sessionID)
				send := make([]string, 0, len(tm.SendTextCalls))
				for _, c := range tm.SendTextCalls {
					send = append(send, c.TxtID)
				}
				return rec, sessionAxisEnsureTxtIDs(tm.SessionGuard), send
			},
		},
	}
}

// sessionAxisPost faz o POST pela ROTA REGISTRADA, autenticado com sessionID.
// Nao usa msgAuthed de proposito: msgAuthed injeta "user-1", e o eixo so' e'
// medido quando o id do contexto e' um sentinela que nenhum handler teria
// como adivinhar.
func sessionAxisPost(t *testing.T, h http.Handler, target, body, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	h.ServeHTTP(rec, withUser(req, sessionID))
	return rec
}

// TestSendCapabilities_AuthenticatedSessionReachesPort trava, nas ONZE
// capabilities, que o txtID entregue a' porta e' o do contexto autenticado —
// nos DOIS pontos.
func TestSendCapabilities_AuthenticatedSessionReachesPort(t *testing.T) {
	casos := sessionAxisCases()
	if len(casos) != 11 {
		t.Fatalf("a tabela cobre %d capabilities, quero as 11 do CAP-16 + CAP-48 + CAP-49 "+
			"(text, image, audio, video, document, sticker, location, contact, poll, pollvote, forward)", len(casos))
	}

	// Sentinela repetido entre casos passaria por coincidencia num handler que
	// usasse a constante do vizinho. Enumerados aqui para que a proxima
	// capability nao herde um id ja' em uso sem que nada acuse.
	vistos := map[string]string{}
	for _, caso := range casos {
		if caso.sessionID == "user-1" {
			t.Fatalf("%s: o sentinela e' %q, o mesmo que msgAuthed injeta — o caso passaria sem medir o eixo",
				caso.nome, caso.sessionID)
		}
		if outro, ok := vistos[caso.sessionID]; ok {
			t.Fatalf("%s e %s compartilham o sentinela %q; cada capability precisa do seu",
				outro, caso.nome, caso.sessionID)
		}
		vistos[caso.sessionID] = caso.nome
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			rec, ensure, send := caso.serve(t, caso.sessionID)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, quero 200 (corpo: %s)", caso.rota, rec.Code, rec.Body.String())
			}

			if len(ensure) != 1 {
				t.Fatalf("%s: EnsureSession chamado %d vez(es), quero 1", caso.rota, len(ensure))
			}
			if got := ensure[0]; got != caso.sessionID {
				t.Errorf("%s: EnsureSession recebeu txtID %q, quero %q (o do contexto autenticado). "+
					"Um txtID errado aqui valida a sessao ERRADA.", caso.rota, got, caso.sessionID)
			}

			if len(send) != 1 {
				t.Fatalf("%s: o metodo de envio foi chamado %d vez(es), quero 1", caso.rota, len(send))
			}
			if got := send[0]; got != caso.sessionID {
				t.Errorf("%s: o metodo de envio recebeu txtID %q, quero %q (o do contexto autenticado). "+
					"Um txtID errado aqui manda a mensagem pela CONTA ERRADA.", caso.rota, got, caso.sessionID)
			}
		})
	}
}
