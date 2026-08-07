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
	"io/fs"
	"net/http"
	"os"
	"strings"
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
	indexFile = "qr.html"

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
// Não recebe a chain de autenticação de propósito: o que ele entrega é HTML
// e CSS estáticos, sem dado de usuário nenhum. O token é digitado na própria
// página e viaja nas chamadas que ELA faz à API, que aí sim passam pela
// chain. Exigir token para baixar o HTML impediria a página de existir antes
// de haver token.
func Handler() http.Handler {
	sub, err := fs.Sub(assets, assetDir)
	if err != nil {
		// Só acontece se o go:embed acima for alterado para um diretório
		// que não existe, o que quebra o build antes de chegar aqui.
		panic("devui: assets embutidos inacessíveis: " + err.Error())
	}
	files := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, BasePath)
		if name == "" {
			name = indexFile
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + name

		w.Header().Set("Cache-Control", cacheControl)
		files.ServeHTTP(w, r2)
	})
}
