package bootstrap

import (
	"encoding/base64"

	"github.com/rs/zerolog/log"
)

// Chaves da entrada do UserInfoCache que a configuração de HMAC toca.
//
// As mesmas strings são escritas por ensureUserInfoCached (user_info_cache.go)
// e por connectOnStartup (lifecycle.go), e "HmacKeyEncrypted" é lida por
// sendToUserWebHook (lifecycle_webhook.go:178) para assinar cada webhook por
// usuário. São constantes por isso: um literal divergente aqui produziria uma
// revogação silenciosamente inerte.
const (
	userInfoHmacKeyField = "HmacKeyEncrypted"
	userInfoHasHmacField = "HasHmac"

	userInfoHasHmacTrue  = "true"
	userInfoHasHmacFalse = "false"
)

// hmacKeyEncryptor implements appport.HmacKeyEncryptor sobre a chave de
// encriptação global do processo.
type hmacKeyEncryptor struct{}

// EncryptHmacKey delega para o mesmo AES-GCM que o resto do processo usa
// (encryptHMACKey em dispatch_webhook.go, que lê appCtx.GlobalEncryptionKey).
func (hmacKeyEncryptor) EncryptHmacKey(plainKey string) ([]byte, error) {
	return encryptHMACKey(plainKey)
}

// userInfoHmacCache implements appport.UserInfoHmacCache sobre
// appCtx.UserInfoCache.
type userInfoHmacCache struct{}

// SetHmacKey republica a entrada do usuário com a chave nova, ou sem chave
// nenhuma quando encryptedKey é vazia.
//
// Ausência de entrada é no-op DELIBERADO: quem não está no cache carrega do
// banco no próximo miss (ensureUserInfoCached), então não há valor obsoleto a
// corrigir. Criar a entrada aqui exigiria as outras onze colunas de
// userInfoColumns, e uma entrada parcial é pior que nenhuma — foi o defeito
// da F70.
//
// O Set é sob cache.NoExpiration porque é assim que TODAS as outras escritas
// desta entrada o fazem; um TTL aqui faria a chave sumir do cache sem sumir do
// banco, e o webhook do usuário pararia de ser assinado sem ninguém ter pedido.
func (userInfoHmacCache) SetHmacKey(userID string, encryptedKey []byte) {
	cached, found := appCtx.UserInfoCache.Get(userID)
	if !found {
		log.Debug().Str("userid", userID).
			Msg("no cached user info while updating the HMAC key; the next miss loads it from the database")
		return
	}
	values, ok := cached.(Values)
	if !ok {
		log.Error().Str("userid", userID).
			Msg("cached user info has an unexpected type; HMAC key not published to the cache")
		return
	}

	encoded := ""
	hasHmac := userInfoHasHmacFalse
	if len(encryptedKey) > 0 {
		encoded = base64.StdEncoding.EncodeToString(encryptedKey)
		hasHmac = userInfoHasHmacTrue
	}

	updated := Values{M: make(map[string]string, len(values.M)+2)}
	for k, v := range values.M {
		updated.M[k] = v
	}
	updated.M[userInfoHmacKeyField] = encoded
	updated.M[userInfoHasHmacField] = hasHmac

	publishUserInfo(userID, "", updated)
	log.Info().Str("userid", userID).Bool("has_hmac", len(encryptedKey) > 0).
		Msg("user info cache updated with the HMAC configuration")
}
