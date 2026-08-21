package devui

import (
	"encoding/json"
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
	Handler(tokenDeTeste).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath, nil))

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
	Handler(tokenDeTeste).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+indexFile, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", rec.Code)
	}
}

// TestHandler_NaoInventaConteudo: pedir um arquivo que não existe tem de dar
// 404, e não cair no index. Cair no index faria um erro de digitação parecer
// sucesso.
func TestHandler_ArquivoInexistenteDa404(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(tokenDeTeste).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+"nao-existe.html", nil))
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
			Handler(tokenDeTeste).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, alvo, nil))
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
	Handler(tokenDeTeste).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath, nil))
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

	Handler(tokenDeTeste).ServeHTTP(rec, req)

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

	Handler(tokenDeTeste).ServeHTTP(rec, req)

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
	Handler(tokenDeTeste).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+nome, nil))
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
		`id="admin"`,      // o campo permanente de token no cabeçalho
		`id="nova-admin"`, // o token de admin pedido ao criar sessão
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
	if !strings.Contains(servido(t, "sessions.js"), "if (op.perigo) {") {
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
		Handler(tokenDeTeste).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+nome, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, quero 200", nome, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, prefixo) {
			t.Errorf("%s: Content-Type = %q, quero %s*", nome, ct, prefixo)
		}
	}
}

// tokenDeTeste é o valor que o Handler entrega em /devui/config nos testes.
// Não é um segredo — é uma marca reconhecível, para que uma asserção de
// "o token saiu" não possa passar por acidente com uma string vazia.
const tokenDeTeste = "admin-de-teste-1234"

// TestConfig_EntregaOTokenDeAdmin trava a decisão de 2026-08-20: o painel
// deixou de pedir o token de admin e passa a recebê-lo do servidor.
//
// A consequência está escrita no comentário de Handler e foi decidida pelo
// humano: esta camada é de desenvolvimento, e um segundo segredo a proteger
// uma ferramenta que só corre com WA_API_DEV_UI ligado protege pouco e
// atrapalha sempre.
func TestConfig_EntregaOTokenDeAdmin(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(tokenDeTeste).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, BasePath+"config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %sconfig = %d, quero 200", BasePath, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, quero application/json", ct)
	}

	var c struct {
		AdminToken string `json:"adminToken"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatalf("resposta não é JSON: %v (corpo: %s)", err, rec.Body.String())
	}
	// O valor EXATO, e não "não vazio": um handler que devolvesse uma
	// constante qualquer passaria numa asserção de presença.
	if c.AdminToken != tokenDeTeste {
		t.Errorf("adminToken = %q, quero %q", c.AdminToken, tokenDeTeste)
	}
}

// TestConfig_NaoVazaEmOutraRota: o token só sai por /devui/config. Se o
// FileServer o servisse por engano noutro caminho, um ficheiro estático
// passaria a conter uma credencial.
func TestConfig_NaoVazaEmOutraRota(t *testing.T) {
	for _, nome := range []string{"sessions.html", "eventos.html", "devui.js", "sessions.js"} {
		if strings.Contains(servido(t, nome), tokenDeTeste) {
			t.Errorf("%s contém o token de admin: ele só deve sair por %sconfig", nome, BasePath)
		}
	}
}

// TestPainel_GeraOTokenDaSessao trava o formato pedido: wa_noise_ mais um
// valor de crypto.getRandomValues.
//
// A asserção inclui getRandomValues de propósito. Um gerador que usasse
// Math.random produziria um token com o prefixo certo e entropia previsível —
// passaria num teste de formato e falharia no que interessa.
func TestPainel_GeraOTokenDaSessao(t *testing.T) {
	js := servido(t, "devui.js")

	if !strings.Contains(js, `"wa_noise_"`) {
		t.Error("o gerador não usa o prefixo wa_noise_")
	}
	if !strings.Contains(js, "crypto.getRandomValues") {
		t.Error("o token não vem de crypto.getRandomValues: Math.random é previsível por desenho")
	}
	// A procura é pela CHAMADA `Math.random(`, e não pelo nome solto.
	//
	// A primeira versão procurava "Math.random" e falhou — por causa do
	// comentário que explica POR QUE não se usa Math.random. É exatamente a
	// HOUSEKEEP F189, que eu tinha acabado de corrigir do lado do logcov: um
	// verificador que conta MENÇÕES em vez de USOS pune quem documenta o
	// mecanismo, e a saída fácil é apagar a explicação.
	if strings.Contains(js, "Math.random(") {
		t.Error("Math.random() é chamado no módulo: não serve para gerar credencial")
	}

	// E o campo tem de ser SÓ DE LEITURA: um token gerado que o utilizador
	// possa reescrever à mão volta a ser um token escolhido por pessoa.
	if !strings.Contains(servido(t, "sessions.html"), `id="nova-token" autocomplete="off" spellcheck="false" readonly`) {
		t.Error("o campo do token não é readonly")
	}
}

// TestDeLoopback aceita as duas formas de endereço, e recusa o resto.
//
// A tabela inclui um endereço COM porta e outro SEM porque `RemoteAddr` traz
// porta e um endereço vindo de outra camada pode não trazer — testar só uma
// forma deixaria metade da função sem exercício.
func TestDeLoopback(t *testing.T) {
	for entrada, quero := range map[string]bool{
		"127.0.0.1:54321": true,
		"127.0.0.1":       true,
		"[::1]:8080":      true,
		"::1":             true,
		"192.168.1.10:80": false,
		"10.0.0.5":        false,
		"":                false,
		"nao-e-um-ip":     false,
	} {
		if got := deLoopback(entrada); got != quero {
			t.Errorf("deLoopback(%q) = %v, quero %v", entrada, got, quero)
		}
	}
}

// --- remoção de sessões ------------------------------------------------------

// TestPainel_RemoverOfereceAsDuasRotas é a trava que mais importa deste bloco.
//
// A API tem DUAS remoções, e elas não são equivalentes:
//
//	DELETE /admin/users/{id}       apaga o utilizador; o TELEMÓVEL FICA PAREADO
//	                               a uma sessão que já não existe
//	DELETE /admin/users/{id}/full  faz logout antes, e o telemóvel liberta o
//	                               aparelho
//
// Um painel que ofereça só a primeira deixa aparelhos-fantasma no telemóvel de
// alguém, e "remover" é a palavra que faz esperar o contrário. Um que ofereça
// só a segunda gasta uma ida ao protocolo para sessões que nunca parearam.
//
// Sem este teste, "simplificar para uma rota só" parece limpeza.
func TestPainel_RemoverOfereceAsDuasRotas(t *testing.T) {
	js := servido(t, "sessions.js")

	if !strings.Contains(js, `${completo ? "/full" : ""}`) {
		t.Error("a remoção não distingue as duas rotas: uma delas deixa o aparelho pareado a uma sessão inexistente")
	}
	if !strings.Contains(js, `API.admin("DELETE"`) {
		t.Error("a remoção não usa o caminho de admin: sessões sem token local ficariam sem forma de ser removidas")
	}

	html := servido(t, "sessions.html")
	for _, id := range []string{`id="rem-simples"`, `id="rem-completo"`} {
		if !strings.Contains(html, id) {
			t.Errorf("o diálogo de remoção não oferece %s", id)
		}
	}
}

// TestPainel_RemoverFuncionaSemTokenLocal trava o caso que motivou o pedido:
// apagar uma sessão criada NOUTRO navegador.
//
// Todos os outros botões do card exigem o token da sessão. Este não pode
// exigir, porque a credencial que ele usa é a de admin — e é justamente a
// ausência do token local que torna a sessão impossível de gerir de outra
// forma.
func TestPainel_RemoverFuncionaSemTokenLocal(t *testing.T) {
	js := servido(t, "sessions.js")

	if !strings.Contains(js, `b.disabled = !s.temToken && b.dataset.a !== "remover"`) {
		t.Error("o botão Remover é desativado junto com os outros quando não há token local; " +
			"uma sessão criada noutro navegador ficaria sem forma de ser removida pelo painel")
	}
	if !strings.Contains(js, `if (!s.temToken && qual !== "remover") return;`) {
		t.Error("a guarda de token bloqueia a remoção antes de ela chegar a acontecer")
	}
}

// TestPainel_RemoverEmLoteEscolheARotaPorSessao: numa limpeza em lote, cada
// sessão vai pelo caminho certo. Usar sempre o simples deixaria aparelhos
// pareados a nada; usar sempre o completo gastaria uma ida ao protocolo por
// sessão que nunca pareou.
func TestPainel_RemoverEmLoteEscolheARotaPorSessao(t *testing.T) {
	js := servido(t, "sessions.js")
	if !strings.Contains(js, `${s.autenticado ? "/full" : ""}`) {
		t.Error("a limpeza em lote não escolhe a rota por sessão")
	}
}

// TestPainel_TokenLocalSoEsquecidoDepoisDaAPI: se a remoção falhar e o token
// já tiver sido esquecido, a sessão fica na lista sem forma de ser operada —
// trocando um problema por outro pior.
func TestPainel_TokenLocalSoEsquecidoDepoisDaAPI(t *testing.T) {
	js := servido(t, "sessions.js")
	i := strings.Index(js, "async function remover(")
	if i < 0 {
		t.Fatal("função remover ausente")
	}
	corpo := js[i:]
	fim := strings.Index(corpo, "\n}\n")
	if fim > 0 {
		corpo = corpo[:fim]
	}
	posErro := strings.Index(corpo, "if (!r.ok)")
	posEsquecer := strings.Index(corpo, "Tokens.esquecer")
	if posErro < 0 || posEsquecer < 0 {
		t.Fatal("remover não trata o erro ou não esquece o token")
	}
	if posEsquecer < posErro {
		t.Error("o token local é esquecido ANTES de a API confirmar: uma remoção falhada " +
			"deixaria a sessão na lista e sem credencial para a operar")
	}
}

// TestPainel_NaoUsaConfirmNativo trava o defeito reportado em campo:
// "tentei remover todos e não deu certo".
//
// O confirm() nativo devolve `false` EM SILÊNCIO quando o navegador suprime
// diálogos — o Chrome oferece "impedir que esta página crie mais diálogos"
// depois de alguns seguidos, e a partir daí TODA ação protegida por ele deixa
// de acontecer sem dizer porquê. Num painel de diagnóstico isso é pior que
// noutro sítio qualquer: o operador conclui que a API está partida.
//
// A verificação é pela CHAMADA `confirm(` precedida do que a distingue de uma
// menção — o módulo fala sobre confirm() em três comentários, e procurar o nome
// solto acusaria a própria explicação. É a HOUSEKEEP F189 outra vez, e é a
// terceira vez hoje que ela aparece num teste meu.
func TestPainel_NaoUsaConfirmNativo(t *testing.T) {
	js := servido(t, "sessions.js")

	for _, chamada := range []string{
		"= confirm(", // const ok = confirm(...)
		"!confirm(",  // if (!confirm(...))
		"window.confirm(",
	} {
		if strings.Contains(js, chamada) {
			t.Errorf("o painel voltou a usar confirm() nativo (%q): ele falha em SILÊNCIO "+
				"quando o navegador suprime diálogos, e a ação parece simplesmente não acontecer", chamada)
		}
	}

	// E o substituto tem de existir: sem ele, "não usa confirm" seria
	// satisfeito por não confirmar nada, que é pior.
	if !strings.Contains(js, "function confirmar({") {
		t.Error("não há substituto para o confirm(): uma operação irreversível ficaria sem confirmação nenhuma")
	}
	if !strings.Contains(servido(t, "sessions.html"), `id="dlg-confirma"`) {
		t.Error("o diálogo de confirmação não existe no HTML")
	}
}

// TestPainel_LoteNaoMarcaPareadasPorOmissao é a trava do acidente que eu
// própria tive ao diagnosticar o defeito acima.
//
// "Remover as sem token" inclui sessões PAREADAS e a funcionar, porque "sem
// token" quer dizer "este navegador não tem a credencial" e não "está morta".
// Com as caixas marcadas por omissão, um clique desvincula telemóveis que
// estavam a trabalhar — foi o que fiz, e custou o pareamento de duas contas
// reais.
func TestPainel_LoteNaoMarcaPareadasPorOmissao(t *testing.T) {
	js := servido(t, "sessions.js")

	if !strings.Contains(js, "cb.checked = !s.autenticado;") {
		t.Error("a lista em lote marca sessões pareadas por omissão: um clique desvincula " +
			"telemóveis que estavam a funcionar")
	}
	if !strings.Contains(js, `id="lote-lista"`) && !strings.Contains(servido(t, "sessions.html"), `id="lote-lista"`) {
		t.Error("o lote não mostra QUAIS sessões vão ser removidas antes de as remover")
	}
}

// TestPainel_LoteRelataFalhasParciais: numa remoção de várias, uma falha no
// meio não pode passar despercebida só porque as outras correram bem.
func TestPainel_LoteRelataFalhasParciais(t *testing.T) {
	js := servido(t, "sessions.js")
	if !strings.Contains(js, "falhas.push(") || !strings.Contains(js, "Removidas ${marcados.length - falhas.length}") {
		t.Error("o lote não relata falhas parciais: sessões que não foram removidas ficariam invisíveis")
	}
}

// --- F192: "Gerar novo QR" não podia depender só do WebSocket --------------
//
// Medido em 2026-08-20 contra o servidor real, com o socket a entrar 3s
// depois do `connect`:
//
//	0.433s HTTP connect(sem-ws)  -> 200 {"status":"connecting"}
//	3.461s HTTP qr(depois-de-3s) -> len=1870   <- o QR EXISTE no banco
//	3.481s WS(tardio) OPEN
//	21.366s WS(tardio) MSG type=QR             <- 17,9s de silêncio
//
// O QR emitido a ~1,4s foi despachado para ZERO conexões e desapareceu. O
// painel só tinha essa entrada: sem evento, ficava em "Pedindo QR à API…"
// sem prazo e sem erro. As três travas abaixo cobrem as três metades da
// correção — esperar o socket, sondar a rota autoritativa, e falhar alto.

// TestPainel_ConectarEsperaOSocketAntesDePedirQR trava a ORDEM.
//
// Inverter as duas linhas — disparar `connect` e só depois abrir o socket —
// reabre exatamente a corrida medida acima, e passaria em qualquer teste que
// só verificasse que ambas as chamadas existem.
func TestPainel_ConectarEsperaOSocketAntesDePedirQR(t *testing.T) {
	js := servido(t, "sessions.js")

	// Casa a FORMA DA CHAMADA, nunca o nome nu: procurar "abrirWS" casaria o
	// comentário que explica a função, e procurar "/session/connect" casaria
	// a declaração da constante. É a armadilha F189, que já custou três
	// sessões a este repositório.
	const abertura = "await abrirWS(s);"
	const pedido = `await API.sessao(s.token, "GET", ROTA_CONNECT);`

	iAbertura := strings.Index(js, abertura)
	iPedido := strings.Index(js, pedido)
	if iAbertura < 0 {
		t.Fatalf("o painel não espera o socket abrir: %q não aparece em sessions.js", abertura)
	}
	if iPedido < 0 {
		t.Fatalf("o painel não pede a conexão: %q não aparece em sessions.js", pedido)
	}
	if iAbertura >= iPedido {
		t.Errorf("o `connect` (offset %d) sai ANTES de o socket abrir (offset %d): "+
			"o QR despachado nessa janela é entregue a zero conexões e perde-se", iPedido, iAbertura)
	}

	// E a espera tem de ser real: uma `abrirWS` que devolva undefined faz o
	// `await` acima resolver no mesmo tick e a ordem volta a não valer nada.
	if !strings.Contains(js, "return new Promise((resolve) => {") ||
		!strings.Contains(js, `ws.addEventListener("open"`) {
		t.Error("abrirWS não resolve no evento `open` do socket: o `await` seria decorativo")
	}
}

// TestPainel_ConectarSondaARotaDeQR: o WebSocket é um canal COM PERDA, e
// `users.qrcode` — servido por GET /session/qr — é a fonte durável do mesmo
// código. Sem esta sondagem, todo evento perdido é um cartão preso para
// sempre.
//
// É o que a Evolution API faz em connectToWhatsapp: depois de conectar, ela
// LÊ o QR guardado em vez de confiar só no evento.
func TestPainel_ConectarSondaARotaDeQR(t *testing.T) {
	js := servido(t, "sessions.js")

	if !strings.Contains(js, `const ROTA_QR = "/session/qr";`) {
		t.Fatal("a rota de QR não está declarada como constante em sessions.js")
	}
	// Outra vez a forma da CHAMADA: "ROTA_QR" sozinho casa os comentários que
	// explicam a sondagem, e "/session/qr" casa a própria declaração.
	if !strings.Contains(js, `await API.sessao(s.token, "GET", ROTA_QR);`) {
		t.Error("o painel nunca busca o QR pela rota REST: um evento perdido no WebSocket " +
			"deixa o cartão preso em \"Pedindo QR à API…\" sem prazo")
	}
	if !strings.Contains(js, "aguardarQR(s);") {
		t.Error("a sondagem existe mas ninguém a dispara depois do connect")
	}
}

// TestPainel_FalhaDeQRNaoFicaEmEspera: um pedido que não produz QR nenhum
// dentro da janela falhou, e o painel tem de o dizer. Espera sem fim é
// indistinguível de painel partido — foi assim que este defeito chegou até
// aqui.
func TestPainel_FalhaDeQRNaoFicaEmEspera(t *testing.T) {
	js := servido(t, "sessions.js")

	if !strings.Contains(js, "falhouQR(s);") {
		t.Error("a janela de espera do QR fecha sem chamar falhouQR: o cartão fica em " +
			"\"Pedindo QR à API…\" indefinidamente, que é o sintoma original")
	}
	// A saída tem de ser accionável, não só um texto de erro.
	if !strings.Contains(js, "Tentar de novo") {
		t.Error("o estado de falha não oferece nova tentativa")
	}
}
