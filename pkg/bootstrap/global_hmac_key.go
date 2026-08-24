package bootstrap

import (
	cryptorand "crypto/rand"
	"fmt"

	"github.com/rs/zerolog/log"
)

// Resolution of the GLOBAL HMAC key — the key that signs the global webhooks
// (dispatch_outbox.go, scope HMACScopeGlobal). See F156 in HOUSEKEEP.md.
//
// Two properties this file exists to hold:
//
//  1. The key value NEVER reaches a log line. A log line is collected,
//     forwarded to an aggregator and retained; whoever reads it can forge a
//     signed global webhook. The log says only THAT a key was resolved, and
//     from where.
//  2. A generated key comes from crypto/rand, not math/rand. math/rand is not
//     a CSPRNG: its output stream is predictable from observed outputs, which
//     is the wrong property for a signing key. (This is NOT the classic
//     fixed-seed case — since Go 1.20 the math/rand top-level functions are
//     auto-seeded with a random value. The defect is the generator's class,
//     not its seed.)
//
// math/rand remains correct — and stays — in dispatch_retry.go, where it draws
// backoff jitter: jitter is not a secret.
const (
	// envGlobalHMACKey is the environment variable that carries an
	// operator-supplied global HMAC key.
	envGlobalHMACKey = "WA_API_GLOBAL_HMAC_KEY"

	// flagGlobalHMACKey is the command-line equivalent, named here so the
	// startup error can point at both channels without repeating a literal.
	flagGlobalHMACKey = "-globalhmackey"

	// hmacKeyCharset is the alphabet a generated key is drawn from. It is kept
	// alphanumeric so the value survives every transport an operator may paste
	// it through (.env file, shell, container manifest) without quoting.
	hmacKeyCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// generatedHMACKeyLength is the length, in characters, of a generated key.
	// It matches the length the previous math/rand block produced, so nothing
	// downstream of the key sees a format change.
	generatedHMACKeyLength = 32
)

// hmacKeySource records where a resolved global HMAC key came from. It exists
// so a caller (and a test) can tell "the operator configured one" from "we
// generated one" without inspecting the key itself.
type hmacKeySource int

const (
	hmacKeyFromFlag hmacKeySource = iota
	hmacKeyFromEnv
	hmacKeyGenerated
)

// String makes the source safe to put in a log field: it describes the key
// without revealing any part of it.
func (s hmacKeySource) String() string {
	switch s {
	case hmacKeyFromFlag:
		return "command_line"
	case hmacKeyFromEnv:
		return "environment"
	case hmacKeyGenerated:
		return "generated"
	default:
		return "unknown"
	}
}

// generateGlobalHMACKey draws a key from crypto/rand.
//
// No production caller: resolveGlobalHMACKey no longer generates a key (F156
// fail-closed). This function is kept because three tests lock real properties
// of the generator (format, uniqueness, crypto/rand source) that would be lost
// if it were deleted — and the structural test TestF156_GeradorEhCriptografico
// asserts that THIS FILE uses crypto/rand, which requires the import to exist.
//
// The draw uses rejection sampling rather than `b % len(charset)`: 256 is not a
// multiple of the 62-character alphabet, so a plain modulo would make the first
// 256%62 == 8 letters measurably likelier than the rest. Bytes at or above the
// largest multiple of the alphabet size that fits in a byte are discarded.
func generateGlobalHMACKey() (string, error) {
	limit := byte(len(hmacKeyCharset) * (256 / len(hmacKeyCharset)))

	out := make([]byte, 0, generatedHMACKeyLength)
	buf := make([]byte, generatedHMACKeyLength)
	for len(out) < generatedHMACKeyLength {
		if _, err := cryptorand.Read(buf); err != nil {
			// The failure is logged here and the error still propagates: the
			// caller turns it into a Fatal, and this line says WHICH step of
			// the startup failed. No key exists yet, so there is nothing to
			// leak.
			log.Error().Err(err).Int("requested_bytes", len(buf)).
				Msg("global HMAC key not generated: the entropy source failed")
			return "", fmt.Errorf("draw random bytes for the global HMAC key: %w", err)
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out = append(out, hmacKeyCharset[int(b)%len(hmacKeyCharset)])
			if len(out) == generatedHMACKeyLength {
				break
			}
		}
	}
	return string(out), nil
}

// resolveGlobalHMACKey decides which global HMAC key the process will use:
// the command-line flag when set, else the environment variable. When neither
// is set it returns an error and the process must NOT start.
//
// It is a function of its two inputs precisely so it can be tested — the block
// it replaced lived inside Main() and read package-level flag pointers, so no
// test could reach it.
func resolveGlobalHMACKey(flagValue, envValue string) (string, hmacKeySource, error) {
	if flagValue != "" {
		log.Info().Str("source", hmacKeyFromFlag.String()).Msg("Global HMAC key configured")
		return flagValue, hmacKeyFromFlag, nil
	}
	if envValue != "" {
		log.Info().Str("source", hmacKeyFromEnv.String()).Msg("Global HMAC key configured")
		return envValue, hmacKeyFromEnv, nil
	}

	return "", hmacKeyFromEnv, fmt.Errorf(
		"%s is not set and is never generated: a key generated at startup would not be verifiable "+
			"by webhook consumers (the value is never logged and changes on every restart), "+
			"so signing with it is security theater — the recipient has no way to check the signature: "+
			"set %s (or %s) to a key the webhook consumer also knows",
		envGlobalHMACKey, envGlobalHMACKey, flagGlobalHMACKey)
}
