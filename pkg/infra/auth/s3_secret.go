package auth

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

// S3SecretEnvelopePrefix marks a versioned AES-GCM envelope in the TEXT column
// users.s3_secret_key (ADR-0009).
//
// The stored form is `enc:v1:<base64 of the AES-GCM ciphertext>`. The prefix is
// what makes a legacy plaintext row DETECTABLE instead of ambiguous: without
// it, "written in the clear by the historical binary" and "ciphertext
// corrupted" both surface as the same Decrypt failure, and the obvious way to
// "handle" that failure is to read the value as plaintext — a silent fallback
// on a secret path.
//
// The `v1` is the algorithm-rotation seam: a future v2 changes the tag, not
// this discussion.
const S3SecretEnvelopePrefix = "enc:v1:"

// ErrS3SecretNotEnveloped is returned by DecryptS3Secret for a NON-EMPTY value
// that lacks S3SecretEnvelopePrefix.
//
// This is the named state ADR-0009 requires: the credential must be treated as
// invalid and reconfigured. It must NEVER be answered by using the value as a
// plaintext secret — that would make the historical defect permanent.
type ErrS3SecretNotEnveloped struct{}

func (ErrS3SecretNotEnveloped) Error() string {
	return "S3 secret is not in the " + S3SecretEnvelopePrefix + " envelope; the credential must be reconfigured"
}

// EncryptS3Secret wraps plainSecret in the versioned envelope, using the same
// AES-GCM primitive that protects the HMAC key (EncryptHMACKey). The algorithm
// is not duplicated here — only the framing is.
//
// An EMPTY secret produces an EMPTY string, not an envelope: "" is the column
// default and the value DELETE writes, so enveloping it would turn "no secret
// configured" into "a secret that decrypts to nothing".
func EncryptS3Secret(plainSecret, encryptionKey string) (string, error) {
	if plainSecret == "" {
		return "", nil
	}
	cipherText, err := EncryptHMACKey(plainSecret, encryptionKey)
	if err != nil {
		// EncryptHMACKey logs under its own component name, which would send
		// whoever reads this line looking at the HMAC path. The secret itself
		// never appears — only its length.
		log.Error().Err(err).
			Str("component", "auth.EncryptS3Secret").
			Int("plaintext_bytes", len(plainSecret)).
			Msg("S3 secret encryption refused: AES-GCM sealing failed")
		return "", fmt.Errorf("failed to encrypt the S3 secret: %w", err)
	}
	return S3SecretEnvelopePrefix + base64.StdEncoding.EncodeToString(cipherText), nil
}

// DecryptS3Secret unwraps a stored value back to the plaintext secret.
//
// The three outcomes are distinct on purpose:
//   - "" means no secret is configured, and is not an error;
//   - a value WITHOUT the prefix is ErrS3SecretNotEnveloped — fail closed;
//   - a value with the prefix is decrypted, and a decryption failure is
//     reported as such.
//
// No branch returns the stored value itself.
func DecryptS3Secret(storedSecret string, encryptionKey []byte) (string, error) {
	if storedSecret == "" {
		return "", nil
	}
	if !strings.HasPrefix(storedSecret, S3SecretEnvelopePrefix) {
		// This is the ADR-0009 named state, and the operator has to see it:
		// the credential was written in the clear by the historical binary,
		// must be considered exposed, and needs to be reconfigured. Warn and
		// not Error because nothing is broken in the process — the stored
		// value is.
		log.Warn().
			Str("component", "auth.DecryptS3Secret").
			Int("stored_bytes", len(storedSecret)).
			Str("expected_prefix", S3SecretEnvelopePrefix).
			Msg("S3 secret refused: stored value has no versioned envelope; the credential must be reconfigured")
		return "", ErrS3SecretNotEnveloped{}
	}
	cipherText, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(storedSecret, S3SecretEnvelopePrefix))
	if err != nil {
		log.Warn().Err(err).
			Str("component", "auth.DecryptS3Secret").
			Int("stored_bytes", len(storedSecret)).
			Msg("S3 secret refused: envelope payload is not valid base64")
		return "", fmt.Errorf("the S3 secret envelope is not valid base64: %w", err)
	}
	plain, err := DecryptHMACKey(cipherText, encryptionKey)
	if err != nil {
		log.Error().Err(err).
			Str("component", "auth.DecryptS3Secret").
			Int("ciphertext_bytes", len(cipherText)).
			Msg("S3 secret decryption failed: envelope present but unreadable (wrong key or tampered data)")
		return "", fmt.Errorf("failed to decrypt the S3 secret: %w", err)
	}
	return plain, nil
}
