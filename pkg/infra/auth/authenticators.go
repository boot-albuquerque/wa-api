// Package auth contém lógica de autenticação e autorização extraída de handlers.go.
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

// ValidateAdminToken compara o token recebido com o admin token via constant-time compare.
// Retorna true se autorizado.
func ValidateAdminToken(authHeader, adminToken string) bool {
	tokenHash := sha256.Sum256([]byte(authHeader))
	adminHash := sha256.Sum256([]byte(adminToken))
	return subtle.ConstantTimeCompare(tokenHash[:], adminHash[:]) == 1
}

// ExtractToken extrai o token do header ou query parameter.
func ExtractToken(r *http.Request) string {
	token := r.Header.Get("token")
	if token == "" {
		token = strings.Join(r.URL.Query()["token"], "")
	}
	return token
}

// UserRecord contém os dados do usuário obtidos do DB.
type UserRecord struct {
	ID            string
	Name          string
	Webhook       string
	JID           string
	Events        string
	ProxyURL      string
	QRCode        string
	History       string
	HasHmac       bool
	S3Enabled     string
	MediaDelivery string
}

// LookupUser busca informações do usuário no banco de dados pelo token.
func LookupUser(db *sql.DB, token string) (*UserRecord, error) {
	query := `SELECT id, name, webhook, jid, events, proxy_url, qrcode, history,
		hmac_key IS NOT NULL AND length(hmac_key) > 0,
		CASE WHEN s3_enabled THEN 'true' ELSE 'false' END,
		COALESCE(media_delivery, 'base64')
		FROM users WHERE token = $1 LIMIT 1`

	rows, err := db.Query(query, token)
	if err != nil {
		return nil, fmt.Errorf("db query: %w", err)
	}
	defer rows.Close()

	rec := &UserRecord{}
	var historyVal sql.NullInt64
	found := false
	for rows.Next() {
		found = true
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Webhook, &rec.JID, &rec.Events,
			&rec.ProxyURL, &rec.QRCode, &historyVal, &rec.HasHmac,
			&rec.S3Enabled, &rec.MediaDelivery); err != nil {
			return nil, fmt.Errorf("db scan: %w", err)
		}
		rec.History = "0"
		if historyVal.Valid {
			rec.History = fmt.Sprintf("%d", historyVal.Int64)
		}
		log.Debug().Str("userId", rec.ID).Bool("historyValid", historyVal.Valid).
			Int64("historyValue", historyVal.Int64).Str("historyStr", rec.History).
			Msg("User authentication - history debug")
	}
	if !found {
		return nil, nil // usuário não encontrado
	}
	return rec, nil
}

// GetOrSetCache recupera do cache ou chama a factory se ausente.
// Retorna (valor, found) — found é false se precisou popular o cache.
func GetOrSetCache(tokenCache *cache.Cache, token string, factory func() interface{}) (interface{}, bool) {
	if val, found := tokenCache.Get(token); found {
		return val, true
	}
	val := factory()
	return val, false
}

// ContextKey é o tipo usado como chave de contexto para userinfo.
type ContextKey string

// UserInfoKey é a chave usada para injetar userinfo no contexto.
const UserInfoKey ContextKey = "userinfo"

// WithUserInfo injeta user info no contexto da requisição.
func WithUserInfo(ctx context.Context, val interface{}) context.Context {
	return context.WithValue(ctx, UserInfoKey, val)
}
