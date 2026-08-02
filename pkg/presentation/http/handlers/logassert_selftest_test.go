package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog/hlog"
)

// O auto-teste do co-gate D. Um helper de assercao que nunca falha e' pior que
// nenhum: os ~5 executores que escrevem teste de handler contra ele precisam
// da prova de que cada uma das quatro clausulas MORDE. Cada caso negativo aqui
// e' uma clausula; se alguem afrouxar checkOutcome/checkNoSecrets, o caso
// correspondente para de acusar erro e este arquivo fica vermelho.
//
// A logica pura (decodeLogLines, checkOutcome, checkNoSecrets) devolve error
// em vez de chamar t.Fatalf justamente para ser testavel no caminho negativo —
// *testing.T e' um tipo concreto e nao se finge.

// mustLines decodifica linhas NDJSON literais do proprio teste.
func mustLines(t *testing.T, raw string) []logLine {
	t.Helper()
	lines, err := decodeLogLines(raw)
	if err != nil {
		t.Fatalf("fixture invalida: %v", err)
	}
	return lines
}

// TestLogAssert_PassesOnWellFormedOutcome roda o helper sobre a saida REAL de
// um handler que loga como a F12 exige: hlog.FromRequest, nivel warn, com a
// causa. E' o caminho positivo de ponta a ponta — zerolog de verdade, cadeia
// hlog de verdade, req_id injetado pelo middleware e nao pelo teste.
func TestLogAssert_PassesOnWellFormedOutcome(t *testing.T) {
	h, capture := logassert.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hlog.FromRequest(r).Warn().
			Err(errors.New("no session for user")).
			Str("route", "/session/status").
			Msg("request rejected")
		w.WriteHeader(http.StatusBadRequest)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session/status", nil))

	got := logassert.OutcomeLogged(t, capture.Records(t), "no session")

	if got.str("req_id") == "" {
		t.Fatal("o registro devolvido nao carrega req_id — a cadeia hlog do Wrap divergiu de router.go")
	}
	if got.str("level") != "warn" {
		t.Fatalf("nivel do registro devolvido: %q", got.str("level"))
	}
}

// TestLogAssert_FailsOnMissingReqID: sem req_id o registro nao correlaciona
// com o registro de fronteira, e o log perde o unico eixo de juncao que tem.
func TestLogAssert_FailsOnMissingReqID(t *testing.T) {
	lines := mustLines(t, `{"level":"error","error":"boom","message":"falhou"}`)

	if _, err := logassert.checkOutcome(lines, nil); err == nil {
		t.Fatal("checkOutcome aceitou registro sem req_id — a clausula (b) nao morde")
	} else if !strings.Contains(err.Error(), "req_id") {
		t.Fatalf("erro nao aponta o req_id ausente: %v", err)
	}
}

// TestLogAssert_FailsOnMissingErrorField: logar o caminho de saida sem a causa
// e' exatamente o Cenario 2 que a F12 existe para evitar.
func TestLogAssert_FailsOnMissingErrorField(t *testing.T) {
	lines := mustLines(t, `{"level":"warn","req_id":"abc","message":"falhou"}`)

	if _, err := logassert.checkOutcome(lines, nil); err == nil {
		t.Fatal("checkOutcome aceitou registro sem campo error — a clausula (b) nao morde")
	}
}

// TestLogAssert_FailsOnWrongLevel: um caminho de saida em info some do
// operador, que filtra por >=warn.
func TestLogAssert_FailsOnWrongLevel(t *testing.T) {
	for _, level := range []string{"info", "debug", "trace"} {
		t.Run(level, func(t *testing.T) {
			lines := mustLines(t, `{"level":"`+level+`","req_id":"abc","error":"boom","message":"falhou"}`)

			if _, err := logassert.checkOutcome(lines, nil); err == nil {
				t.Fatalf("checkOutcome aceitou nivel %q — a clausula (c) nao morde", level)
			} else if !strings.Contains(err.Error(), level) {
				t.Fatalf("erro nao aponta o nivel: %v", err)
			}
		})
	}
}

// TestLogAssert_FailsOnEmptyOutput: nenhum log e' falha, nao sucesso vacuo.
func TestLogAssert_FailsOnEmptyOutput(t *testing.T) {
	if _, err := logassert.checkOutcome(nil, nil); err == nil {
		t.Fatal("checkOutcome aceitou saida vazia")
	}
}

// TestLogAssert_FailsOnMissingErrorSubstring garante que o parametro variadico
// nao e' decorativo.
func TestLogAssert_FailsOnMissingErrorSubstring(t *testing.T) {
	lines := mustLines(t, `{"level":"error","req_id":"abc","error":"boom","message":"falhou"}`)

	if _, err := logassert.checkOutcome(lines, []string{"no session"}); err == nil {
		t.Fatal("checkOutcome aceitou campo error sem a substring pedida")
	}
	if _, err := logassert.checkOutcome(lines, []string{"boom"}); err != nil {
		t.Fatalf("checkOutcome recusou a substring presente: %v", err)
	}
}

// TestLogAssert_FailsOnLeakedSecretValue e' a regressao permanente da F9.4: o
// VALOR de um dos tres segredos em qualquer linha da saida.
func TestLogAssert_FailsOnLeakedSecretValue(t *testing.T) {
	for name, secret := range map[string]string{
		"admin_token":           logassertAdminToken,
		"global_encryption_key": logassertGlobalEncryptionKey,
		"global_hmac_key":       logassertGlobalHMACKey,
	} {
		t.Run(name, func(t *testing.T) {
			lines := mustLines(t,
				`{"level":"error","req_id":"abc","error":"boom","message":"falhou"}`+"\n"+
					`{"level":"info","req_id":"abc","message":"config carregada com `+secret+`"}`)

			// A linha do caminho de saida esta' impecavel...
			if _, err := logassert.checkOutcome(lines, nil); err != nil {
				t.Fatalf("checkOutcome recusou o registro bem formado: %v", err)
			}
			// ...e ainda assim a saida como um todo vazou.
			if err := logassert.checkNoSecrets(lines, nil); err == nil {
				t.Fatal("checkNoSecrets aceitou saida com valor de segredo — a clausula (d) nao morde")
			}
		})
	}
}

// TestLogAssert_FailsOnSecretFieldName: logar a CHAVE ja' e' o bug, mesmo com
// valor que o teste nao conhece — foi assim que bootstrap/main.go:231,247,275
// vazou.
func TestLogAssert_FailsOnSecretFieldName(t *testing.T) {
	for _, key := range logassertSecretKeys {
		t.Run(key, func(t *testing.T) {
			lines := mustLines(t, `{"level":"warn","req_id":"abc","`+key+`":"gerado-aleatoriamente","message":"m"}`)

			if err := logassert.checkNoSecrets(lines, nil); err == nil {
				t.Fatalf("checkNoSecrets aceitou o campo %q", key)
			}
		})
	}
}

// TestLogAssert_ExtraSecretsAreHonored cobre o segredo especifico do caso.
func TestLogAssert_ExtraSecretsAreHonored(t *testing.T) {
	lines := mustLines(t, `{"level":"warn","req_id":"abc","error":"hunter2","message":"m"}`)

	if err := logassert.checkNoSecrets(lines, nil); err != nil {
		t.Fatalf("checkNoSecrets acusou sem segredo conhecido: %v", err)
	}
	if err := logassert.checkNoSecrets(lines, []string{"hunter2"}); err == nil {
		t.Fatal("checkNoSecrets ignorou o segredo extra do caso")
	}
}

// TestLogAssert_FailsOnInvalidJSON e' a clausula (a): uma linha que nao e'
// JSON nao pode ser silenciosamente pulada.
func TestLogAssert_FailsOnInvalidJSON(t *testing.T) {
	if _, err := decodeLogLines(`{"level":"warn","req_id":"abc"` + "\n"); err == nil {
		t.Fatal("decodeLogLines aceitou linha que nao e' JSON — a clausula (a) nao morde")
	}
	if _, err := decodeLogLines("2026/08/02 12:00:00 log em texto puro\n"); err == nil {
		t.Fatal("decodeLogLines aceitou log nao estruturado")
	}
}

// TestLogAssert_CleanOutputPasses fecha o ciclo: a saida boa passa em todas as
// quatro clausulas, entao os casos negativos acima falham pelo motivo que
// dizem, e nao porque o helper recusa tudo.
func TestLogAssert_CleanOutputPasses(t *testing.T) {
	lines := mustLines(t,
		`{"level":"info","req_id":"abc","message":"Got API Request","status":400}`+"\n"+
			`{"level":"warn","req_id":"abc","error":"no session","message":"falhou"}`)

	got, err := logassert.checkOutcome(lines, []string{"no session"})
	if err != nil {
		t.Fatalf("checkOutcome recusou saida limpa: %v", err)
	}
	if got.str("level") != "warn" {
		t.Fatalf("checkOutcome escolheu o registro errado: %s", got.Raw)
	}
	if err := logassert.checkNoSecrets(lines, nil); err != nil {
		t.Fatalf("checkNoSecrets recusou saida limpa: %v", err)
	}
}
