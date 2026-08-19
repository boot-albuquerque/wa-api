package port

import "context"

// S3ConfigRecord is the per-user S3 configuration as it crosses the ports of
// the four S3 use cases. Its ten fields are exactly the ten columns the
// historical UPDATE wrote (`41bc8e2^:handlers.go:6243`).
//
// SecretKey carries DIFFERENT things on the two sides of the boundary, and
// which one it is depends on the port:
//
//   - S3ConfigStore: the STORED form, i.e. the `enc:v1:` envelope of
//     ADR-0009. The store never sees the plaintext, so "encrypt before
//     writing" is a type-level fact and not a convention someone can forget.
//   - S3ClientManager: the PLAINTEXT secret. The AWS SDK signs requests with
//     it, so a client cannot be built from the envelope.
type S3ConfigRecord struct {
	Enabled       bool
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	PathStyle     bool
	PublicURL     string
	MediaDelivery string
	RetentionDays int
}

// S3ConfigStore is the persistence port of the four S3 use cases, over the ten
// s3_* columns of the users table (pkg/infra/db/migrations.go:404-424).
//
// Writing anywhere else would light up nothing: the same columns are what
// S3Manager.EnsureClientFromDB (pkg/infra/storage/s3.go:85) reads to build the
// client that actually uploads media.
type S3ConfigStore interface {
	// SaveS3Config writes the ten columns for userID. cfg.SecretKey must
	// already be the envelope.
	SaveS3Config(ctx context.Context, userID string, cfg S3ConfigRecord) error

	// LoadS3Config reads the ten columns, SecretKey included, still
	// enveloped. It is the read of the connection test, which is the only
	// caller that legitimately needs the secret back.
	LoadS3Config(ctx context.Context, userID string) (*S3ConfigRecord, error)

	// LoadS3ConfigWithoutSecret reads the same row with a SELECT that does
	// NOT name s3_secret_key, and returns SecretKey empty.
	//
	// It is a separate method, and not a caller-side blanking of the field,
	// because the historical GET owed its safety to the SELECT itself
	// (`41bc8e2^:handlers.go:6334`): a secret that is never read cannot be
	// leaked by a later refactor of the response shape.
	LoadS3ConfigWithoutSecret(ctx context.Context, userID string) (*S3ConfigRecord, error)

	// DeleteS3Config resets the ten columns to the historical cleared state
	// (`41bc8e2^:handlers.go:6465`). Clearing a configuration that is already
	// absent is not an error — revocation is idempotent by nature.
	DeleteS3Config(ctx context.Context, userID string) error
}

// S3SecretCipher wraps the versioned envelope of ADR-0009 around the process
// AES-GCM key.
//
// It is a S3-specific port rather than a reuse of HmacKeyEncryptor because the
// two carry different STORED types: the HMAC key lives in a BYTEA column and
// crosses as []byte, while s3_secret_key is TEXT and crosses as the `enc:v1:`
// string. The ALGORITHM is not duplicated — pkg/infra/auth.EncryptS3Secret
// delegates to the same AES-GCM that protects the HMAC key.
type S3SecretCipher interface {
	// EncryptS3Secret returns the envelope for plainSecret, or "" when
	// plainSecret is "". It fails when the global encryption key is absent or
	// invalid for AES — and a failure here must NOT be followed by a write,
	// or the column would hold plaintext.
	EncryptS3Secret(plainSecret string) (string, error)

	// DecryptS3Secret returns the plaintext secret behind a stored value. A
	// non-empty value without the envelope prefix is an ERROR, never a
	// plaintext secret (ADR-0009).
	DecryptS3Secret(storedSecret string) (string, error)
}

// S3ClientManager is the in-memory registry of per-user S3 clients.
//
// This port is not a convenience. The client it holds is built once from the
// credentials and then serves every media upload
// (S3Manager.ProcessMediaForS3); a DELETE that cleared only the database would
// leave the revoked credential uploading from this registry until the process
// restarts: same 200, opposite outcome (HOUSEKEEP F157).
type S3ClientManager interface {
	// InitializeS3Client creates or replaces the client of userID. cfg
	// carries the PLAINTEXT secret.
	InitializeS3Client(userID string, cfg S3ConfigRecord) error

	// RemoveClient drops the client of userID. Removing a client that is not
	// registered is a no-op.
	RemoveClient(userID string)

	// TestConnection reaches the configured endpoint with the registered
	// client. It is the only method here that touches the network.
	TestConnection(ctx context.Context, userID string) error
}

// UserInfoS3Cache mutates the long-lived userinfo cache entry of a user.
//
// Only the two fields that READERS consume are published: "S3Enabled" and
// "MediaDelivery", which eventhandler_message.go:81-82 reads to decide whether
// an inbound media goes to S3. The credentials are deliberately NOT cached —
// see the comment on the adapter (pkg/bootstrap/s3_config_adapters.go).
type UserInfoS3Cache interface {
	// SetS3Config publishes the post-operation state of userID. A user with
	// no cached entry is a no-op: the entry is rebuilt from the database on
	// the next miss (ensureUserInfoCached).
	SetS3Config(userID string, enabled bool, mediaDelivery string)
}
