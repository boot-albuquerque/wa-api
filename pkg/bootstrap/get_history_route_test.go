package bootstrap

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"wa-api/pkg/domain"
)

// CAP-32 — GET /webhook/history.
//
// Estes testes exercitam a ROTA REGISTRADA (registerCustomRoutes +
// gorilla/mux) sobre a cadeia de autenticacao de PRODUCAO, reusando a fixture
// do CAP-30 (session_config_route_test.go): a leitura e a escrita compartilham
// o mesmo `SessionConfigRepository` sobre o mesmo SQLite com o schema real, o
// que e' justamente o que permite provar o round-trip.
//
// O contrato aqui e' NOVO e minimo — devolver o limite gravado em
// `users.history`. Nao ha' texto historico para reusar: `41bc8e2^:handlers.go:6497`
// lia historico de MENSAGENS e servia as duas rotas, e o literal
// "History configuration retrieved" foi invencao do stub da migracao
// (HOUSEKEEP F157/F166).

// literalDoStub e' a string que o stub respondia. Fica aqui como VALOR
// PROIBIDO, e nao como discriminador: e' a diferenca que a F166 aponta.
const literalDoStub = "History configuration retrieved"

// getHistory chama a rota e devolve o resultado decodificado junto do corpo
// cru — o corpo cru importa porque parte do contrato e' a PRESENCA do campo,
// que um struct decodificado nao distingue de ausencia.
func (f *sessionCfgFixture) getHistory(t *testing.T) (domain.WebhookHistoryResult, string) {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/webhook/history", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webhook/history: status = %d, quero 200 (corpo: %s)", rec.Code, rec.Body.String())
	}
	var lido domain.WebhookHistoryResult
	if err := json.Unmarshal(decodeEnvelope(t, rec).Data, &lido); err != nil {
		t.Fatalf("data nao e' a configuracao de historico: %v (corpo: %s)", err, rec.Body.String())
	}
	return lido, rec.Body.String()
}

// TESTE 1 (rota) — o valor gravado volta na leitura.
//
// A escrita passa pela ROTA, e nao por um INSERT do teste: o que esta' sob
// prova e' que a leitura le' A MESMA coluna que a escrita grava. Um teste que
// semeasse a linha na mao passaria mesmo se as duas rotas usassem colunas
// diferentes.
func TestGetHistoryRoute_DevolveOLimiteGravado(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()

	const limite = 50
	if rec := f.do(t, http.MethodPost, "/session/history", `{"history":50}`); rec.Code != http.StatusOK {
		t.Fatalf("POST /session/history = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := f.storedHistory(t); got != limite {
		t.Fatalf("users.history = %d, quero %d — a linha de base da leitura nao existe", got, limite)
	}

	lido, corpo := f.getHistory(t)
	if lido.History != limite {
		t.Fatalf("History = %d, quero %d — a rota nao le' users.history (corpo: %s)", lido.History, limite, corpo)
	}
}

// TESTE 2 (rota) — history=0 responde 0 PRESENTE no corpo.
//
// Este e' o teste da HOUSEKEEP F165. Zero e' "historico desligado", a resposta
// mais comum desta rota, e com `omitempty` no campo `History` ele SUMIA do
// corpo: uma rota de leitura que omite o valor lido nao responde nada. A
// assercao e' sobre o CORPO CRU, e nao sobre o struct decodificado, porque
// `json.Unmarshal` entrega 0 tanto para `"History":0` quanto para o campo
// ausente — asserir pelo struct deixaria o defeito passar.
func TestGetHistoryRoute_HistoryZeroApareceNoCorpo(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()

	// seedUser ja' grava history = 0; a escrita explicita torna o estado
	// medido pelo teste, e nao herdado da fixture.
	if rec := f.do(t, http.MethodPost, "/session/history", `{"history":0}`); rec.Code != http.StatusOK {
		t.Fatalf("POST /session/history = %d (%s)", rec.Code, rec.Body.String())
	}

	lido, corpo := f.getHistory(t)
	if lido.History != 0 {
		t.Fatalf("History = %d, quero 0 (corpo: %s)", lido.History, corpo)
	}
	if !strings.Contains(corpo, `"History":0`) {
		t.Fatalf("o corpo nao carrega `\"History\":0`: o `omitempty` do campo History voltou e o desligamento "+
			"do historico deixou de ecoar o valor lido (F165); corpo: %s", corpo)
	}
}

// TESTE 3 (rota) — falha de leitura no banco: 500, sem valor inventado.
//
// O erro vem do driver REAL (banco fechado), como no teste de falha de
// gravacao do CAP-30, e nao de um erro fabricado por dublê.
func TestGetHistoryRoute_FalhaDeLeitura_500SemValorInventado(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()

	// Uma requisicao antes de fechar o banco, para que AuthAlice ja' tenha a
	// entrada no cache de autenticacao: sem ela a falha viria da AUTENTICACAO,
	// e o teste mediria outra coisa.
	f.do(t, http.MethodGet, "/chats/history?chat_jid=index", "")

	if err := f.db.Close(); err != nil {
		t.Fatalf("fechar o banco: %v", err)
	}

	rec := f.do(t, http.MethodGet, "/webhook/history", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500 (corpo: %s)", rec.Code, rec.Body.String())
	}
	if corpo := rec.Body.String(); strings.Contains(corpo, `"History"`) {
		t.Fatalf("a falha de leitura respondeu um valor de History: %s", corpo)
	}
}

// TESTE 4 (rota) — o corpo nao carrega mais o literal do stub.
//
// A F157 fechava dez rotas que respondiam 200 sem tocar em nada. Esta era a
// ultima, e o literal e' a assinatura dela.
func TestGetHistoryRoute_NaoRespondeMaisOLiteralDoStub(t *testing.T) {
	f := newSessionCfgFixture(t)
	f.seedCacheEntry()

	if rec := f.do(t, http.MethodPost, "/session/history", `{"history":50}`); rec.Code != http.StatusOK {
		t.Fatalf("POST /session/history = %d (%s)", rec.Code, rec.Body.String())
	}

	_, corpo := f.getHistory(t)
	if strings.Contains(corpo, literalDoStub) {
		t.Fatalf("a rota voltou a responder %q — o stub da F151/F157 esta' de volta; corpo: %s", literalDoStub, corpo)
	}
}
