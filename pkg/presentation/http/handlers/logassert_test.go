package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

// Co-gate D — o helper de assercao de log dos handlers.
//
// Todo teste de CAMINHO DE SAIDA (resposta >=400, ou `if err != nil` que nao
// propaga) desta camada assere, sobre a saida real do zerolog:
//
//	a. o registro e' JSON valido;
//	b. carrega `error` E `req_id`;
//	c. o nivel e' warn ou error — nunca info/debug;
//	d. NENHUM registro contem valor (nem nome de campo) de admin_token,
//	   global_encryption_key ou global_hmac_key — regressao permanente do
//	   achado da F9.4, onde bootstrap/main.go:231,247,275 loga os tres em
//	   claro quando sao gerados automaticamente.
//
// A checagem (d) roda sobre a saida INTEIRA, nao so' sobre o registro
// encontrado: um segredo vazado numa linha vizinha e' o mesmo vazamento.
//
// Derivado de boundaryDeps/decodeRecords/boundaryRecords
// (pkg/bootstrap/boundary_log_test.go:77,132,149). Vive aqui, e nao num pacote
// proprio, porque um pacote de helper seria codigo de producao e entraria no
// denominador de cobertura (Cenario 3 do plano).

// Valores-sentinela dos tres segredos. Um teste que precise plantar
// admin_token / chave de encriptacao / chave HMAC no caminho do handler usa
// ESTES valores — e' o que torna a checagem (d) nao-vacua: o segredo tem de
// existir no teste para que sua ausencia no log signifique algo.
const (
	logassertAdminToken          = "logassert-admin-token-must-never-be-logged"
	logassertGlobalEncryptionKey = "logassert-global-encryption-key-must-never-be-logged"
	logassertGlobalHMACKey       = "logassert-global-hmac-key-must-never-be-logged"
)

// logassertSecretKeys sao nomes de campo proibidos em qualquer registro, em
// qualquer nivel de aninhamento. Logar a CHAVE ja' e' o bug da F9.4,
// independentemente do valor.
var logassertSecretKeys = []string{"admin_token", "global_encryption_key", "global_hmac_key"}

// logassertSecretValues sao as substrings proibidas em qualquer registro.
var logassertSecretValues = []string{
	logassertAdminToken,
	logassertGlobalEncryptionKey,
	logassertGlobalHMACKey,
}

// logLine e' um registro decodificado, com a linha crua preservada — a
// checagem de segredo e' feita sobre o texto original, para pegar o vazamento
// em qualquer profundidade.
type logLine struct {
	Fields map[string]any
	Raw    string
}

func (l logLine) str(key string) string {
	if v, ok := l.Fields[key].(string); ok {
		return v
	}
	return ""
}

func (l logLine) has(key string) bool {
	_, ok := l.Fields[key]
	return ok
}

// logCapture guarda a saida de log de uma requisicao.
type logCapture struct{ buf bytes.Buffer }

// logAssertions e' o namespace do helper. A instancia unica `logassert` da' a
// forma de chamada `logassert.OutcomeLogged(t, recs, ...)` sem custo de um
// pacote separado.
type logAssertions struct{}

var logassert = logAssertions{}

// Wrap monta a mesma cadeia request-scoped que router.go:206-217 instala
// (hlog.NewHandler + RequestIDHandler("req_id")), de modo que o handler sob
// teste veja de hlog.FromRequest exatamente o logger que veria em producao.
func (logAssertions) Wrap(h http.Handler) (http.Handler, *logCapture) {
	capture := &logCapture{}
	l := zerolog.New(&capture.buf).With().Timestamp().Logger()
	return hlog.NewHandler(l)(hlog.RequestIDHandler("req_id", "Request-Id")(h)), capture
}

// Records decodifica a saida como NDJSON. Falha se alguma linha nao for JSON
// valido — e' a checagem (a).
func (c *logCapture) Records(t *testing.T) []logLine {
	t.Helper()
	out, err := decodeLogLines(c.buf.String())
	if err != nil {
		t.Fatalf("co-gate D: %v", err)
	}
	return out
}

// decodeLogLines e' a logica pura de (a).
func decodeLogLines(s string) ([]logLine, error) {
	var out []logLine
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			return nil, fmt.Errorf("linha de log nao e' JSON valido (%v): %s", err, line)
		}
		out = append(out, logLine{Fields: fields, Raw: line})
	}
	return out, nil
}

// OutcomeLogged e' o ponto de entrada do co-gate D: exige que exista ao menos
// um registro de caminho de saida bem formado (b + c), que o campo `error`
// contenha cada substring pedida, e que NENHUM registro carregue segredo (d).
// Devolve o registro encontrado, para assercoes adicionais do chamador.
func (a logAssertions) OutcomeLogged(t *testing.T, recs []logLine, wantErrSubstrings ...string) logLine {
	t.Helper()
	got, err := a.checkOutcome(recs, wantErrSubstrings)
	if err != nil {
		t.Fatalf("co-gate D: %v", err)
	}
	a.NoSecrets(t, recs)
	return got
}

// NoSecrets e' a checagem (d) isolada, para o teste que so' quer o guarda de
// vazamento. `extra` acrescenta segredos especificos do caso.
func (a logAssertions) NoSecrets(t *testing.T, recs []logLine, extra ...string) {
	t.Helper()
	if err := a.checkNoSecrets(recs, extra); err != nil {
		t.Fatalf("co-gate D: %v", err)
	}
}

// checkOutcome e' a logica pura de (b) e (c) — devolve erro em vez de falhar,
// e e' o que o auto-teste exercita nos casos negativos.
func (logAssertions) checkOutcome(recs []logLine, wantErrSubstrings []string) (logLine, error) {
	if len(recs) == 0 {
		return logLine{}, fmt.Errorf("nenhum registro de log: o caminho de saida nao logou nada")
	}
	var candidates []logLine
	for _, r := range recs {
		if r.has("error") {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return logLine{}, fmt.Errorf("nenhum dos %d registros carrega campo `error` — "+
			"o caminho de saida logou sem a causa", len(recs))
	}
	var last error
	for _, r := range candidates {
		if !r.has("req_id") {
			last = fmt.Errorf("registro com `error` nao carrega `req_id` (nao correlaciona "+
				"com o registro de fronteira): %s", r.Raw)
			continue
		}
		if lvl := r.str("level"); lvl != "warn" && lvl != "error" {
			last = fmt.Errorf("registro de caminho de saida no nivel %q — tem de ser warn ou error: %s",
				lvl, r.Raw)
			continue
		}
		if miss := missingSubstrings(r.str("error"), wantErrSubstrings); miss != "" {
			last = fmt.Errorf("campo `error` (%q) nao contem %q: %s", r.str("error"), miss, r.Raw)
			continue
		}
		return r, nil
	}
	return logLine{}, last
}

func missingSubstrings(got string, want []string) string {
	for _, w := range want {
		if !strings.Contains(got, w) {
			return w
		}
	}
	return ""
}

// checkNoSecrets e' a logica pura de (d).
func (logAssertions) checkNoSecrets(recs []logLine, extra []string) error {
	for _, r := range recs {
		for _, k := range logassertSecretKeys {
			if strings.Contains(r.Raw, `"`+k+`":`) {
				return fmt.Errorf("registro carrega o campo proibido %q (F9.4): %s", k, r.Raw)
			}
		}
		for _, v := range append(append([]string{}, logassertSecretValues...), extra...) {
			if v != "" && strings.Contains(r.Raw, v) {
				return fmt.Errorf("valor de segredo vazou no log (F9.4): %s", r.Raw)
			}
		}
	}
	return nil
}
