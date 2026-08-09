package devui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEnabled_PadraoEhDesligado é o teste que mais importa deste pacote: o
// esquecimento tem de ser seguro. Se alguém inverter a lógica para opt-out,
// uma página que lista os endpoints da API passa a ser servida em produção
// sem ninguém ter pedido.
func TestEnabled_PadraoEhDesligado(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	if Enabled() {
		t.Error("Enabled() = true sem a variável definida; o padrão tem de ser desligado")
	}
}

func TestEnabled_ValoresAceitos(t *testing.T) {
	for _, tc := range []struct {
		valor string
		quero bool
	}{
		{"true", true}, {"TRUE", true}, {"True", true},
		{"1", true}, {"yes", true}, {"  true  ", true},
		{"false", false}, {"0", false}, {"no", false},
		{"", false}, {"talvez", false},
		// "2" não é verdadeiro: só os valores da convenção do projeto
		// contam, senão qualquer lixo na variável liga a página.
		{"2", false},
	} {
		t.Run(tc.valor, func(t *testing.T) {
			t.Setenv(EnvEnabled, tc.valor)
			if got := Enabled(); got != tc.quero {
				t.Errorf("Enabled() com %q = %v, quero %v", tc.valor, got, tc.quero)
			}
		})
	}
}

// TestHandler_RaizServeOIndex: a URL do próprio BasePath tem de entregar a
// página, e não um índice de diretório.
func TestHandler_RaizServeOIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<!doctype html>") {
		t.Errorf("o corpo não parece HTML: %.80q", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, quero text/html", ct)
	}
}

// TestPaginaSuportaMultiplasSessoes trava o que a pagina precisa oferecer:
// varias sessoes simultaneas e as DUAS formas de encerrar, que nao sao
// equivalentes — desconectar mantem o pareamento, logout desvincula o
// aparelho e exige QR novo. Uma pagina que ofereca so' uma das duas leva o
// operador a desvincular quando queria apenas derrubar a conexao.
func TestPaginaSuportaMultiplasSessoes(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath, nil))
	body := rec.Body.String()

	for _, marca := range []string{
		"/session/disconnect", // derruba o transporte, mantem o pareamento
		"/session/logout",     // desvincula o aparelho
		"/admin/users",        // criacao de sessao nova
		"localStorage",        // registro local dos tokens (a API os redige)
	} {
		if !strings.Contains(body, marca) {
			t.Errorf("a pagina nao menciona %q", marca)
		}
	}
}

func TestHandler_ArquivoNomeado(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+indexFile, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", rec.Code)
	}
}

// TestHandler_NaoInventaConteudo: pedir um arquivo que não existe tem de dar
// 404, e não cair no index. Cair no index faria um erro de digitação parecer
// sucesso.
func TestHandler_ArquivoInexistenteDa404(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+"nao-existe.html", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, quero 404", rec.Code)
	}
}

// TestHandler_NaoEscapaDoDiretorioEmbutido: um path traversal não pode
// alcançar nada fora de assets/. Com embed.FS o alcance máximo já seria o
// binário, mas o teste trava a propriedade em vez de confiar nela.
func TestHandler_NaoEscapaDoDiretorio(t *testing.T) {
	for _, alvo := range []string{
		BasePath + "../devui.go",
		BasePath + "..%2fdevui.go",
		BasePath + "../../bootstrap/wiring_routes.go",
	} {
		t.Run(alvo, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, alvo, nil))
			if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "package devui") {
				t.Errorf("%s vazou código-fonte", alvo)
			}
		})
	}
}

// TestHandler_NaoDeixaCachear: a página muda junto do código que ela testa.
// Uma versão velha em cache faria alguém depurar um comportamento que já não
// existe — que é o pior modo de falha de uma ferramenta de diagnóstico.
func TestHandler_NaoDeixaCachear(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath, nil))
	if got := rec.Header().Get("Cache-Control"); got != cacheControl {
		t.Errorf("Cache-Control = %q, quero %q", got, cacheControl)
	}
}

// TestPaginaTrataOsDoisSchemasDeQR trava o que a página precisa saber sobre
// a API: o mesmo evento "QR" chega com dois formatos de payload (ver F68).
// Se alguém simplificar o HTML e tratar só um, o pareamento quebra num dos
// dois fluxos — e o teste que pegaria isso é este.
func TestPaginaTrataOsDoisSchemasDeQR(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath, nil))
	body := rec.Body.String()

	for _, marca := range []string{
		"qrCodeBase64", // schema do fluxo de pareamento
		"expiresAt",    // validade real do código atual
		"qrtimeout",    // fim da janela: exige novo /session/connect
		"session/ws",   // o WebSocket, um por sessão
	} {
		if !strings.Contains(body, marca) {
			t.Errorf("a página não menciona %q; o tratamento correspondente sumiu", marca)
		}
	}
}

// TestHandler_RedirectsPathWithoutTrailingSlash cobre a F94.
//
// `/devui` JÁ ERA rota registrada (wiring_routes.go), mas o handler devolvia
// 404: `strings.TrimPrefix(path, "/devui/")` não casa sem a barra, então o
// caminho virava "//devui" e o FileServer não achava nada.
//
// O 404 era indistinguível de "devui desligado" ou "instância caiu" — foi
// exatamente a hipótese levantada quando aconteceu, e custou uma rodada de
// diagnóstico para descobrir que faltava uma barra.
//
// Redirecionar, e não servir o índice ali, porque URL relativa dentro do HTML
// resolveria contra a raiz (`/app.js`) em vez de `/devui/app.js`.
func TestHandler_RedirectsPathWithoutTrailingSlash(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, strings.TrimSuffix(BasePath, "/"), nil)

	Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, quero %d (redirecionamento para a forma com barra)", rec.Code, http.StatusMovedPermanently)
	}
	if loc := rec.Header().Get("Location"); loc != BasePath {
		t.Errorf("Location = %q, quero %q", loc, BasePath)
	}
}

// TestHandler_ServesIndexOnBasePath: o redirecionamento acima só tem valor se o
// destino funcionar. Sem esta asserção, apontar o Location para um caminho
// quebrado passaria no teste anterior.
func TestHandler_ServesIndexOnBasePath(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, BasePath, nil)

	Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d para %s, quero 200", rec.Code, BasePath)
	}
	if rec.Body.Len() == 0 {
		t.Error("corpo vazio: o redirecionamento levaria a uma pagina em branco")
	}
}
