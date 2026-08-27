package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"wa-api/pkg/presentation/http/contracttest"
)

// Teste de contrato da família MENSAGENS, no molde de
// handler_group_info_contract_test.go.
//
// Ele reutiliza sendWireCases() — as quinze capabilities de envio já montadas
// com a sua ROTA REGISTADA (gorilla/mux) e os seus dublês — e acrescenta o que
// aquela suíte não afirma:
//
//  1. TODA chave do corpo, recursivamente, é snake_case minúsculo, afirmado
//     pelo helper partilhado e não por uma lista escrita à mão;
//  2. as chaves da forma HISTÓRICA (`Details`, `Id`, `Timestamp`) e os nomes
//     Go dos campos do domínio desapareceram;
//  3. o `timestamp` é null — e não 0 — quando o motor não reportou instante.
//     0 no fio é 1970-01-01T00:00:00Z, uma data real.

// TestSendFamily_ContratoPublico_NomesCanonicos é a afirmação (1), pelas
// quinze rotas registadas.
func TestSendFamily_ContratoPublico_NomesCanonicos(t *testing.T) {
	for _, caso := range sendWireCases() {
		t.Run(caso.nome, func(t *testing.T) {
			rec := caso.serve(t)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, quero 200; corpo: %s", caso.rota, rec.Code, rec.Body.String())
			}
			contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
		})
	}
}

// TestSendFamily_ContratoPublico_ChavesAntigasSumiram é a afirmação (2).
//
// `Details`/`Id`/`Timestamp` são a forma histórica do envio
// (`git show 41bc8e2^:handlers.go`); `MessageID`, `Status`, `CaptionStatus` e
// `CaptionMessageID` são os nomes Go dos campos de domain.Send*Result, que é o
// que o codificador emitiria se alguém removesse a etiqueta em vez de a
// corrigir.
func TestSendFamily_ContratoPublico_ChavesAntigasSumiram(t *testing.T) {
	for _, caso := range sendWireCases() {
		t.Run(caso.nome, func(t *testing.T) {
			rec := caso.serve(t)
			contracttest.AssertNoKeys(t, rec.Body.Bytes(),
				"Details", "Id", "Timestamp",
				"MessageID", "Status", "CaptionStatus", "CaptionMessageID",
			)
		})
	}
}

// TestSendFamily_ContratoPublico_ValoresMapeados prova que o apresentador pôs
// os VALORES certos nas chaves certas — uma troca entre message_id e status
// passaria em todos os testes acima.
func TestSendFamily_ContratoPublico_ValoresMapeados(t *testing.T) {
	for _, caso := range sendWireCases() {
		t.Run(caso.nome, func(t *testing.T) {
			rec := caso.serve(t)
			env := decodeEnvelope(t, rec)
			var obj map[string]any
			if err := json.Unmarshal(env.Data, &obj); err != nil {
				t.Fatalf("%s: data não é objeto: %v", caso.rota, err)
			}
			// Cada dublê devolve sendWireResult("wire-<nome>-1") em
			// sendWireCases, e o instante é sendWireSentAt para todos.
			queroID := "wire-" + caso.nome + "-1"
			if obj["message_id"] != queroID {
				t.Errorf("%s: message_id = %#v, quero %q", caso.rota, obj["message_id"], queroID)
			}
			if obj["timestamp"] != float64(sendWireSentAt) {
				t.Errorf("%s: timestamp = %#v, quero %d", caso.rota, obj["timestamp"], sendWireSentAt)
			}
			if obj["status"] != "sent" {
				t.Errorf("%s: status = %#v, quero \"sent\"", caso.rota, obj["status"])
			}
		})
	}
}

// TestSendAudio_ContratoPublico_LegendaAusenteENull trava a decisão sobre o par
// da legenda: as duas chaves EXISTEM sempre, com null quando não houve legenda.
//
// Sem isso, um cliente não distingue "não pedi legenda" de "esta versão deixou
// de mandar a chave" — que é exactamente o que `omitempty` fazia antes.
func TestSendAudio_ContratoPublico_LegendaAusenteENull(t *testing.T) {
	var caso sendWireCase
	for _, c := range sendWireCases() {
		if c.nome == "audio" {
			caso = c
		}
	}
	if caso.serve == nil {
		t.Fatal("a capability de áudio saiu de sendWireCases")
	}
	rec := caso.serve(t)
	env := decodeEnvelope(t, rec)
	var obj map[string]any
	if err := json.Unmarshal(env.Data, &obj); err != nil {
		t.Fatalf("data não é objeto: %v", err)
	}
	for _, chave := range []string{"caption_message_id", "caption_status"} {
		valor, presente := obj[chave]
		if !presente {
			t.Errorf("%s ausente: a chave tem de existir mesmo sem legenda", chave)
		}
		if valor != nil {
			t.Errorf("%s = %#v, quero null quando não houve legenda", chave, valor)
		}
	}
}
