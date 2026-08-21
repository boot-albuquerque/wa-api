// Package devui serve as páginas HTML de teste manual do wa-api.
//
// # Por que isto vive na camada de apresentação
//
// São páginas servidas por HTTP, e é isso que esta camada faz. A alternativa
// — deixá-las soltas num diretório e abrir via file:// — não funciona: a
// wa-api não tem middleware de CORS, então uma página de outra origem não
// consegue nem enviar o fetch. Servidas daqui, elas ficam na MESMA ORIGEM da
// API e o problema desaparece sem que seja preciso afrouxar CORS em produção
// por causa de ferramenta de desenvolvimento.
//
// # Por que são embutidas no binário
//
// go:embed em vez de leitura de disco: o binário continua sendo um artefato
// único, e a página não pode divergir da versão da API que a serve — que é
// exatamente o risco de uma cópia solta em /tmp.
//
// # Exposição
//
// As rotas daqui NÃO são registradas por padrão. Ver EnvEnabled: sem a
// variável de ambiente, o pacote não é montado no router. Uma página de
// diagnóstico exposta em produção conta a qualquer visitante quais endpoints
// existem e como autenticar neles.
package devui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

//go:embed assets
var assets embed.FS

const (
	// EnvEnabled liga as páginas de teste. Sem ela, Enabled() é falso e
	// Register não monta rota nenhuma.
	//
	// Opt-in explícito, e não opt-out: quem esquecer de configurar fica
	// seguro, que é a direção certa do erro.
	EnvEnabled = "WA_API_DEV_UI"

	// BasePath é o prefixo sob o qual as páginas são servidas.
	BasePath = "/devui/"

	// assetDir é o diretório embutido; o prefixo é retirado das URLs.
	assetDir = "assets"

	// indexFile é servido quando a URL aponta para o próprio BasePath.
	indexFile = "sessions.html"

	// configPath entrega ao painel o que ele não tem como saber sozinho.
	//
	// Hoje é só o token de admin. Ver Handler para a consequência de
	// segurança, que NÃO é pequena e está escrita lá.
	configPath = "config"

	// cacheControl desliga o cache. Estas páginas mudam junto do código que
	// elas testam, e uma versão velha em cache faria alguém depurar um
	// comportamento que já não existe.
	cacheControl = "no-store"
)

// Enabled informa se as páginas de teste devem ser montadas.
//
// Aceita os mesmos valores que o resto do projeto trata como verdadeiro em
// variável de ambiente, para não criar uma convenção nova só aqui.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvEnabled))) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

// Handler devolve o http.Handler que serve as páginas embutidas.
//
// `adminToken` é entregue ao painel em GET /devui/config, e isso MUDA A
// FRONTEIRA DE SEGURANÇA desta ferramenta. Vale escrever com todas as letras,
// porque quem ler o código a seguir tem de poder discordar com conhecimento:
//
//   - ANTES: alcançar o devui dava o HTML e mais nada. Para criar utilizadores
//     ou listar sessões era preciso apresentar o token de admin, que a página
//     pedia a quem a usasse.
//   - AGORA: alcançar o devui dá acesso administrativo completo, porque o
//     token vem de graça.
//
// A decisão foi do humano, explícita, e o raciocínio é defensável: o devui só
// existe com WA_API_DEV_UI ligado, que já é a declaração "esta instância é de
// desenvolvimento". Um segundo segredo a proteger uma ferramenta que só corre
// em modo de desenvolvimento protege pouco e atrapalha sempre.
//
// O que NÃO muda, e é o que continua a segurar isto: sem a variável de
// ambiente, nem o painel nem este endpoint existem — Register não monta rota
// nenhuma. A proteção real sempre foi essa, e continua a ser.
//
// Não recebe a chain de autenticação de propósito: o que ele entrega é HTML e
// CSS estáticos. Exigir token para baixar o HTML impediria a página de existir
// antes de haver token.
func Handler(adminToken string) http.Handler {
	sub, err := fs.Sub(assets, assetDir)
	if err != nil {
		// Só acontece se o go:embed acima for alterado para um diretório
		// que não existe, o que quebra o build antes de chegar aqui.
		panic("devui: assets embutidos inacessíveis: " + err.Error())
	}
	files := http.FileServer(http.FS(sub))

	basePathNoSlash := strings.TrimSuffix(BasePath, "/")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// `/devui` (sem barra) É uma rota registrada, mas o TrimPrefix abaixo
		// não casa com ela — BasePath termina em "/", então `name` ficaria
		// "/devui", o caminho viraria "//devui" e o FileServer devolveria 404.
		//
		// Redireciona em vez de servir o índice aqui: servido em `/devui`,
		// qualquer URL relativa dentro do HTML resolveria contra a RAIZ
		// (`/app.js`) em vez de `/devui/app.js`. É por isso que servidores
		// redirecionam diretório em vez de servir os dois caminhos.
		//
		// F94: o 404 é indistinguível de "devui desligado" ou "instância
		// caiu", e custou uma rodada de diagnóstico.
		if r.URL.Path == basePathNoSlash {
			http.Redirect(w, r, BasePath, http.StatusMovedPermanently)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, BasePath)
		if name == "" {
			name = indexFile
		}

		if name == configPath {
			// WARN e não recusa: bloquear origens não-locais partiria quem
			// corre a API num contentor e abre o painel do hospedeiro, que é
			// uso normal. O aviso deixa rasto de que um segredo de
			// desenvolvimento saiu para fora da máquina, que é a informação
			// que alguém a investigar precisaria.
			if !deLoopback(r.RemoteAddr) {
				log.Warn().
					Str("component", "devui.config").
					Str("remote", r.RemoteAddr).
					Msg("token de admin servido a origem nao-local")
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", cacheControl)
			_ = json.NewEncoder(w).Encode(map[string]string{"adminToken": adminToken})
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + name

		w.Header().Set("Cache-Control", cacheControl)
		files.ServeHTTP(w, r2)
	})
}

// deLoopback informa se o pedido veio da própria máquina.
//
// Aceita `1.2.3.4:5678` e `1.2.3.4` porque `RemoteAddr` traz porta e um
// endereço vindo de outra camada pode não trazer. Testar as duas formas é mais
// simples do que decidir qual delas se recebeu.
//
// A primeira versão fazia `if err != nil { host = remoteAddr }`, e o gate de
// cobertura de log acusou — com razão: era um erro engolido sem registo. Mas um
// log ali seria ruído, porque "endereço sem porta" é formato normal e não
// falha. A saída foi tirar o erro do caminho em vez de o documentar.
func deLoopback(remoteAddr string) bool {
	semPorta, _, _ := net.SplitHostPort(remoteAddr)
	return ehLoopback(semPorta) || ehLoopback(remoteAddr)
}

// ehLoopback aceita só o que é IP de loopback. Endereço que não parseia conta
// como NÃO-loopback: na dúvida, avisar a mais é melhor do que calar um caso
// real.
func ehLoopback(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.IsLoopback()
}
