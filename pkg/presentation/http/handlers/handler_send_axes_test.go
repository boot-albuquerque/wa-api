package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
)

// CAP-18 — os QUATRO eixos de fronteira nas SEIS capabilities de envio que
// ficaram sem nenhum deles.
//
// Os eixos sao os quatro que a tabela de handler_interactive_test.go cobria
// antes da migracao do CAP-08A e que so' foram recuperados nas capabilities
// migradas DEPOIS da auditoria de conservacao (location, contact, poll,
// template — CAP-08 em diante):
//
//	1. missing session id     — autenticado, `Id` vazio => 400, porta intacta
//	2. corpo malformado       — JSON truncado => 400, porta intacta
//	3. sucesso nao loga       — caminho feliz sem registro warn/error
//	4. wrong type in context  — valor que nao satisfaz userInfo => 401, sem panico
//
// As seis deste arquivo — text, image, audio, video, document, sticker —
// vieram do CAP-01 ao CAP-07, antes da auditoria existir, e nasceram sem os
// quatro. handler_boundary_test.go cobre text nos eixos 1, 2 e 4, mas por
// `handler.ServeHTTP` CRU, sem rota registrada e sem assercao de CAUSA: um
// 400 por decode e um 400 por sessao ausente sao indistinguiveis la'. As
// outras cinco nao tinham nada.
//
// Por que UMA tabela e nao 24 copias: as seis divergem apenas em QUAIS dubles
// montam o roteador, em QUAL slice de chamadas conta o envio, e no texto da
// causa do decode. Cada closure resolve isso dentro de si e devolve a mesma
// forma; as quatro assercoes, que sao o que este arquivo mede, sao escritas
// uma vez cada. A DUPLICACAO foi a causa raiz da F143 e nao se repete aqui.

// sendAxisOutcome e' o resultado de UMA requisicao pela rota registrada, com
// tudo que os quatro eixos precisam observar.
type sendAxisOutcome struct {
	rec *httptest.ResponseRecorder
	// recs e' a saida da mesma cadeia hlog que router.go instala.
	recs []logLine
	// portCalls conta as chamadas do metodo de envio da capability.
	portCalls int
	// fetchCalls conta as chamadas de MediaFetcher.FetchBytes. Sempre 0 nas
	// capabilities que nao buscam bytes.
	fetchCalls int
}

// sendAxisCase e' UMA capability de envio vista pelos quatro eixos.
type sendAxisCase struct {
	// nome e' o rotulo do subteste, sem barra, para que `go test -run` possa
	// isolar UMA capability — que e' o que o controle negativo precisa fazer.
	nome string
	rota string
	// validBody e' o corpo que produz 200 no caminho feliz.
	validBody string
	// decodeCause e' a substring que o campo `error` do registro tem de
	// carregar quando o corpo e' malformado.
	//
	// Ela DIVERGE entre familias de handler, e a divergencia e' medida, nao
	// suposta: handler_interactive.go:92 (location, contact, poll) loga o
	// erro-sentinela `errDecodePayload` ("could not decode payload"), enquanto
	// handler_media.go, handler_media_ext.go e handler_message_send.go logam
	// o erro CRU do json.Decoder — que para o corpo truncado deste arquivo e'
	// "unexpected EOF". Uniformizar a assercao esconderia essa diferenca; o
	// campo a torna visivel.
	decodeCause string
	// fetchesMedia diz se a capability busca bytes antes de enviar. Quando
	// true, o eixo 2 exige fetchCalls==0 (um handler que baixasse o arquivo
	// antes de validar o corpo passaria sem isso) e o eixo 3 exige
	// fetchCalls==1 — sem a segunda metade a primeira seria vacua.
	fetchesMedia bool
	// serve faz UM POST pela rota REGISTRADA com a mutacao dada, sob captura
	// de log, e devolve o que os eixos observam.
	serve func(t *testing.T, body string, mut func(*http.Request) *http.Request) sendAxisOutcome
}

// sendAxisServe e' o unico ponto do arquivo que monta requisicao: envolve o
// roteador na cadeia hlog de producao, faz o POST e devolve resposta e log.
func sendAxisServe(t *testing.T, h http.Handler, target, body string, mut func(*http.Request) *http.Request) (*httptest.ResponseRecorder, []logLine) {
	t.Helper()
	wrapped, capture := logassert.Wrap(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	wrapped.ServeHTTP(rec, mut(req))
	return rec, capture.Records(t)
}

const sendAxisMalformedBody = `{"phone":"5511`

// sendAxisDecodeCauseRaw e' a causa das capabilities que logam o erro cru do
// json.Decoder para o corpo truncado acima.
const sendAxisDecodeCauseRaw = "unexpected EOF"

func sendAxisCases() []sendAxisCase {
	return []sendAxisCase{
		{
			nome:        "text",
			rota:        "POST /chat/send/text",
			validBody:   `{"phone":"5511999999999","body":"ola"}`,
			decodeCause: sendAxisDecodeCauseRaw,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) sendAxisOutcome {
				tm := &contractsfake.TextMessenger{}
				rec, recs := sendAxisServe(t, sendTextRouter(tm, &contractsfake.JIDResolver{}),
					"/chat/send/text", body, mut)
				return sendAxisOutcome{rec: rec, recs: recs, portCalls: len(tm.SendTextCalls)}
			},
		},
		{
			nome:         "image",
			rota:         "POST /chat/send/image",
			validBody:    `{"phone":"5511999999999","image":"` + sendImageTestURL + `","caption":"legenda"}`,
			decodeCause:  sendAxisDecodeCauseRaw,
			fetchesMedia: true,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) sendAxisOutcome {
				mm := &contractsfake.MediaMessenger{}
				mf := defaultSendImageFetcher()
				rec, recs := sendAxisServe(t, sendImageRouter(mm, &contractsfake.JIDResolver{}, mf),
					"/chat/send/image", body, mut)
				return sendAxisOutcome{rec: rec, recs: recs,
					portCalls: len(mm.SendImageCalls), fetchCalls: len(mf.FetchBytesCalls)}
			},
		},
		{
			nome:         "audio",
			rota:         "POST /chat/send/audio",
			validBody:    `{"phone":"5511999999999","audio":"` + sendAudioTestURL + `"}`,
			decodeCause:  sendAxisDecodeCauseRaw,
			fetchesMedia: true,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) sendAxisOutcome {
				mm := &contractsfake.MediaMessenger{}
				mf := defaultSendAudioFetcher()
				rec, recs := sendAxisServe(t, sendAudioRouter(mm, &contractsfake.JIDResolver{}, mf),
					"/chat/send/audio", body, mut)
				return sendAxisOutcome{rec: rec, recs: recs,
					portCalls: len(mm.SendAudioCalls), fetchCalls: len(mf.FetchBytesCalls)}
			},
		},
		{
			nome:         "video",
			rota:         "POST /chat/send/video",
			validBody:    `{"phone":"5511999999999","video":"` + sendVideoTestURL + `"}`,
			decodeCause:  sendAxisDecodeCauseRaw,
			fetchesMedia: true,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) sendAxisOutcome {
				mm := &contractsfake.MediaMessenger{}
				mf := defaultSendVideoFetcher()
				rec, recs := sendAxisServe(t, sendVideoRouter(mm, &contractsfake.JIDResolver{}, mf),
					"/chat/send/video", body, mut)
				return sendAxisOutcome{rec: rec, recs: recs,
					portCalls: len(mm.SendVideoCalls), fetchCalls: len(mf.FetchBytesCalls)}
			},
		},
		{
			nome:         "document",
			rota:         "POST /chat/send/document",
			validBody:    `{"phone":"5511999999999","document":"` + sendDocumentTestURL + `","file_name":"relatorio.pdf"}`,
			decodeCause:  sendAxisDecodeCauseRaw,
			fetchesMedia: true,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) sendAxisOutcome {
				mm := &contractsfake.MediaMessenger{}
				mf := defaultSendDocumentFetcher()
				rec, recs := sendAxisServe(t, sendDocumentRouter(mm, &contractsfake.JIDResolver{}, mf),
					"/chat/send/document", body, mut)
				return sendAxisOutcome{rec: rec, recs: recs,
					portCalls: len(mm.SendDocumentCalls), fetchCalls: len(mf.FetchBytesCalls)}
			},
		},
		{
			nome:         "sticker",
			rota:         "POST /chat/send/sticker",
			validBody:    `{"phone":"5511999999999","sticker":"` + sendStickerTestURL + `"}`,
			decodeCause:  sendAxisDecodeCauseRaw,
			fetchesMedia: true,
			serve: func(t *testing.T, body string, mut func(*http.Request) *http.Request) sendAxisOutcome {
				mm := &contractsfake.MediaMessenger{}
				mf := defaultSendStickerFetcher()
				rec, recs := sendAxisServe(t, sendStickerRouter(mm, &contractsfake.JIDResolver{}, mf, defaultSendStickerProcessor()),
					"/chat/send/sticker", body, mut)
				return sendAxisOutcome{rec: rec, recs: recs,
					portCalls: len(mm.SendStickerCalls), fetchCalls: len(mf.FetchBytesCalls)}
			},
		},
	}
}

// sendAxisCasesChecked devolve a tabela depois de verificar que ela ainda
// cobre as SEIS capabilities do CAP-18. Uma capability removida em silencio
// levaria os quatro eixos junto, sem nenhuma falha.
func sendAxisCasesChecked(t *testing.T) []sendAxisCase {
	t.Helper()
	casos := sendAxisCases()
	want := []string{"text", "image", "audio", "video", "document", "sticker"}
	if len(casos) != len(want) {
		t.Fatalf("a tabela cobre %d capabilities, quero as %d do CAP-18 (%s)",
			len(casos), len(want), strings.Join(want, ", "))
	}
	for i, w := range want {
		if casos[i].nome != w {
			t.Fatalf("a tabela na posicao %d e' %q, quero %q", i, casos[i].nome, w)
		}
	}
	return casos
}

// sendAxisNoSessionID e' a requisicao AUTENTICADA cujo `Id` e' vazio: userinfo
// esta' no contexto, entao a recusa tem de ser 400 do cliente e nao 401.
func sendAxisNoSessionID(r *http.Request) *http.Request { return withUser(r, "") }

// sendAxisWrongType poe no contexto um valor que NAO satisfaz userInfo. A
// chave e' tipada mas o valor e' `any`: a assercao de tipo do handler tem de
// ser a de duas variaveis (`info, ok := ...`), ou isto e' panico em producao.
func sendAxisWrongType(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), appport.UserInfoKey, 42))
}

// TestSendCapabilities_MissingSessionID_ViaRegisteredRoute — eixo 1.
//
// O corpo e' VALIDO de proposito: se a guarda de sessao nao disparar, nada
// mais impede o envio e a porta e' alcancada. A CAUSA e' asseverada porque o
// envelope de erro deste repo e' o generico "bad request" — sem o registro de
// saida, este 400 e' indistinguivel do 400 por corpo malformado.
func TestSendCapabilities_MissingSessionID_ViaRegisteredRoute(t *testing.T) {
	for _, caso := range sendAxisCasesChecked(t) {
		t.Run(caso.nome, func(t *testing.T) {
			out := caso.serve(t, caso.validBody, sendAxisNoSessionID)

			assertErrorEnvelope(t, out.rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, out.recs, "missing session id")
			if out.portCalls != 0 {
				t.Fatalf("%s: requisicao sem session id alcancou a porta %d vez(es)", caso.rota, out.portCalls)
			}
			if caso.fetchesMedia && out.fetchCalls != 0 {
				t.Fatalf("%s: requisicao sem session id buscou midia %d vez(es)", caso.rota, out.fetchCalls)
			}
		})
	}
}

// TestSendCapabilities_MalformedBody_ViaRegisteredRoute — eixo 2.
//
// A requisicao e' AUTENTICADA de proposito: sem isso o 401 mascara o 400 e o
// teste nao mede o decode. A CAUSA distingue o decode da rejeicao do use case
// — o corpo truncado tambem produziria 400 por "missing Phone in payload" se
// o decode fosse ignorado.
//
// Nas capabilities de MIDIA a assercao tem uma segunda metade: a busca de
// bytes tambem nao pode ter acontecido. Um handler que baixasse o arquivo
// antes de validar o corpo passaria em tudo o mais deste arquivo.
func TestSendCapabilities_MalformedBody_ViaRegisteredRoute(t *testing.T) {
	for _, caso := range sendAxisCasesChecked(t) {
		t.Run(caso.nome, func(t *testing.T) {
			out := caso.serve(t, sendAxisMalformedBody, msgAuthed)

			assertErrorEnvelope(t, out.rec, http.StatusBadRequest)
			logassert.OutcomeLogged(t, out.recs, caso.decodeCause)
			if out.portCalls != 0 {
				t.Fatalf("%s: corpo malformado alcancou a porta %d vez(es)", caso.rota, out.portCalls)
			}
			if caso.fetchesMedia && out.fetchCalls != 0 {
				t.Fatalf("%s: corpo malformado, mas a midia foi buscada %d vez(es) — "+
					"o handler baixou o arquivo antes de validar o corpo", caso.rota, out.fetchCalls)
			}
		})
	}
}

// TestSendCapabilities_WrongTypeInContext_ViaRegisteredRoute — eixo 4.
func TestSendCapabilities_WrongTypeInContext_ViaRegisteredRoute(t *testing.T) {
	for _, caso := range sendAxisCasesChecked(t) {
		t.Run(caso.nome, func(t *testing.T) {
			out := caso.serve(t, caso.validBody, sendAxisWrongType)

			assertErrorEnvelope(t, out.rec, http.StatusUnauthorized)
			logassert.OutcomeLogged(t, out.recs, "unauthorized")
			if out.portCalls != 0 {
				t.Fatalf("%s: contexto com tipo errado alcancou a porta %d vez(es)", caso.rota, out.portCalls)
			}
			if caso.fetchesMedia && out.fetchCalls != 0 {
				t.Fatalf("%s: contexto com tipo errado buscou midia %d vez(es)", caso.rota, out.fetchCalls)
			}
		})
	}
}

// TestSendCapabilities_SuccessEmitsNoOutcomeLog — eixo 3.
//
// E' o eixo que os outros tres nao pegam: um handler que logasse TODO request
// em warn passaria em cada assercao de caminho de erro deste arquivo e ainda
// assim seria o ruido que a Fase 12 existe para evitar. O logger do use case
// e' silentLogger{}, entao o que se mede aqui e' so' o registro do HANDLER.
//
// A assercao e' `assertNoOutcomeLog` (logassert_test.go), que compara por
// NIVEL. A forma fraca `has("error")` deixaria passar um Warn de ruido, que
// nao traz campo "error" nenhum — foi a licao do FIX-10 e da F143.
//
// O caminho feliz tambem trava os contadores no valor OPOSTO ao dos outros
// tres eixos: envio uma vez, e busca de midia uma vez onde ha' midia. Sem
// isso, as assercoes de "nao alcancou" seriam vacuas — um contador que nunca
// se move passa em todas elas.
func TestSendCapabilities_SuccessEmitsNoOutcomeLog(t *testing.T) {
	for _, caso := range sendAxisCasesChecked(t) {
		t.Run(caso.nome, func(t *testing.T) {
			out := caso.serve(t, caso.validBody, msgAuthed)

			if out.rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, quero 200 (corpo: %s)", caso.rota, out.rec.Code, out.rec.Body.String())
			}
			if out.portCalls != 1 {
				t.Fatalf("%s: a porta foi chamada %d vez(es), quero 1", caso.rota, out.portCalls)
			}
			if caso.fetchesMedia && out.fetchCalls != 1 {
				t.Fatalf("%s: a midia foi buscada %d vez(es), quero 1", caso.rota, out.fetchCalls)
			}
			assertNoOutcomeLog(t, out.recs)
		})
	}
}
