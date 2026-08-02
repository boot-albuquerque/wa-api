package db

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// UserRepository implementa appport.UserRepository sobre *sqlx.DB.
//
// Todo o SQL dos use cases de user/ mora aqui — inclusive a montagem do
// UPDATE dinâmico, que antes era construída com concatenação de string dentro
// de EditUserUseCase.Execute.
type UserRepository struct {
	db *sqlx.DB
}

// NewUserRepository cria o repositório.
func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

// isUniqueViolation reconhece violação de unicidade nos dois backends
// suportados. O casamento é por texto porque database/sql não expõe SQLSTATE
// de forma portável; aqui, ao contrário de na camada de aplicação, conhecer
// dialeto de banco é legítimo.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	unique := strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
	if unique {
		log.Warn().Err(err).Str("table", "users").Str("constraint", "unique").
			Msg("database rejected write on unique constraint")
	}
	return unique
}

// CreateUser insere o usuário.
//
// ON CONFLICT DO NOTHING mais linhas afetadas substitui o par
// SELECT COUNT(*)/INSERT que existia antes: a checagem prévia rodava fora de
// transação e sem lock, então duas requisições concorrentes com o mesmo token
// passavam ambas pela checagem e ambas inseriam. Quem decide é o índice
// UNIQUE de token_hash, atomicamente.
func (r *UserRepository) CreateUser(ctx context.Context, rec domain.UserRecord) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, name, token, token_hash, webhook, expiration, events, jid, qrcode, proxy_url,
		 webhook_use_proxy, s3_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key, s3_secret_key,
		 s3_path_style, s3_public_url, media_delivery, s3_retention_days, hmac_key, history)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		 ON CONFLICT DO NOTHING`,
		rec.ID, rec.Name, rec.Token, domain.HashToken(rec.Token), rec.Webhook, rec.Expiration, rec.Events, "", "",
		rec.ProxyURL, rec.WebhookUseProxy,
		rec.S3.Enabled, rec.S3.Endpoint, rec.S3.Region, rec.S3.Bucket,
		rec.S3.AccessKey, rec.S3.SecretKey, rec.S3.PathStyle, rec.S3.PublicURL,
		rec.S3.MediaDelivery, rec.S3.RetentionDays, rec.HmacKey, rec.History)

	if err != nil {
		if isUniqueViolation(err) {
			log.Warn().Err(err).Str("table", "users").Str("user_id", rec.ID).
				Str("column", "token_hash").Msg("create user rejected: duplicate token")
			return false, domain.ErrDuplicateToken
		}
		log.Error().Err(err).Str("table", "users").Str("user_id", rec.ID).
			Str("query", "insert_user").Msg("failed to insert user")
		return false, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		log.Error().Err(err).Str("table", "users").Str("user_id", rec.ID).
			Msg("failed to read rows affected after insert")
		return false, err
	}
	return affected > 0, nil
}

// UserExists reporta se há usuário com o id informado.
func (r *UserRepository) UserExists(ctx context.Context, id string) (bool, error) {
	var count int
	if err := r.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM users WHERE id = $1", id); err != nil {
		log.Error().Err(err).Str("table", "users").Str("user_id", id).
			Str("query", "user_exists").Msg("failed to check user existence")
		return false, err
	}
	return count > 0, nil
}

// UpdateUser aplica os campos presentes em upd.
//
// A ordem em que os campos entram no SET é a mesma que EditUserUseCase usava,
// para que a instrução gerada seja idêntica à de antes.
func (r *UserRepository) UpdateUser(ctx context.Context, id string, upd domain.UserUpdate) error {
	query := "UPDATE users SET "
	args := []interface{}{}
	argIndex := 1

	addField := func(fieldName string, value interface{}) {
		if argIndex > 1 {
			query += ", "
		}
		query += fieldName + " = $" + strconv.Itoa(argIndex)
		args = append(args, value)
		argIndex++
	}

	if upd.Name != nil {
		addField("name", *upd.Name)
	}
	if upd.Token != nil {
		addField("token", *upd.Token)
		addField("token_hash", domain.HashToken(*upd.Token))
	}
	if upd.Webhook != nil {
		addField("webhook", *upd.Webhook)
	}
	if upd.Expiration != nil {
		addField("expiration", *upd.Expiration)
	}
	if upd.Events != nil {
		addField("events", *upd.Events)
	}
	if upd.History != nil {
		addField("history", *upd.History)
	}
	if upd.ProxyURL != nil {
		addField("proxy_url", *upd.ProxyURL)
	}
	if upd.WebhookUseProxy != nil {
		addField("webhook_use_proxy", *upd.WebhookUseProxy)
	}
	if upd.S3 != nil {
		addField("s3_enabled", upd.S3.Enabled)
		addField("s3_endpoint", upd.S3.Endpoint)
		addField("s3_region", upd.S3.Region)
		addField("s3_bucket", upd.S3.Bucket)
		addField("s3_access_key", upd.S3.AccessKey)
		addField("s3_secret_key", upd.S3.SecretKey)
		addField("s3_path_style", upd.S3.PathStyle)
		addField("s3_public_url", upd.S3.PublicURL)
		addField("media_delivery", upd.S3.MediaDelivery)
		addField("s3_retention_days", upd.S3.RetentionDays)
	}

	if argIndex == 1 {
		log.Warn().Str("table", "users").Str("user_id", id).
			Msg("update user rejected: no fields to update")
		return domain.ErrNoFieldsToUpdate
	}

	query += " WHERE id = $" + strconv.Itoa(argIndex)
	args = append(args, id)

	// Antes desta fase não havia checagem nenhuma de duplicidade aqui: trocar
	// o token de um usuário para o token de outro passava silenciosamente.
	// Quem rejeita é o índice UNIQUE de token_hash; o papel deste bloco é
	// traduzir o erro do driver antes que ele vaze para o cliente HTTP.
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		if isUniqueViolation(err) {
			log.Warn().Err(err).Str("table", "users").Str("user_id", id).
				Str("column", "token_hash").Msg("update user rejected: duplicate token")
			return domain.ErrDuplicateToken
		}
		log.Error().Err(err).Str("table", "users").Str("user_id", id).
			Str("query", "update_user").Msg("failed to update user")
		return err
	}
	return nil
}

// ListUsers devolve todos os usuários, ou apenas o de id informado.
func (r *UserRepository) ListUsers(ctx context.Context, id string) ([]domain.UserListEntry, error) {
	const base = `SELECT id, name, webhook, jid, qrcode, connected, expiration, proxy_url,
				 COALESCE(webhook_use_proxy, true) AS webhook_use_proxy, events, history
				 FROM users`

	query := base
	var args []interface{}
	if id != "" {
		query = base + ` WHERE id = $1`
		args = append(args, id)
	}

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		log.Error().Err(err).Str("table", "users").Str("user_id", id).
			Str("query", "list_users").Msg("failed to list users")
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.UserListEntry
	for rows.Next() {
		var row struct {
			ID              string         `db:"id"`
			Name            string         `db:"name"`
			Webhook         string         `db:"webhook"`
			JID             string         `db:"jid"`
			QRCode          string         `db:"qrcode"`
			Connected       sql.NullBool   `db:"connected"`
			Expiration      sql.NullInt64  `db:"expiration"`
			ProxyURL        sql.NullString `db:"proxy_url"`
			WebhookUseProxy bool           `db:"webhook_use_proxy"`
			Events          string         `db:"events"`
			History         sql.NullInt64  `db:"history"`
		}
		if err := rows.StructScan(&row); err != nil {
			log.Error().Err(err).Str("table", "users").Str("query", "list_users").
				Msg("failed to scan user row")
			return nil, err
		}

		entry := domain.UserListEntry{
			ID:              row.ID,
			Name:            row.Name,
			Webhook:         row.Webhook,
			JID:             row.JID,
			QRCode:          row.QRCode,
			Expiration:      row.Expiration.Int64,
			ProxyURL:        row.ProxyURL.String,
			HasProxyURL:     row.ProxyURL.Valid && row.ProxyURL.String != "",
			WebhookUseProxy: row.WebhookUseProxy,
			Events:          row.Events,
		}

		// A configuração de S3 vem numa segunda consulta, como antes. Falha
		// aqui não derruba a listagem: o registro sai com S3 zerado, que é o
		// comportamento que o use case tinha ao ignorar o erro.
		if s3, err := r.userS3Config(ctx, row.ID); err == nil {
			entry.S3 = s3
		}

		out = append(out, entry)
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Str("table", "users").Str("query", "list_users").
			Msg("failed to iterate user rows")
		return nil, err
	}
	return out, nil
}

// userS3Config lê a configuração de S3 de um usuário.
func (r *UserRepository) userS3Config(ctx context.Context, id string) (domain.S3Config, error) {
	var s3 domain.S3Config
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(s3_enabled, false), COALESCE(s3_endpoint, ''), COALESCE(s3_region, ''),
		 COALESCE(s3_bucket, ''), COALESCE(s3_path_style, false), COALESCE(s3_public_url, ''),
		 COALESCE(media_delivery, ''), COALESCE(s3_retention_days, 0) FROM users WHERE id = $1`,
		id).Scan(&s3.Enabled, &s3.Endpoint, &s3.Region, &s3.Bucket, &s3.PathStyle, &s3.PublicURL,
		&s3.MediaDelivery, &s3.RetentionDays)
	if err != nil {
		log.Warn().Err(err).Str("table", "users").Str("user_id", id).
			Str("query", "user_s3_config").Msg("failed to read s3 config for user")
	}
	return s3, err
}

// DeleteUser remove o usuário.
func (r *UserRepository) DeleteUser(ctx context.Context, id string) (bool, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM users WHERE id=$1", id)
	if err != nil {
		log.Error().Err(err).Str("table", "users").Str("user_id", id).
			Str("query", "delete_user").Msg("failed to delete user")
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		log.Error().Err(err).Str("table", "users").Str("user_id", id).
			Msg("failed to read rows affected after delete")
		return false, err
	}
	return affected > 0, nil
}

// Verificação em tempo de compilação de que o repositório implementa a porta.
var _ appport.UserRepository = (*UserRepository)(nil)
