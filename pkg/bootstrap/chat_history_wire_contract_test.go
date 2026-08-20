package bootstrap

import (
	"encoding/json"
	"net/http"
	"sort"
	"testing"
	"time"
)

// CAP-09A — TRAVA DOS NOMES DO WIRE de GET /chat/history.
//
// Por que este arquivo existe, separado dos outros testes da rota: toda a
// suite de chat_history_route_test.go decodifica a resposta EM
// appport.ChatHistoryMessage / appport.ChatIndexEntry. Um teste assim segue a
// tag: renomeie `text_content` para `body` na struct e o teste continua verde,
// porque o encoder e o decoder passam a concordar no nome errado. Foi
// exatamente isso que a EVAL-09 provou — renomear as tags para o vocabulario
// do tipo orfao `domain.HistoryMessage` (HOUSEKEEP F123) deixava a suite
// INTEIRA passar.
//
// Aqui a assercao e' sobre o JSON REAL, decodificado em map[string]any, pela
// ROTA REGISTRADA (gorilla/mux), com as chaves ENUMERADAS uma a uma. Nenhuma
// lista e' derivada da struct: derivar da struct e' reintroduzir o defeito.
//
// A referencia e' o commit 3dafae0 (handlers.go:5012 e db.go:103), que
// serializava o slice de HistoryMessage direto no corpo.

// chatHistoryWireKeys sao os nomes das chaves de UMA mensagem no wire.
// Escritos a mao, um por linha, a partir de 3dafae0:db.go:103 mais os dois
// campos que a persistencia ganhou depois (quoted_message_id, data_json).
var chatHistoryWireKeys = []string{
	"id",
	"user_id",
	"chat_jid",
	"sender_jid",
	"message_id",
	"timestamp",
	"message_type",
	"text_content",
	"media_link",
	"quoted_message_id",
	"data_json",
}

// chatIndexWireKeys sao os nomes das chaves de UMA entrada do ramo
// `chat_jid=index` (ChatInfo em 3dafae0:handlers.go:5083).
var chatIndexWireKeys = []string{
	"chat_jid",
	"last_updated",
}

// orphanWireKeys e' o vocabulario do tipo domain.HistoryMessage, que NUNCA
// existiu no wire e foi APAGADO na limpeza da F123. O tipo ja' nao pode ser
// usado por engano; estas chaves continuam proibidas porque nada impede
// alguem de as escrever a' mao. A troca por estes
// nomes e' precisamente a mutacao que este arquivo tem de impedir: quem no
// futuro "arrumar" o handler para usar o tipo orfao cai na F123, e nenhum
// outro teste do repo acusa.
var orphanWireKeys = []string{
	"jid",
	"from",
	"body",
	"direction",
	"status",
	"media_url",
}

// seedHistoryRowAllColumns grava uma mensagem com TODAS as colunas nao vazias.
// Necessario porque `quoted_message_id` carrega `omitempty`: com o valor vazio
// a chave nao aparece no JSON e a assercao de presenca nao mediria nada.
func (f *chatHistoryFixture) seedHistoryRowAllColumns(t *testing.T, userID, chatJID, messageID string, ts time.Time) {
	t.Helper()
	query := f.db.Rebind(`INSERT INTO message_history
		(user_id, chat_jid, sender_jid, message_id, timestamp, message_type,
		 text_content, media_link, quoted_message_id, datajson, sender_push_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if _, err := f.db.Exec(query, userID, chatJID, "s@s.whatsapp.net", messageID, ts,
		"text", "texto "+messageID, "https://exemplo/midia.bin", "QUOTED-1",
		`{"origem":"teste"}`, "push"); err != nil {
		t.Fatalf("seed %s/%s: %v", userID, messageID, err)
	}
}

// assertWireKeys confere, chave por chave, a PRESENCA das esperadas, a
// AUSENCIA das do orfao, e denuncia qualquer chave inesperada nomeando-a.
func assertWireKeys(t *testing.T, ramo string, obj map[string]any, esperadas []string) {
	t.Helper()

	for _, chave := range esperadas {
		if _, ok := obj[chave]; !ok {
			t.Errorf("%s: a chave %q SUMIU do wire. As chaves presentes sao %v.\n"+
				"       Os nomes do wire sao contrato publico (3dafae0); renomear uma tag JSON quebra todo cliente.",
				ramo, chave, chavesOrdenadas(obj))
		}
	}

	for _, proibida := range orphanWireKeys {
		if _, ok := obj[proibida]; ok {
			t.Errorf("%s: a chave %q APARECEU no wire. Esse e' o vocabulario do tipo orfao "+
				"tipo apagado na F123 (HOUSEKEEP), cujo vocabulario nunca existiu nesta resposta.\n"+
				"       Chaves presentes: %v", ramo, proibida, chavesOrdenadas(obj))
		}
	}

	permitidas := map[string]bool{}
	for _, chave := range esperadas {
		permitidas[chave] = true
	}
	for chave := range obj {
		if !permitidas[chave] {
			t.Errorf("%s: chave INESPERADA %q no wire. O contrato e' exatamente %v.",
				ramo, chave, esperadas)
		}
	}
}

func chavesOrdenadas(obj map[string]any) []string {
	out := make([]string, 0, len(obj))
	for k := range obj {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestChatHistoryWireContract_MessageFieldNames trava o ramo da LISTA DE
// MENSAGENS.
func TestChatHistoryWireContract_MessageFieldNames(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	f.seedUser(t, "A", 10)
	f.seedHistoryRowAllColumns(t, "A", historyChatA, "A-1",
		time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chat/history?chat_jid="+historyChatA)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}

	// map[string]any, e nao a struct do port: decodificar na struct seguiria a
	// tag renomeada e nao mediria nome nenhum.
	var msgs []map[string]any
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &msgs); err != nil {
		t.Fatalf("data nao e' um array de objetos: %v (corpo: %s)", err, rec.Body.String())
	}
	if len(msgs) != 1 {
		t.Fatalf("len = %d, quero 1 (corpo: %s)", len(msgs), rec.Body.String())
	}

	assertWireKeys(t, "GET /chat/history (mensagens)", msgs[0], chatHistoryWireKeys)
}

// TestChatHistoryWireContract_IndexFieldNames trava o ramo `chat_jid=index`:
// as chaves do ChatInfo E a chave externa do mapa, que e' o user_id do
// chamador.
func TestChatHistoryWireContract_IndexFieldNames(t *testing.T) {
	f := newChatHistoryFixture(t, func() *Values { return userValues("A", 10) })
	f.seedUser(t, "A", 10)
	f.seedHistoryRowAllColumns(t, "A", historyChatA, "A-1",
		time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	rec := f.get(t, "/chat/history?chat_jid=index")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}

	var index map[string][]map[string]any
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &index); err != nil {
		t.Fatalf("data nao e' o mapa do index: %v (corpo: %s)", err, rec.Body.String())
	}

	// A chave EXTERNA e' o user_id do chamador, e nao um rotulo fixo.
	entradas, ok := index["A"]
	if !ok {
		t.Fatalf("o mapa do index nao tem a chave do chamador \"A\"; chaves = %v (corpo: %s)",
			chavesDoIndex(index), rec.Body.String())
	}
	if len(entradas) != 1 {
		t.Fatalf("index[\"A\"] tem %d entradas, quero 1 (corpo: %s)", len(entradas), rec.Body.String())
	}

	assertWireKeys(t, "GET /chat/history?chat_jid=index (ChatInfo)", entradas[0], chatIndexWireKeys)
}

func chavesDoIndex(index map[string][]map[string]any) []string {
	out := make([]string, 0, len(index))
	for k := range index {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
