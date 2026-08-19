package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
)

// CAP-13 — TRAVA DOS NOMES DO WIRE das capabilities de envio. Nasceu com
// OITO; o CAP-14 acrescentou a nona (/chat/send/poll), o CAP-15 a decima
// (/chat/send/template) e o CAP-21 a DECIMA PRIMEIRA
// (/chat/send/buttons), cada uma no mesmo movimento em que passou a enviar
// de verdade. Capability nova que fica de fora desta trava e' a proxima
// F137.
//
// Por que este arquivo existe, separado dos testes de cada rota: todo o resto
// da suite de envio decodifica `envelope.data` numa struct anonima com as
// MESMAS tags do DTO de producao. Um teste assim SEGUE a tag: renomeie
// `message_id` para `Id` no DTO e no teste e ele continua verde, porque
// encoder e decoder passam a concordar no nome errado. A EVAL-09 provou
// exatamente isso no CAP-09A para /chat/history — trocar as tags pelo
// vocabulario de um tipo orfao deixava a suite INTEIRA passar. Aquele buraco
// foi fechado so' para /chat/history; as oito rotas de envio ficaram sem
// trava nenhuma ate' aqui.
//
// A assercao e' sobre o JSON REAL, decodificado em map[string]any, pela ROTA
// REGISTRADA (gorilla/mux), com as chaves ENUMERADAS uma a uma. Nenhuma lista
// e' derivada da struct por reflexao: derivar da struct e' reintroduzir o
// defeito, porque a lista esperada mudaria junto com a tag.
//
// A forma travada e' a ATUAL — {message_id, timestamp, status} —, que diverge
// do historico ({Details, Timestamp, Id}). Manter a forma atual foi decisao
// registrada (HOUSEKEEP F131), por consistencia entre as oito. Este arquivo
// trava a decisao; nao a reabre.

// sendResultWireKeys sao os nomes das chaves da resposta de envio, escritos a
// mao, um por linha, a partir dos DTOs domain.Send*Result. Identicos nas dez
// capabilities depois que SendStickerResult ganhou Timestamp (F137),
// SendPollResult ganhou Timestamp (CAP-14) e SendTemplateResult ganhou
// Timestamp (CAP-15).
var sendResultWireKeys = []string{
	"message_id",
	"timestamp",
	"status",
}

// sendForeignWireKeys e' o vocabulario que NAO e' desta resposta. `Details`,
// `Id` e `Timestamp` (maiusculo) sao a forma HISTORICA do envio
// (`git show 41bc8e2^:handlers.go`), e portanto a troca mais provavel de quem
// no futuro "restaurar fidelidade" ao historico — foi assim que a F123 nasceu
// em /chat/history. `jid`, `from`, `body`, `direction` e `media_url` sao o
// vocabulario do tipo orfao domain.HistoryMessage, que tambem nunca existiu
// aqui.
var sendForeignWireKeys = []string{
	"Details",
	"Id",
	"Timestamp",
	"jid",
	"from",
	"body",
	"direction",
	"media_url",
}

// sendWireSentAt e' o instante que os dubles devolvem em
// MessageSendResult.Timestamp. Precisa ser NAO-ZERO: `timestamp` carrega
// `omitempty`, e com o valor zero a chave sumiria do JSON e a assercao de
// presenca nao mediria nada — mesmo cuidado de seedHistoryRowAllColumns no
// CAP-09A.
const sendWireSentAt = int64(1755500123)

// sendWireResult e' o resultado que todo duble de porta devolve nesta suite.
func sendWireResult(id string) domain.MessageSendResult {
	return domain.MessageSendResult{ID: id, Timestamp: time.Unix(sendWireSentAt, 0)}
}

// assertSendWireKeys confere, chave por chave, a PRESENCA das esperadas, a
// AUSENCIA das de vocabulario alheio, e denuncia qualquer chave inesperada
// nomeando-a.
func assertSendWireKeys(t *testing.T, rota string, obj map[string]any) {
	t.Helper()

	for _, chave := range sendResultWireKeys {
		if _, ok := obj[chave]; !ok {
			t.Errorf("%s: a chave %q SUMIU do wire. As chaves presentes sao %v.\n"+
				"       Os nomes do wire sao contrato publico; renomear uma tag JSON quebra todo cliente.",
				rota, chave, sendWireChavesOrdenadas(obj))
		}
	}

	for _, proibida := range sendForeignWireKeys {
		if _, ok := obj[proibida]; ok {
			t.Errorf("%s: a chave %q APARECEU no wire. Esse nome e' de outro vocabulario "+
				"(forma historica do envio ou tipo orfao domain.HistoryMessage) e nunca existiu "+
				"nesta resposta.\n       Chaves presentes: %v",
				rota, proibida, sendWireChavesOrdenadas(obj))
		}
	}

	permitidas := map[string]bool{}
	for _, chave := range sendResultWireKeys {
		permitidas[chave] = true
	}
	for chave := range obj {
		if !permitidas[chave] {
			t.Errorf("%s: chave INESPERADA %q no wire. O contrato e' exatamente %v.",
				rota, chave, sendResultWireKeys)
		}
	}
}

func sendWireChavesOrdenadas(obj map[string]any) []string {
	out := make([]string, 0, len(obj))
	for k := range obj {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sendWireCase e' UMA capability de envio: a rota registrada e o serve que a
// exercita com os dubles ja' configurados para devolver ID e Timestamp.
type sendWireCase struct {
	// nome e' o rotulo do subteste. Sem barra de proposito: `/` separa
	// niveis em `go test -run`, e um nome como "POST /chat/send/text"
	// tornaria impossivel rodar UMA capability isolada — que e' exatamente
	// o que o controle negativo precisa fazer, onze vezes.
	nome  string
	rota  string
	serve func(t *testing.T) *httptest.ResponseRecorder
}

// sendWireCases enumera as ONZE capabilities de envio, uma por entrada. Cada
// serve monta o roteador gorilla/mux da propria capability (os helpers
// sendXRouter de cada arquivo de teste) e faz um POST autenticado.
func sendWireCases() []sendWireCase {
	return []sendWireCase{
		{
			nome: "text",
			rota: "POST /chat/send/text",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				tm := &contractsfake.TextMessenger{
					SendTextFunc: func(context.Context, string, domain.JID, string, *domain.LinkPreviewData, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-text-1"), nil
					},
				}
				return sendWirePost(t, sendTextRouter(tm, &contractsfake.JIDResolver{}),
					"/chat/send/text", `{"Phone":"5511999999999","Body":"ola"}`)
			},
		},
		{
			nome: "image",
			rota: "POST /chat/send/image",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				mm := &contractsfake.MediaMessenger{
					SendImageFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-image-1"), nil
					},
				}
				return sendWirePost(t, sendImageRouter(mm, &contractsfake.JIDResolver{}, defaultSendImageFetcher()),
					"/chat/send/image", `{"Phone":"5511999999999","Image":"`+sendImageTestURL+`","Caption":"legenda"}`)
			},
		},
		{
			nome: "audio",
			rota: "POST /chat/send/audio",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				mm := &contractsfake.MediaMessenger{
					SendAudioFunc: func(context.Context, string, domain.JID, domain.AudioPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-audio-1"), nil
					},
				}
				return sendWirePost(t, sendAudioRouter(mm, &contractsfake.JIDResolver{}, defaultSendAudioFetcher()),
					"/chat/send/audio", `{"Phone":"5511999999999","Audio":"`+sendAudioTestURL+`"}`)
			},
		},
		{
			nome: "video",
			rota: "POST /chat/send/video",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				mm := &contractsfake.MediaMessenger{
					SendVideoFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-video-1"), nil
					},
				}
				return sendWirePost(t, sendVideoRouter(mm, &contractsfake.JIDResolver{}, defaultSendVideoFetcher()),
					"/chat/send/video", `{"Phone":"5511999999999","Video":"`+sendVideoTestURL+`"}`)
			},
		},
		{
			nome: "document",
			rota: "POST /chat/send/document",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				mm := &contractsfake.MediaMessenger{
					SendDocumentFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-document-1"), nil
					},
				}
				return sendWirePost(t, sendDocumentRouter(mm, &contractsfake.JIDResolver{}, defaultSendDocumentFetcher()),
					"/chat/send/document",
					`{"Phone":"5511999999999","Document":"`+sendDocumentTestURL+`","FileName":"relatorio.pdf"}`)
			},
		},
		{
			nome: "sticker",
			rota: "POST /chat/send/sticker",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				mm := &contractsfake.MediaMessenger{
					SendStickerFunc: func(context.Context, string, domain.JID, domain.MediaPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-sticker-1"), nil
					},
				}
				return sendWirePost(t, sendStickerRouter(mm, &contractsfake.JIDResolver{},
					defaultSendStickerFetcher(), defaultSendStickerProcessor()),
					"/chat/send/sticker", `{"Phone":"5511999999999","Sticker":"`+sendStickerTestURL+`"}`)
			},
		},
		{
			nome: "location",
			rota: "POST /chat/send/location",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				sm := &contractsfake.SimpleMessenger{
					SendLocationFunc: func(context.Context, string, domain.JID, domain.LocationPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-location-1"), nil
					},
				}
				return sendWirePost(t, sendLocationRouter(sm, &contractsfake.JIDResolver{}),
					"/chat/send/location",
					`{"Phone":"5511999999999","Name":"Praca da Se","Latitude":-23.5505,"Longitude":-46.6333}`)
			},
		},
		{
			nome: "contact",
			rota: "POST /chat/send/contact",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				sm := &contractsfake.SimpleMessenger{
					SendContactFunc: func(context.Context, string, domain.JID, domain.ContactPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-contact-1"), nil
					},
				}
				return sendWirePost(t, sendContactRouter(sm, &contractsfake.JIDResolver{}),
					"/chat/send/contact",
					`{"Phone":"5511999999999","Name":"Alice","Vcard":"BEGIN:VCARD\nVERSION:3.0\nFN:Alice\nEND:VCARD"}`)
			},
		},
		{
			nome: "poll",
			rota: "POST /chat/send/poll",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				sm := &contractsfake.SimpleMessenger{
					SendPollFunc: func(context.Context, string, domain.JID, domain.PollPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-poll-1"), nil
					},
				}
				return sendWirePost(t, sendPollRouter(sm, &contractsfake.JIDResolver{}),
					"/chat/send/poll",
					`{"Group":"120363313346913103@g.us","Header":"Que horas almocamos?","Options":["12h","13h"]}`)
			},
		},
		{
			nome: "template",
			rota: "POST /chat/send/template",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				sm := &contractsfake.SimpleMessenger{
					SendTemplateFunc: func(context.Context, string, domain.JID, domain.TemplatePayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-template-1"), nil
					},
				}
				return sendWirePost(t, sendTemplateRouter(sm, &contractsfake.JIDResolver{}),
					"/chat/send/template",
					`{"Phone":"5511999999999","Content":"Escolha","Footer":"Equipe",`+
						`"Buttons":[{"DisplayText":"Sim","Type":"quickreply"}]}`)
			},
		},
		{
			nome: "buttons",
			rota: "POST /chat/send/buttons",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				im := &contractsfake.InteractiveMessenger{
					SendButtonsFunc: func(context.Context, string, domain.JID, domain.ButtonsPayload, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-buttons-1"), nil
					},
				}
				return sendWirePost(t, sendButtonsRouter(im, &contractsfake.JIDResolver{}, &contractsfake.MediaFetcher{}),
					"/chat/send/buttons",
					`{"Phone":"5511999999999","Body":"Escolha",`+
						`"Buttons":[{"type":"reply","title":"Sim","id":"btn-sim"}]}`)
			},
		},
	}
}

func sendWirePost(t *testing.T, h http.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	h.ServeHTTP(rec, msgAuthed(req))
	return rec
}

// TestSendWireContract_FieldNames trava os nomes do wire das ONZE
// capabilities de envio, cada uma pela sua rota registrada.
func TestSendWireContract_FieldNames(t *testing.T) {
	casos := sendWireCases()
	if len(casos) != 11 {
		t.Fatalf("a suite cobre %d capabilities de envio, quero as 11 enumeradas no CAP-13 + CAP-14 + CAP-15 + CAP-21", len(casos))
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			rec := caso.serve(t)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, quero 200 (corpo: %s)", caso.rota, rec.Code, rec.Body.String())
			}
			env := decodeEnvelope(t, rec)

			// map[string]any, e nao a struct do DTO: decodificar na struct
			// seguiria a tag renomeada e nao mediria nome nenhum.
			var obj map[string]any
			if err := json.Unmarshal(env.Data, &obj); err != nil {
				t.Fatalf("%s: data nao e' um objeto: %v (corpo: %s)", caso.rota, err, rec.Body.String())
			}
			assertSendWireKeys(t, caso.rota, obj)
		})
	}
}
