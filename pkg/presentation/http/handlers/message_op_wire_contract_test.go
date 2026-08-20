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
					EditMessageFunc: func(context.Context, string, domain.JID, string, string) (domain.MessageSendResult, error) {
						return sendWireResult("wire-edit-1"), nil
					},
				}
				return sendWirePost(t, mutationRouter(cm, &contractsfake.JIDResolver{}),
					"/chat/send/edit", `{"Phone":"5511999999999","Id":"MSG1","Body":"novo texto"}`)
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
					"/chat/delete/message", `{"Phone":"5511999999999","Id":"MSG1"}`)
			},
		},
		{
			// A react é travada na forma HISTÓRICA porque é essa que ela
			// devolve — medido contra o servidor real em 2026-08-20:
			//   {"Details":"Sent","Id":"3EB0D89C...","Timestamp":1787255613}
			// Isto DOCUMENTA a divergência da F190; não a aprova.
			nome:      "react",
			rota:      "POST /chat/react",
			esperadas: formaHistorica,
			alheias:   append(append([]string{}, formaNova...), vocabularioF123...),
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
					`{"Phone":"5511999999999","Id":"MSG1","Body":"👍"}`)
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

// TestMessageOpWireContract_ReactDivergeDasOutras é a asserção que torna a
// F190 VISÍVEL em vez de implícita.
//
// Os testes acima passariam se um dia a react fosse alinhada com as outras —
// bastaria alguém trocar as duas listas. Este falha, de propósito, no dia em
// que a divergência acabar: obriga quem a alinhar a vir aqui, ler a F190, e
// remover a trava conscientemente, em vez de a mudança passar como detalhe.
func TestMessageOpWireContract_ReactDivergeDasOutras(t *testing.T) {
	var react, edit map[string]any
	for _, caso := range messageOpWireCases() {
		var obj map[string]any
		if err := json.Unmarshal(decodeEnvelope(t, caso.serve(t)).Data, &obj); err != nil {
			t.Fatalf("%s: %v", caso.rota, err)
		}
		switch caso.nome {
		case "react":
			react = obj
		case "edit":
			edit = obj
		}
	}

	if _, ok := edit["message_id"]; !ok {
		t.Fatal("edit deixou de devolver message_id; a premissa deste teste caiu")
	}
	if _, ok := react["message_id"]; ok {
		t.Fatal("a react passou a devolver message_id: a divergencia da F190 ACABOU.\n" +
			"       Se foi decisao, remova este teste e alinhe as listas em messageOpWireCases.\n" +
			"       Se nao foi, alguem mudou contrato publico sem reparar.")
	}
	if _, ok := react["Id"]; !ok {
		t.Fatal("a react deixou de devolver Id sem passar a devolver message_id: o wire ficou sem identificador nenhum")
	}
}
