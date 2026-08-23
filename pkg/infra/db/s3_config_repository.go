package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
)

// The four statements over the ten s3_* columns of users, named.
//
// They are constants rather than inline literals because the repository test
// has to execute THE SAME statement against the real schema — that is how F71
// escaped: a query naming a column that did not exist, exercised by no test.
// `?` is the portable placeholder; db.Rebind translates it per dialect.
const (
	// s3ConfigUpdateQuery writes the same ten columns the historical UPDATE
	// wrote (`41bc8e2^:handlers.go:6243`).
	s3ConfigUpdateQuery = `UPDATE users SET
		s3_enabled = ?,
		s3_endpoint = ?,
		s3_region = ?,
		s3_bucket = ?,
		s3_access_key = ?,
		s3_secret_key = ?,
		s3_path_style = ?,
		s3_public_url = ?,
		media_delivery = ?,
		s3_retention_days = ?
	WHERE id = ?`

	// s3ConfigSelectQuery is the read of the connection test: it DOES name
	// s3_secret_key, because building a client needs the credential back.
	s3ConfigSelectQuery = `SELECT s3_enabled, s3_endpoint, s3_region, s3_bucket,
		s3_access_key, s3_secret_key, s3_path_style, s3_public_url,
		COALESCE(media_delivery, ?) AS media_delivery,
		COALESCE(s3_retention_days, ?) AS s3_retention_days
	FROM users WHERE id = ?`

	// s3ConfigSelectWithoutSecretQuery is the read of GET /s3/config, and the
	// ABSENCE of s3_secret_key in it is the whole point: the historical
	// handler owed its safety to the SELECT, not to a later blanking of the
	// field (`41bc8e2^:handlers.go:6334`).
	s3ConfigSelectWithoutSecretQuery = `SELECT s3_enabled, s3_endpoint, s3_region, s3_bucket,
		s3_access_key, s3_path_style, s3_public_url,
		COALESCE(media_delivery, ?) AS media_delivery,
		COALESCE(s3_retention_days, ?) AS s3_retention_days
	FROM users WHERE id = ?`

	// s3ConfigClearQuery resets the ten columns to the historical cleared
	// state (`41bc8e2^:handlers.go:6465`), including its defaults:
	// path_style true, media_delivery 'base64' and 30 days of retention.
	s3ConfigClearQuery = `UPDATE users SET
		s3_enabled = ?,
		s3_endpoint = '',
		s3_region = '',
		s3_bucket = '',
		s3_access_key = '',
		s3_secret_key = '',
		s3_path_style = ?,
		s3_public_url = '',
		media_delivery = ?,
		s3_retention_days = ?
	WHERE id = ?`
)

// s3ConfigTable is the table label in this repository's structured records.
const s3ConfigTable = "users"

// The cleared/default values of the S3 configuration, as constants: the same
// numbers are the COALESCE fallback of the reads and the value the clear
// writes, and a literal that drifted between them would make a deleted
// configuration read back as a different one.
const (
	s3DefaultMediaDelivery = "base64"
	s3DefaultRetentionDays = 30
	s3DefaultPathStyle     = true
	s3ClearedEnabled       = false
)

// S3ConfigRepository implements appport.S3ConfigStore over *sqlx.DB.
//
// It stores the secret exactly as it receives it: enveloping is the job of
// appport.S3SecretCipher, and duplicating it here would give the same rule two
// places to diverge.
type S3ConfigRepository struct {
	db *sqlx.DB
}

var _ appport.S3ConfigStore = (*S3ConfigRepository)(nil)

// NewS3ConfigRepository creates the repository.
func NewS3ConfigRepository(db *sqlx.DB) *S3ConfigRepository {
	return &S3ConfigRepository{db: db}
}

// s3ConfigRow is the scan destination shared by the two reads. SecretKey stays
// zero on the read that does not select it.
type s3ConfigRow struct {
	Enabled       bool   `db:"s3_enabled"`
	Endpoint      string `db:"s3_endpoint"`
	Region        string `db:"s3_region"`
	Bucket        string `db:"s3_bucket"`
	AccessKey     string `db:"s3_access_key"`
	SecretKey     string `db:"s3_secret_key"`
	PathStyle     bool   `db:"s3_path_style"`
	PublicURL     string `db:"s3_public_url"`
	MediaDelivery string `db:"media_delivery"`
	RetentionDays int    `db:"s3_retention_days"`
}

func (r s3ConfigRow) toRecord() *appport.S3ConfigRecord {
	return &appport.S3ConfigRecord{
		Enabled:       r.Enabled,
		Endpoint:      r.Endpoint,
		Region:        r.Region,
		Bucket:        r.Bucket,
		AccessKey:     r.AccessKey,
		SecretKey:     r.SecretKey,
		PathStyle:     r.PathStyle,
		PublicURL:     r.PublicURL,
		MediaDelivery: r.MediaDelivery,
		RetentionDays: r.RetentionDays,
	}
}

// SaveS3Config implements appport.S3ConfigStore.
func (r *S3ConfigRepository) SaveS3Config(ctx context.Context, userID string, cfg appport.S3ConfigRecord) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(s3ConfigUpdateQuery),
		cfg.Enabled, cfg.Endpoint, cfg.Region, cfg.Bucket,
		cfg.AccessKey, cfg.SecretKey, cfg.PathStyle, cfg.PublicURL,
		cfg.MediaDelivery, cfg.RetentionDays, userID)
	if err != nil {
		log.Error().Err(err).Str("table", s3ConfigTable).Str("user_id", userID).
			Str("query", "save_s3_config").Msg("failed to store the user S3 configuration")
		return err
	}
	return nil
}

// LoadS3Config implements appport.S3ConfigStore.
//
// A missing row returns (nil, nil), not an error: the S3 configuration of a
// user who has none is "not enabled", which is what every caller then acts on.
func (r *S3ConfigRepository) LoadS3Config(ctx context.Context, userID string) (*appport.S3ConfigRecord, error) {
	var row s3ConfigRow
	err := r.db.GetContext(ctx, &row, r.db.Rebind(s3ConfigSelectQuery),
		s3DefaultMediaDelivery, s3DefaultRetentionDays, userID)
	if errors.Is(err, sql.ErrNoRows) {
		log.Debug().Str("table", s3ConfigTable).Str("user_id", userID).
			Str("query", "load_s3_config").
			Msg("no user row while reading the S3 configuration; reporting absence")
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("table", s3ConfigTable).Str("user_id", userID).
			Str("query", "load_s3_config").Msg("failed to read the user S3 configuration")
		return nil, err
	}
	return row.toRecord(), nil
}

// LoadS3ConfigWithoutSecret implements appport.S3ConfigStore.
//
// It is written out rather than sharing a helper with LoadS3Config so that the
// two reads log under their OWN query label: "the read failed" is a different
// operational event depending on whether the secret was part of it.
func (r *S3ConfigRepository) LoadS3ConfigWithoutSecret(ctx context.Context, userID string) (*appport.S3ConfigRecord, error) {
	var row s3ConfigRow
	err := r.db.GetContext(ctx, &row, r.db.Rebind(s3ConfigSelectWithoutSecretQuery),
		s3DefaultMediaDelivery, s3DefaultRetentionDays, userID)
	if errors.Is(err, sql.ErrNoRows) {
		log.Debug().Str("table", s3ConfigTable).Str("user_id", userID).
			Str("query", "load_s3_config_without_secret").
			Msg("no user row while reading the S3 configuration; reporting absence")
		return nil, nil
	}
	if err != nil {
		log.Error().Err(err).Str("table", s3ConfigTable).Str("user_id", userID).
			Str("query", "load_s3_config_without_secret").Msg("failed to read the user S3 configuration")
		return nil, err
	}
	return row.toRecord(), nil
}

// DeleteS3Config implements appport.S3ConfigStore.
func (r *S3ConfigRepository) DeleteS3Config(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(s3ConfigClearQuery),
		s3ClearedEnabled, s3DefaultPathStyle, s3DefaultMediaDelivery,
		s3DefaultRetentionDays, userID)
	if err != nil {
		log.Error().Err(err).Str("table", s3ConfigTable).Str("user_id", userID).
			Str("query", "delete_s3_config").Msg("failed to clear the user S3 configuration")
		return err
	}
	return nil
}
