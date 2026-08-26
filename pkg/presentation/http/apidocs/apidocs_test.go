package apidocs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestEspecificacaoEmbutidaEValida afirma que o que vai DENTRO do binário é um
// documento OpenAPI 3.x utilizável — e não um ficheiro que ficou por gerar.
func TestEspecificacaoEmbutidaEValida(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(Specification(), &doc); err != nil {
		t.Fatalf("especificação embutida não é YAML válido: %v", err)
	}
	versao, _ := doc["openapi"].(string)
	if !strings.HasPrefix(versao, "3.") {
		t.Errorf("openapi = %q, esperado 3.x", versao)
	}
	info, ok := doc["info"].(map[string]any)
	if !ok {
		t.Fatal("sem bloco info")
	}
	for _, campo := range []string{"title", "version", "description"} {
		if strings.TrimSpace(toStr(info[campo])) == "" {
			t.Errorf("info.%s vazio", campo)
		}
	}
	if _, ok := doc["paths"].(map[string]any); !ok {
		t.Error("sem secção paths")
	}
	componentes, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("sem secção components")
	}
	esquemas, ok := componentes["securitySchemes"].(map[string]any)
	if !ok {
		t.Fatal("sem securitySchemes: o botão Authorize não apareceria")
	}
	for _, nome := range []string{"TokenSessao", "TokenAdmin"} {
		if _, ok := esquemas[nome]; !ok {
			t.Errorf("securityScheme %q ausente", nome)
		}
	}
}

func toStr(v any) string { s, _ := v.(string); return s }

// TestHandlerServeAPaginaOsAtivosEAEspecificacao exercita os três caminhos que
// a página precisa. Sem o do meio, o Swagger UI carrega e fica em branco — que
// é o modo de falha mais difícil de diagnosticar, porque o HTML responde 200.
func TestHandlerServeAPaginaOsAtivosEAEspecificacao(t *testing.T) {
	h := Handler()

	casos := []struct {
		nome        string
		caminho     string
		tipo        string
		contemTexto string
	}{
		{"pagina", BasePath, "text/html", "swagger-ui"},
		{"pagina com barra", BasePath + "/", "text/html", "SwaggerUIBundle"},
		{"especificacao", SpecPath, "application/yaml", "openapi:"},
		{"bundle", BasePath + "/swagger-ui-bundle.js", "text/javascript", ""},
		{"css", BasePath + "/swagger-ui.css", "text/css", ""},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, caso.caminho, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, esperado 200", rec.Code)
			}
			if tipo := rec.Header().Get("Content-Type"); !strings.Contains(tipo, caso.tipo) {
				t.Errorf("Content-Type %q não contém %q", tipo, caso.tipo)
			}
			if rec.Body.Len() == 0 {
				t.Fatal("corpo vazio")
			}
			if caso.contemTexto != "" && !strings.Contains(rec.Body.String(), caso.contemTexto) {
				t.Errorf("corpo não contém %q", caso.contemTexto)
			}
		})
	}
}

// TestPaginaApontaParaAEspecificacaoServida trava o par que mais facilmente
// diverge: o HTML pede um URL, e o manipulador serve outro. Quando isso
// acontece a página abre e fica vazia, sem erro visível.
func TestPaginaApontaParaAEspecificacaoServida(t *testing.T) {
	if !strings.Contains(indexHTML, `url: "`+SpecPath+`"`) {
		t.Fatalf("a página não pede %q — o Swagger UI carregaria vazio", SpecPath)
	}
}

// TestHandlerRecusaTravessiaDeCaminho: o manipulador serve ficheiros, logo a
// travessia é a pergunta óbvia a fazer-lhe.
func TestHandlerRecusaTravessiaDeCaminho(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+"/../../../etc/passwd", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("travessia devolveu 200: %s", rec.Body.String()[:min(120, rec.Body.Len())])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
