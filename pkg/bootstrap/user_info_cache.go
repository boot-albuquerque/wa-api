package bootstrap

import (
	"encoding/base64"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

// userInfoColumns é a lista de colunas que compõem uma entrada do
// UserInfoCache.
//
// Constante compartilhada, e não a mesma string copiada em dois lugares:
// connectOnStartup e ensureUserInfoCached preenchem a MESMA estrutura, e
// divergir as colunas faria uma entrada nascer incompleta dependendo do
// caminho por onde o usuário passou — o tipo de defeito que só aparece em
// produção, num dos dois fluxos.
// historyDaysQuery lê quantos dias de histórico sincronizar após o
// pareamento.
//
// Constante, e não string inline no handler, para que o teste possa executar
// A MESMA query contra o schema real. Era esse o buraco da F71: a query
// pedia `days_to_sync_history`, coluna inexistente, e nenhum teste a
// exercitava — falhava em produção como um Warn silencioso.
const historyDaysQuery = "SELECT COALESCE(history, 0) FROM users WHERE id=$1"

const userInfoColumns = `id,name,token,jid,webhook,events,proxy_url,` +
	`CASE WHEN s3_enabled THEN 'true' ELSE 'false' END AS s3_enabled,` +
	`media_delivery,COALESCE(history, 0) as history,hmac_key`

// ensureUserInfoCached garante que exista entrada no UserInfoCache para
// token, carregando do banco quando não houver.
//
// # Por que isto precisa existir (F70 em HOUSEKEEP.md)
//
// Até a F70, o único ponto que CRIAVA entrada era connectOnStartup, que roda
// na subida do servidor iterando `WHERE connected=1`. Um usuário criado por
// POST /admin/users depois disso — o caminho normal de onboarding — nunca era
// cacheado. E o `Set` do PairSuccess está dentro do `else` de um
// `if !found`: sem entrada prévia ele apenas logava e não criava nada.
//
// Como getUserWebhookUrl lê SÓ do cache e devolve "" no miss, o resultado era
// um webhook configurado na tabela `users` ser silenciosamente ignorado até o
// processo reiniciar.
//
// # Cache-aside, não remendo
//
// A leitura do banco no miss é o padrão cache-aside: a fonte de verdade é a
// tabela, o cache é aceleração. O que havia antes era um cache tratado como
// fonte de verdade — populado uma vez e nunca reconciliado.
//
// Idempotente: com a entrada presente, não toca no banco. É chamada no
// caminho de Attach, que roda uma vez por sessão, não por evento.
func ensureUserInfoCached(db *sqlx.DB, userID string) error {
	if _, found := appCtx.UserInfoCache.Get(userID); found {
		return nil
	}

	query := db.Rebind(`SELECT ` + userInfoColumns + ` FROM users WHERE id=?`)

	var (
		txtid, name, dbToken, jid string
		webhook, events, proxyURL string
		s3Enabled, mediaDelivery  string
		history                   int
		hmacKey                   []byte
	)
	if err := db.QueryRowx(query, userID).Scan(
		&txtid, &name, &dbToken, &jid, &webhook, &events, &proxyURL,
		&s3Enabled, &mediaDelivery, &history, &hmacKey,
	); err != nil {
		return fmt.Errorf("ensureUserInfoCached: user %s: %w", userID, err)
	}

	hmacKeyEncrypted := ""
	if len(hmacKey) > 0 {
		hmacKeyEncrypted = base64.StdEncoding.EncodeToString(hmacKey)
	}

	// A chave é o token vindo do BANCO, não o recebido por parâmetro: é o
	// token que os leitores usam (evh.Token vem da mesma origem), e gravar
	// sob outra chave criaria uma entrada que ninguém encontra.
	appCtx.UserInfoCache.Set(userID, Values{M: map[string]string{
		"Id":               txtid,
		"Name":             name,
		"Jid":              jid,
		"Webhook":          webhook,
		"Token":            dbToken,
		"Proxy":            proxyURL,
		"Events":           events,
		"S3Enabled":        s3Enabled,
		"MediaDelivery":    mediaDelivery,
		"History":          fmt.Sprintf("%d", history),
		"HmacKeyEncrypted": hmacKeyEncrypted,
	}}, cache.NoExpiration)

	log.Info().Str("userid", txtid).Msg("User info carregado do banco para o cache")
	return nil
}
