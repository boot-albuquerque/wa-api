package port

import "context"

// HmacKeyStore is the persistence port of the three per-user HMAC use cases.
//
// It carries the ENCRYPTED key and nothing else: the plaintext exists only
// between the request body and HmacKeyEncryptor.EncryptHmacKey, and never
// crosses this boundary. A store that took plaintext would make "encrypt
// before writing" a convention instead of a type.
//
// The column behind it is users.hmac_key BYTEA (pkg/infra/db/migrations.go),
// the same one auth.LookupUser reads to compute HasHmac and
// resolverChaveHMAC (pkg/bootstrap/dispatch_outbox.go) reads to sign a pending
// delivery. Writing anywhere else would light up nothing.
type HmacKeyStore interface {
	// SaveHmacKey stores encryptedKey as the user's HMAC key, replacing any
	// previous one.
	SaveHmacKey(ctx context.Context, userID string, encryptedKey []byte) error

	// LoadHmacKey returns the stored encrypted key, or nil when the user has
	// none. A user row that does not exist is NOT an error: the historical
	// handler answered sql.ErrNoRows with 200 and an empty key
	// (41bc8e2^:handlers.go:6825), and that is public contract.
	LoadHmacKey(ctx context.Context, userID string) ([]byte, error)

	// DeleteHmacKey clears the user's HMAC key. Deleting a key that is
	// already absent is not an error — revocation is idempotent by nature.
	DeleteHmacKey(ctx context.Context, userID string) error
}

// HmacKeyEncryptor wraps the AES-GCM encryption of the key at rest.
//
// The use case does not own the encryption key: it lives in the process
// configuration (appCtx.GlobalEncryptionKey), and reaching for it from the
// application layer would drag bootstrap into it.
type HmacKeyEncryptor interface {
	// EncryptHmacKey encrypts the plaintext key. It fails when the global
	// encryption key is absent or invalid for AES — and a failure here must
	// NOT be followed by a write, or the column would hold plaintext.
	EncryptHmacKey(plainKey string) ([]byte, error)
}

// UserInfoHmacCache mutates the long-lived userinfo cache entry of a user.
//
// This port is not a convenience. The entry it writes lives in
// appCtx.UserInfoCache under cache.NoExpiration, and
// pkg/bootstrap/lifecycle_webhook.go:178 reads "HmacKeyEncrypted" from it to
// sign every per-user webhook. A DELETE that cleared only the database would
// leave the revoked key signing from that cache until the process restarts:
// same 200, opposite outcome (HOUSEKEEP F157).
type UserInfoHmacCache interface {
	// SetHmacKey publishes encryptedKey to the cached entry of userID. A nil
	// or empty encryptedKey means REVOKED, and must clear the entry rather
	// than leave the previous value in place.
	//
	// A user with no cached entry is a no-op: the entry is built from the
	// database on the next miss (ensureUserInfoCached), so there is nothing
	// stale to correct.
	SetHmacKey(userID string, encryptedKey []byte)
}
