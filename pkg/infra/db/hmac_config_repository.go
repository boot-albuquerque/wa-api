package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// As três instruções da coluna users.hmac_key, nomeadas.
//
// São constantes, e não literais inline, porque o teste do repositório tem de
// executar A MESMA instrução contra o schema real — foi assim que a F71
// escapou: uma query com nome de coluna inexistente que nenhum teste
// exercitava. `?` é o placeholder portável; db.Rebind traduz para o dialeto.
const (
	hmacKeySelectQuery = "SELECT hmac_key FROM users WHERE id = ?"
	hmacKeyUpdateQuery = "UPDATE users SET hmac_key = ? WHERE id = ?"
	hmacKeyClearQuery  = "UPDATE users SET hmac_key = NULL WHERE id = ?"
)

// hmacKeyTable é o rótulo da tabela nos registros estruturados deste
// repositório.
const hmacKeyTable = "users"

// HmacConfigRepository implements appport.HmacKeyStore over *sqlx.DB.
//
// Ele guarda o valor CIFRADO tal como recebe: cifrar é responsabilidade de
// appport.HmacKeyEncryptor, e duplicá-la aqui daria dois lugares para a mesma
// regra divergir.
type HmacConfigRepository struct {
	db *sqlx.DB
}

// NewHmacConfigRepository creates the repository.
func NewHmacConfigRepository(db *sqlx.DB) *HmacConfigRepository {
	return &HmacConfigRepository{db: db}
}

// SaveHmacKey implements appport.HmacKeyStore.
func (r *HmacConfigRepository) SaveHmacKey(ctx context.Context, userID string, encryptedKey []byte) error {
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(hmacKeyUpdateQuery), encryptedKey, userID); err != nil {
		log.Error().Err(err).Str("table", hmacKeyTable).Str("user_id", userID).
			Str("query", "save_hmac_key").Msg("failed to store the user HMAC key")
		return err
	}
	return nil
}

// LoadHmacKey implements appport.HmacKeyStore.
//
// Linha ausente devolve (nil, nil), e não erro: o handler histórico respondia
// sql.ErrNoRows com 200 e `hmac_key` vazio (41bc8e2^:handlers.go:6832), o que
// é contrato público. Coluna NULL cai no mesmo lugar por outro caminho — o
// destino de scan é []byte, que recebe NULL como nil.
func (r *HmacConfigRepository) LoadHmacKey(ctx context.Context, userID string) ([]byte, error) {
	var encryptedKey []byte
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(hmacKeySelectQuery), userID).Scan(&encryptedKey)
	if errors.Is(err, sql.ErrNoRows) {
		log.Debug().Str("table", hmacKeyTable).Str("user_id", userID).
			Msg("no user row while reading the HMAC key; reporting absence")
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("table", hmacKeyTable).Str("user_id", userID).
			Str("query", "load_hmac_key").Msg("failed to read the user HMAC key")
		return nil, err
	}
	return encryptedKey, nil
}

// DeleteHmacKey implements appport.HmacKeyStore.
func (r *HmacConfigRepository) DeleteHmacKey(ctx context.Context, userID string) error {
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(hmacKeyClearQuery), userID); err != nil {
		log.Error().Err(err).Str("table", hmacKeyTable).Str("user_id", userID).
			Str("query", "delete_hmac_key").Msg("failed to clear the user HMAC key")
		return err
	}
	return nil
}
