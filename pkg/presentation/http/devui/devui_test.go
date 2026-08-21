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
	body := pacoteServido(t)

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
	body := pacoteServido(t)
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

// servido devolve o conteúdo de um ficheiro embutido, pelo handler.
func servido(t *testing.T, nome string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+nome, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s%s = %d, quero 200", BasePath, nome, rec.Code)
	}
	return rec.Body.String()
}

// pacoteServido concatena TUDO o que o devui serve.
//
// Os testes abaixo verificam o que o painel OFERECE, e isso deixou de viver
// num ficheiro só: em 2026-08-20 o JavaScript saiu do <script> embutido para
// módulos (devui.js, operacoes.js, sessions.js, eventos.js) e a folha de
// estilo para devui.css. Procurar as marcas apenas no HTML passaria a acusar
// ausência de comportamento que existe — foi o que aconteceu, e é por isso que
// esta função existe.
//
// A lista é escrita à mão e não derivada do embed: derivá-la faria um ficheiro
// novo entrar na verificação sem ninguém decidir que ele devia entrar, e um
// ficheiro esquecido deixaria de ser detetável.
func pacoteServido(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, f := range []string{
		"sessions.html", "eventos.html",
		"devui.css", "devui.js", "operacoes.js", "sessions.js", "eventos.js",
	} {
		b.WriteString(servido(t, f))
		b.WriteString("\n")
	}
	return b.String()
}

// --- 2026-08-20: o que a reformulação do painel garantiu -------------------

// TestPainel_NaoTemLoginNemAdicionarExistente trava as duas coisas que saíram.
//
// O painel tinha um campo de token de admin PERMANENTE no cabeçalho, que o
// fazia parecer um ecrã de login, e um botão "adicionar existente" que existia
// só porque a listagem vinha do localStorage em vez da API.
//
// Sem esta trava, qualquer um dos dois volta na primeira vez que alguém
// precisar de operar uma sessão cujo token se perdeu — e a solução certa para
// isso é criar uma sessão nova, não reintroduzir o campo.
func TestPainel_NaoTemLoginNemAdicionarExistente(t *testing.T) {
	html := servido(t, "sessions.html")

	for _, proibido := range []string{
		"Adicionar existente",
		`id="admin"`, // o campo permanente de token no cabeçalho
	} {
		if strings.Contains(html, proibido) {
			t.Errorf("o painel voltou a ter %q: a listagem vem da API e o token de admin "+
				"é pedido só ao criar sessão", proibido)
		}
	}
}

// TestPainel_ListagemVemDaAPI: a grelha tem de ser construída a partir de
// GET /admin/users, e não do que estiver guardado no navegador.
//
// É a diferença entre "recupera sozinha" e "mostra o que alguém digitou aqui".
func TestPainel_ListagemVemDaAPI(t *testing.T) {
	js := servido(t, "devui.js") + servido(t, "sessions.js")

	if !strings.Contains(js, `API.admin("GET", "/admin/users")`) {
		t.Error("a listagem não chama GET /admin/users: ela deixaria de recuperar sozinha")
	}
	// A chave do armazenamento local é o ID do utilizador. Chavear pelo token
	// faria a lista ser o que está no navegador — o desenho anterior.
	if !strings.Contains(js, "Tokens.de(u.id)") {
		t.Error("os tokens não são associados por id de utilizador; a lista voltaria a depender do armazenamento local")
	}
}

// TestPainel_OperacoesDeEnvioEChatNoCard trava o catálogo.
//
// Uma rota que exista na API e não apareça aqui é invisível para quem usa o
// painel — e foi assim que a HOUSEKEEP F190 nasceu do lado do servidor: três
// rotas ficaram de fora de uma trava porque ninguém as enumerou.
func TestPainel_OperacoesDeEnvioEChatNoCard(t *testing.T) {
	ops := servido(t, "operacoes.js")

	for _, rota := range []string{
		"/chat/send/text", "/chat/send/image", "/chat/send/video", "/chat/send/audio",
		"/chat/send/document", "/chat/send/sticker", "/chat/send/location",
		"/chat/send/contact", "/chat/send/poll", "/chat/send/buttons",
		"/chat/send/template", "/chat/send/list", "/chat/send/edit",
		"/chat/list", "/chat/history", "/chat/react", "/chat/markread",
		"/chat/presence", "/chat/delete/message",
	} {
		// A busca é pela forma CITADA (`"/chat/send/edit"`), e não pela rota
		// solta. Com a rota solta, `strings.Contains` casa também um
		// `/chat/send/edit_QUALQUERCOISA` — o controlo negativo CN-48 renomeou
		// exatamente assim e o teste passou. Uma rota estragada continuava a
		// "estar no catálogo" por ser prefixo dela própria.
		if !strings.Contains(ops, `"`+rota+`"`) {
			t.Errorf("a rota %q não está no catálogo do painel", rota)
		}
	}
}

// TestPainel_EventosEmRotaPropria: os eventos saíram do painel global.
//
// A asserção é nos DOIS sentidos — a página existe E a de sessões deixou de
// carregar o fluxo — porque só a primeira metade deixaria passar uma duplicação
// silenciosa, com as duas páginas a abrir sockets para as mesmas sessões.
func TestPainel_EventosEmRotaPropria(t *testing.T) {
	if !strings.Contains(servido(t, "eventos.html"), "eventos.js") {
		t.Error("eventos.html não carrega o seu módulo")
	}
	if !strings.Contains(servido(t, "sessions.html"), `href="eventos.html"`) {
		t.Error("a página de sessões não liga para os eventos: a rota nova ficaria inalcançável")
	}
	if strings.Contains(servido(t, "sessions.html"), `id="log"`) {
		t.Error("o painel de eventos voltou para a página de sessões")
	}
}

// TestPainel_ApagarMensagemPedeConfirmacao: é a única operação irreversível do
// catálogo, e o painel não pode executá-la a um clique de distância.
func TestPainel_ApagarMensagemPedeConfirmacao(t *testing.T) {
	if !strings.Contains(servido(t, "operacoes.js"), "perigo: true") {
		t.Error("nenhuma operação está marcada como perigosa; apagar mensagem é IRREVERSÍVEL")
	}
	if !strings.Contains(servido(t, "sessions.js"), "op.perigo && !confirm(") {
		t.Error("a marca de perigo não produz confirmação antes de executar")
	}
}

// TestHandler_ServeOsModulos: um .js servido com o Content-Type errado é
// recusado por `<script type="module">` e a página fica muda — sem erro
// visível na rede, que é o pior modo de falha para diagnosticar.
func TestHandler_ServeOsModulos(t *testing.T) {
	for nome, prefixo := range map[string]string{
		"devui.js":  "text/javascript",
		"devui.css": "text/css",
	} {
		rec := httptest.NewRecorder()
		Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+nome, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, quero 200", nome, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, prefixo) {
			t.Errorf("%s: Content-Type = %q, quero %s*", nome, ct, prefixo)
		}
	}
}
