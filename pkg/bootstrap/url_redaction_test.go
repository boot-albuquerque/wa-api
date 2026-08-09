package bootstrap

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// F75. A query string continua valendo para `/session/ws` porque o WebSocket de
// navegador não tem alternativa. O preço é o token na URL — e a URL vai para o
// log em três sítios. Sem redação, a exceção justificada vira credencial
// registrada, que é o defeito que acabamos de fechar em três outros lugares.

func TestRedactURL_MascaraOToken(t *testing.T) {
	u, err := url.Parse("/session/ws?token=segredo-do-usuario&events=All")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := redactURL(u)

	if strings.Contains(got, "segredo-do-usuario") {
		t.Fatalf("o token sobreviveu na URL registrada: %s", got)
	}
	if !strings.Contains(got, redactedURLValue) {
		t.Errorf("nao ha marcador de redacao em %q; quem le o log perde o sinal de que aquele cliente ainda usa query string", got)
	}
	// Os demais parâmetros continuam: redigir demais cega o diagnóstico.
	if !strings.Contains(got, "events=All") {
		t.Errorf("a redacao levou junto parametro que nao e credencial: %s", got)
	}
}

// TestRedactURL_NaoMutaAOriginal: a URL redigida é para o LOG. Se a redação
// mudasse a URL da requisição, o roteamento e qualquer handler que a lesse
// depois veriam `[REDACTED]` no lugar do token — o log alteraria o que observa.
func TestRedactURL_NaoMutaAOriginal(t *testing.T) {
	const bruta = "/session/ws?token=segredo"
	u, err := url.Parse(bruta)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	_ = redactURL(u)

	if got := u.String(); got != bruta {
		t.Errorf("a URL original foi alterada: %q, want %q", got, bruta)
	}
	if got := u.Query().Get("token"); got != "segredo" {
		t.Errorf("o token da requisicao foi alterado: %q", got)
	}
}

func TestRedactURL_SemTokenNaoMuda(t *testing.T) {
	const bruta = "/chat/list?limit=10"
	u, _ := url.Parse(bruta)

	if got := redactURL(u); got != bruta {
		t.Errorf("URL sem token foi alterada: %q, want %q", got, bruta)
	}
}

func TestRedactURL_NilNaoQuebra(t *testing.T) {
	if got := redactURL(nil); got != "" {
		t.Errorf("redactURL(nil) = %q, want vazio", got)
	}
}

// TestBoundaryRecord_NaoRegistraOToken é o teste que fecha o caminho REAL. Os
// três anteriores provam a função; este prova o SÍTIO — e a diferença importa,
// porque a função pode estar certa e o chamador continuar logando `r.URL`
// direto, que foi exatamente como os três sítios passaram a vazar.
func TestBoundaryRecord_NaoRegistraOToken(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	r := httptest.NewRequest(http.MethodGet, "/session/ws?token=segredo-do-usuario", nil)
	// O logger precisa estar NO CONTEXTO da requisição: writeBoundaryRecord usa
	// hlog.FromRequest, que devolve um logger DESABILITADO quando não encontra
	// um. Sem esta linha o teste passava com o buffer vazio — e passava também
	// com a redação removida, que é como o controle negativo o pegou. Terceira
	// ocorrência da ARMADILHAS 25 nesta leva.
	r = r.WithContext(logger.WithContext(r.Context()))

	writeBoundaryRecord(r, http.StatusOK, 0, 0)

	out := buf.String()

	// Controle de que o registro SAIU. Um buffer vazio satisfaz "não contém o
	// token" sem provar coisa alguma.
	if !strings.Contains(out, "Got API Request") {
		t.Fatalf("o registro de fronteira nao foi emitido; o teste nao esta observando nada: %q", out)
	}
	if strings.Contains(out, "segredo-do-usuario") {
		t.Fatalf("o registro de fronteira gravou o token: %s", out)
	}
	if !strings.Contains(out, redactedURLValue) {
		t.Errorf("o registro nao marca a redacao; some o sinal de que o cliente usa query string: %s", out)
	}
}
