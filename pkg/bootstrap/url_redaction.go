package bootstrap

import "net/url"

// Redação do token na URL registrada em log (F75, ADR-0006).
//
// A query string continua sendo um caminho de autenticação VÁLIDO para
// `/session/ws`, e vai continuar: a API `WebSocket` do navegador não permite
// header customizado no handshake — é limitação da especificação, não do nosso
// código. Um painel web não tem outra forma de autenticar ali.
//
// O custo dessa exceção é que o token passa a viajar na URL daquela rota. E a
// URL é registrada em três lugares (o registro de fronteira, o observador de
// limite de taxa e o relatório de pânico), o que transformaria uma exceção
// justificada em credencial no log — que é justamente o defeito que acabamos de
// fechar em três outros sítios.
//
// Por isso a exceção só é honesta acompanhada desta redação.

// tokenQueryParam é o nome do parâmetro que carrega a credencial.
const tokenQueryParam = "token"

// redactedURLValue é o que aparece no lugar do token.
//
// Um marcador, e não a remoção do parâmetro: quem lê o log precisa saber que a
// requisição VEIO com token por query string — é o sinal de que aquele cliente
// ainda não migrou, e some junto com o valor se o parâmetro for apagado.
//
// Sem colchetes de propósito. `url.Values.Encode` percent-encoda o que não for
// seguro em URL, e `[REDACTED]` saía como `%5BREDACTED%5D` no log: a redação
// funcionava e o marcador ficava ilegível justamente para quem precisa lê-lo.
const redactedURLValue = "REDACTED"

// redactURL devolve a URL com o token mascarado, pronta para log.
//
// Recebe *url.URL e devolve string porque o chamador loga com `Str`, não com
// `Stringer`: um `Stringer` sobre a URL original seria avaliado LÁ DENTRO do
// zerolog, e a redação teria de acontecer antes — foi assim que os três sítios
// passaram a vazar sem ninguém notar.
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	if !u.Query().Has(tokenQueryParam) {
		// Caminho comum: nada a fazer, e nada a copiar. A imensa maioria das
		// requisições não usa query string para token, e esta função roda uma
		// vez por requisição registrada.
		return u.String()
	}

	// Cópia: mutar a URL da requisição afetaria o roteamento e qualquer
	// handler que a leia depois. O log não pode alterar o que observa.
	copia := *u
	q := copia.Query()
	q.Set(tokenQueryParam, redactedURLValue)
	copia.RawQuery = q.Encode()
	return copia.String()
}
