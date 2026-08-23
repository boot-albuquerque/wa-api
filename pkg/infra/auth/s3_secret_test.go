package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// s3SecretTestKey has 32 bytes because AES-256 requires exactly that —
// aes.NewCipher rejects any other length (hmac.go:46).
const s3SecretTestKey = "0123456789abcdef0123456789abcdef"

const s3SecretTestPlain = "sUp3r-s3cr3t-s3-key-value"

// TestS3Secret_RoundTrip — the envelope has to be REVERSIBLE, not merely
// different from the plaintext. "different" would also hold for a hash, and a
// hash cannot sign an S3 request.
func TestS3Secret_RoundTrip(t *testing.T) {
	envelope, err := EncryptS3Secret(s3SecretTestPlain, s3SecretTestKey)
	if err != nil {
		t.Fatalf("EncryptS3Secret: %v", err)
	}
	if !strings.HasPrefix(envelope, S3SecretEnvelopePrefix) {
		t.Fatalf("envelope = %q, want the %q prefix", envelope, S3SecretEnvelopePrefix)
	}
	if strings.Contains(envelope, s3SecretTestPlain) {
		t.Fatalf("the plaintext survives inside the envelope: %q", envelope)
	}

	plain, err := DecryptS3Secret(envelope, []byte(s3SecretTestKey))
	if err != nil {
		t.Fatalf("DecryptS3Secret: %v", err)
	}
	if plain != s3SecretTestPlain {
		t.Fatalf("round trip = %q, want %q", plain, s3SecretTestPlain)
	}
}

// TestS3Secret_TwoEncryptionsDiffer — AES-GCM uses a fresh nonce per call, so
// the same secret must not produce the same stored string twice. Equal
// ciphertexts would mean a fixed nonce, which breaks GCM outright.
func TestS3Secret_TwoEncryptionsDiffer(t *testing.T) {
	first, err := EncryptS3Secret(s3SecretTestPlain, s3SecretTestKey)
	if err != nil {
		t.Fatalf("EncryptS3Secret: %v", err)
	}
	second, err := EncryptS3Secret(s3SecretTestPlain, s3SecretTestKey)
	if err != nil {
		t.Fatalf("EncryptS3Secret: %v", err)
	}
	if first == second {
		t.Fatal("two encryptions of the same secret are identical: the nonce is not fresh")
	}
}

// TestS3Secret_EmptyIsNotEnveloped — "" is the column default and the value
// DELETE writes. Enveloping it would turn "no secret configured" into "a
// secret that decrypts to nothing", and every reader would have to unwrap
// before it could tell the two apart.
func TestS3Secret_EmptyIsNotEnveloped(t *testing.T) {
	envelope, err := EncryptS3Secret("", s3SecretTestKey)
	if err != nil {
		t.Fatalf("EncryptS3Secret(\"\"): %v", err)
	}
	if envelope != "" {
		t.Fatalf("empty secret produced %q, want \"\"", envelope)
	}
	plain, err := DecryptS3Secret("", []byte(s3SecretTestKey))
	if err != nil {
		t.Fatalf("DecryptS3Secret(\"\"): %v", err)
	}
	if plain != "" {
		t.Fatalf("decrypting \"\" produced %q", plain)
	}
}

// TestS3Secret_LegacyPlaintextIsRefused is the ADR-0009 control.
//
// A non-empty value without the prefix is the legacy row, and the answer is a
// NAMED error — never the value itself. The assertion on the returned string
// is the one that matters: an implementation that "handled" the error by
// returning the stored value would still return a non-nil error and pass a
// test that only checked err != nil.
func TestS3Secret_LegacyPlaintextIsRefused(t *testing.T) {
	for _, legacy := range []string{
		s3SecretTestPlain,
		"enc:v0:" + base64.StdEncoding.EncodeToString([]byte("x")),
		base64.StdEncoding.EncodeToString([]byte(s3SecretTestPlain)),
	} {
		t.Run(legacy, func(t *testing.T) {
			plain, err := DecryptS3Secret(legacy, []byte(s3SecretTestKey))
			var notEnveloped ErrS3SecretNotEnveloped
			if !errors.As(err, &notEnveloped) {
				t.Fatalf("err = %v, want ErrS3SecretNotEnveloped", err)
			}
			if plain != "" {
				t.Fatalf("the refusal returned %q — the legacy value is being handed back as a credential", plain)
			}
		})
	}
}

// TestS3Secret_CorruptEnvelopeIsRefused — a value WITH the prefix whose
// payload does not decrypt is a distinct failure from the legacy row, and both
// fail closed.
func TestS3Secret_CorruptEnvelopeIsRefused(t *testing.T) {
	cases := map[string]string{
		"not base64":       S3SecretEnvelopePrefix + "!!!not-base64!!!",
		"base64 but short": S3SecretEnvelopePrefix + base64.StdEncoding.EncodeToString([]byte("ab")),
		"wrong key": func() string {
			other, err := EncryptS3Secret(s3SecretTestPlain, "ffffffffffffffffffffffffffffffff")
			if err != nil {
				t.Fatalf("EncryptS3Secret: %v", err)
			}
			return other
		}(),
	}
	for name, stored := range cases {
		t.Run(name, func(t *testing.T) {
			plain, err := DecryptS3Secret(stored, []byte(s3SecretTestKey))
			if err == nil {
				t.Fatal("a corrupt envelope was accepted")
			}
			var notEnveloped ErrS3SecretNotEnveloped
			if errors.As(err, &notEnveloped) {
				t.Fatal("a corrupt envelope was reported as a legacy row: the two states must stay distinct")
			}
			if plain != "" {
				t.Fatalf("the refusal returned %q", plain)
			}
		})
	}
}

// TestS3Secret_NoEncryptionKeyFailsClosed — both directions refuse when the
// process has no key, and neither returns the input.
func TestS3Secret_NoEncryptionKeyFailsClosed(t *testing.T) {
	if envelope, err := EncryptS3Secret(s3SecretTestPlain, ""); err == nil {
		t.Fatalf("EncryptS3Secret with no key returned %q and no error", envelope)
	} else if strings.Contains(err.Error(), s3SecretTestPlain) {
		t.Fatalf("the secret leaked into the error: %v", err)
	}

	envelope, err := EncryptS3Secret(s3SecretTestPlain, s3SecretTestKey)
	if err != nil {
		t.Fatalf("EncryptS3Secret: %v", err)
	}
	if plain, err := DecryptS3Secret(envelope, nil); err == nil {
		t.Fatalf("DecryptS3Secret with no key returned %q and no error", plain)
	}
}
