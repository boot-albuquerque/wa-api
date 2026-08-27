package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
)

// HOUSEKEEP F190 — trava dos nomes do wire das TRÊS operações de mensagem que
// ficaram de fora de send_wire_contract_test.go.
//
// Aquele ficheiro trava DOZE rotas de `/chat/send/*` e diz, no topo, que
// "capability nova que fica de fora desta trava é a próxima F137". Ficaram
// três, e nenhuma delas é `/chat/send/*` no nome — foi por aí que escaparam:
//
//	/chat/send/edit       envia uma mensagem (a edição)
//	/chat/delete/message  envia uma mensagem (a revogação)
//	/chat/react           envia uma mensagem (a reação)
//
// As três devolvem identificador e instante, como as doze. Renomear
// `message_id` em qualquer uma passa hoje com a suíte verde — que é
// exatamente o `REQUIRED_FIX` da EVAL-09 que originou o ficheiro irmão.
//
// POR QUE ESTE FICHEIRO EM VEZ DE ACRESCENTAR CASOS LÁ: o outro tem uma trava
// de contagem (`len(casos) != 12`) e uma identidade declarada — "a superfície
// de envio". Enfiar lá dentro rotas que não são `/chat/send/*` diluiria o que
// ele afirma. Aqui a afirmação é outra: operações que MEXEM numa mensagem
// existente ou reagem a ela.
//
// E A DIVERGÊNCIA QUE ESTE FICHEIRO NÃO CONSERTA, apenas DECLARA: `/chat/react`
// devolve `{Details, Id, Timestamp}` — a forma HISTÓRICA que a F131 descartou
// por escrito — enquanto as outras catorze devolvem
// `{message_id, timestamp, status}`. Um cliente que leia `message_id` das
// treze rotas de envio PARTE ao ler a reação.
//
// Travar a react na forma que ela REALMENTE tem, em vez de na forma que
// deveria ter, é escolha deliberada: alinhá-la é mudança de contrato público e
// precisa de decisão, não de um teste que a force de passagem. O que o teste
// impede é a divergência mudar outra vez sem ninguém reparar.

// messageOpWireCase é uma operação sobre mensagem, com o vocabulário que a sua
// resposta TEM e o que ela não pode ter.
//
// As chaves esperadas são por caso, e não uma lista global como no ficheiro
// irmão, precisamente porque a react diverge: uma lista única obrigaria a
// escolher entre não a cobrir ou fingir que ela é igual às outras.
type messageOpWireCase struct {
	nome      string
	rota      string
	esperadas []string
	alheias   []string
	serve     func(t *testing.T) *httptest.ResponseRecorder
}

// formaNova e formaHistorica são os dois vocabulários em jogo, escritos à mão
// um por linha — nunca derivados da struct por reflexão, que reintroduziria o
// defeito que a trava existe para apanhar.
var (
	formaNova       = []string{"message_id", "timestamp", "status"}
	formaHistorica  = []string{"Details", "Id", "Timestamp"}
	vocabularioF123 = []string{"jid", "from", "body", "direction", "media_url"}
)

func messageOpWireCases() []messageOpWireCase {
	reactResult := func(id string) domain.MessageSendResult { return sendWireResult(id) }

	return []messageOpWireCase{
		{
			nome:      "edit",
			rota:      "POST /chat/send/edit",
			esperadas: formaNova,
			alheias:   append(append([]string{}, formaHistorica...), vocabularioF123...),
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				cm := &contractsfake.ChatMessenger{
					EditMessageFunc: func(context.Context, string, domain.JID, string, string, *domain.EditContextInfo) (domain.MessageSendResult, error) {
						return sendWireResult("wire-edit-1"), nil
					},
				}
				return sendWirePost(t, mutationRouter(cm, &contractsfake.JIDResolver{}),
					"/chat/send/edit", `{"phone":"5511999999999","id":"MSG1","body":"novo texto"}`)
			},
		},
		{
			nome:      "delete_message",
			rota:      "POST /chat/delete/message",
			esperadas: formaNova,
			alheias:   append(append([]string{}, formaHistorica...), vocabularioF123...),
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				cm := &contractsfake.ChatMessenger{
					RevokeMessageFunc: func(context.Context, string, domain.JID, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-del-1"), nil
					},
				}
				return sendWirePost(t, mutationRouter(cm, &contractsfake.JIDResolver{}),
					"/chat/delete/message", `{"phone":"5511999999999","id":"MSG1"}`)
			},
		},
		{
			// A react ESTEVE travada na forma histórica, porque era essa que
			// ela devolvia — medido contra o servidor real em 2026-08-20:
			//   {"Details":"Sent","id":"3EB0D89C...","Timestamp":1787255613}
			//
			// Alinhada na F190 depois de autorização explícita do humano: o
			// use case passou a devolver um domain.SendReactionResult tipado
			// em vez de um map literal. O tipo é o que importa mais do que os
			// nomes — sem ele não havia onde pendurar a tag, e foi por isso
			// que a rota escapou a esta trava.
			nome:      "react",
			rota:      "POST /chat/react",
			esperadas: formaNova,
			alheias:   append(append([]string{}, formaHistorica...), vocabularioF123...),
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				sm := &contractsfake.ChatMessenger{
					SendReactionFunc: func(context.Context, string, domain.JID, domain.Reaction) (domain.MessageSendResult, error) {
						return reactResult("wire-react-1"), nil
					},
				}
				r := mux.NewRouter()
				r.Handle("/chat/react",
					NewReactHandler(message.NewReactUseCase(sm, &contractsfake.JIDResolver{}, silentLogger{}))).
					Methods(http.MethodPost)
				return sendWirePost(t, r, "/chat/react",
					`{"phone":"5511999999999","id":"MSG1","body":"👍"}`)
			},
		},
	}
}

// TestMessageOpWireContract_FieldNames trava as três rotas que operam sobre
// uma mensagem e que a trava da superfície de envio não alcança.
func TestMessageOpWireContract_FieldNames(t *testing.T) {
	casos := messageOpWireCases()
	if len(casos) != 3 {
		t.Fatalf("a suite cobre %d operacoes, quero as 3 da F190 (edit, delete/message, react)", len(casos))
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			rec := caso.serve(t)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, quero 200 (corpo: %s)", caso.rota, rec.Code, rec.Body.String())
			}

			// map[string]any, e não a struct do DTO: decodificar na struct
			// SEGUIRIA a tag renomeada e não mediria nome nenhum.
			var obj map[string]any
			if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &obj); err != nil {
				t.Fatalf("%s: data nao e' um objeto: %v (corpo: %s)", caso.rota, err, rec.Body.String())
			}

			for _, chave := range caso.esperadas {
				if _, ok := obj[chave]; !ok {
					t.Errorf("%s: a chave %q SUMIU do wire. Presentes: %v.\n"+
						"       Os nomes do wire sao contrato publico; renomear uma tag JSON quebra todo cliente.",
						caso.rota, chave, sendWireChavesOrdenadas(obj))
				}
			}
			for _, proibida := range caso.alheias {
				if _, ok := obj[proibida]; ok {
					t.Errorf("%s: a chave %q APARECEU no wire, e e' de outro vocabulario.\n"+
						"       Presentes: %v", caso.rota, proibida, sendWireChavesOrdenadas(obj))
				}
			}
		})
	}
}

// TestMessageOpWireContract_TodasNaMesmaForma substitui o
// TestMessageOpWireContract_ReactDivergeDasOutras, que existia para tornar a
// divergência da F190 VISÍVEL e falhava, de propósito, no dia em que ela
// acabasse.
//
// Esse dia chegou, e o teste fez exatamente o que devia: obrigou quem alinhou a
// react a vir aqui, ler a F190, e remover a trava conscientemente em vez de a
// mudança passar como detalhe. A mensagem que ele deu foi
// "a divergencia da F190 ACABOU. Se foi decisao, remova este teste".
//
// Foi decisão, e este é o que fica no lugar: a afirmação passa a ser que as
// TRÊS estão na mesma forma. Um teste que só verificasse cada uma isoladamente
// deixaria a próxima divergir sem que a INCOERÊNCIA fosse dita em lado nenhum.
func TestMessageOpWireContract_TodasNaMesmaForma(t *testing.T) {
	chaves := map[string][]string{}
	for _, caso := range messageOpWireCases() {
		var obj map[string]any
		if err := json.Unmarshal(decodeEnvelope(t, caso.serve(t)).Data, &obj); err != nil {
			t.Fatalf("%s: %v", caso.rota, err)
		}
		chaves[caso.nome] = sendWireChavesOrdenadas(obj)
	}

	if len(chaves) < 2 {
		t.Fatal("menos de duas operacoes: nao ha' o que comparar")
	}

	var referencia string
	for nome, ks := range chaves {
		if referencia == "" {
			referencia = nome
			continue
		}
		if !mesmasChaves(chaves[referencia], ks) {
			t.Errorf("%s devolve %v e %s devolve %v: as operacoes sobre mensagem divergem no wire.\n"+
				"       Se a divergencia for deliberada, ela tem de estar REGISTADA — foi assim que a F190 nasceu.",
				referencia, chaves[referencia], nome, ks)
		}
	}
}

func mesmasChaves(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
