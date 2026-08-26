package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wa-api/pkg/domain/apperr"
)

// Este ficheiro trava a propriedade de segurança mais silenciosa da fronteira
// HTTP: um erro que NÃO seja da taxonomia nunca pode chegar ao cliente com o
// seu texto próprio.
//
// POR QUE PRECISA DE TESTE. `reportPanic` (pkg/bootstrap/router.go) entrega a
// RespondJSON um `fmt.Errorf("panic: %v", rec)`, e o valor de um panic de
// runtime traz endereços, tipos internos e, num panic com dado do pedido,
// potencialmente o próprio dado. Hoje isso não vaza porque RespondJSON só
// serializa `err` quando ele é `*apperr.AppError`, e cai em
// `genericErrorMessage` caso contrário. Nada, porém, obriga a que continue
// assim: trocar o ramo `else` por `err.Error()` seria uma linha, passaria em
// todos os outros testes, e transformaria cada panic numa fuga.

// segredosQueNuncaPodemSair são fragmentos que, se aparecerem num corpo de
// resposta, significam que algo interno atravessou a fronteira.
var segredosQueNuncaPodemSair = []string{
	"panic",
	"goroutine",
	"runtime error",
	"nil pointer",
	"/Users/",
	"wa-api/pkg/",
	"SELECT ",
	"password",
	"secret",
}

func TestRespondJSONNaoVazaDetalheDeErroInterno(t *testing.T) {
	casos := []struct {
		nome   string
		status int
		err    error
	}{
		{
			nome:   "panic de runtime, como reportPanic o entrega",
			status: http.StatusInternalServerError,
			err:    fmt.Errorf("panic: %v", "runtime error: invalid memory address or nil pointer dereference"),
		},
		{
			nome:   "erro com caminho de ficheiro do servidor",
			status: http.StatusInternalServerError,
			err:    errors.New("open /Users/operador/wa-live-data/dbdata/main.db: permission denied"),
		},
		{
			nome:   "erro com consulta SQL",
			status: http.StatusInternalServerError,
			err:    errors.New(`pq: syntax error at or near "SELECT token FROM users WHERE id=$1"`),
		},
		{
			nome:   "erro com segredo embutido",
			status: http.StatusBadGateway,
			err:    errors.New("dial failed: proxy socks5://utilizador:segredo@interno:1080"),
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			rec := httptest.NewRecorder()
			RespondJSON(rec, caso.status, nil, caso.err)

			corpo := rec.Body.String()
			for _, proibido := range segredosQueNuncaPodemSair {
				if strings.Contains(strings.ToLower(corpo), strings.ToLower(proibido)) {
					t.Errorf("o corpo da resposta contém %q, que é detalhe interno:\n%s", proibido, corpo)
				}
			}

			var envelope map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("corpo não é JSON: %v", err)
			}
			if envelope["success"] != false {
				t.Errorf("success = %v, esperado false", envelope["success"])
			}
			// O texto que sobra tem de ser o genérico do status, e nada mais.
			if got, want := envelope["error"], strings.ToLower(http.StatusText(caso.status)); got != want {
				t.Errorf("error = %q, esperado o texto genérico %q", got, want)
			}
		})
	}
}

// TestRespondJSONPreservaOErroDaTaxonomia é o par de controlo do teste acima.
//
// Sem ele, apagar o ramo de `*apperr.AppError` faria o teste anterior passar —
// tudo passaria a ser genérico, incluindo as recusas que o cliente PRECISA de
// distinguir. Segurança que apaga a informação legítima não é segurança.
func TestRespondJSONPreservaOErroDaTaxonomia(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondJSON(rec, http.StatusInternalServerError, nil,
		apperr.New("missing_chat", apperr.CategoryValidation, "missing chat in payload", false, nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperado 400: a categoria da taxonomia é que decide, não o argumento", rec.Code)
	}
	var envelope struct {
		Code  int `json:"code"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo não é JSON: %v", err)
	}
	if envelope.Error.Code != "missing_chat" {
		t.Errorf("error.code = %q, esperado missing_chat", envelope.Error.Code)
	}
	if envelope.Error.Message != "missing chat in payload" {
		t.Errorf("error.message = %q", envelope.Error.Message)
	}
}
