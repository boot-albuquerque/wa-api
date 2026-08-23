package bootstrap

import (
	cryptorand "crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// Resolution of the two startup secrets that used to be generated with
// math/rand and echoed in clear text on a log line — the admin token and the
// global encryption key. See F169 in HOUSEKEEP.md.
//
// They share a defect but NOT a remedy, and the difference is the whole point
// of this file:
//
//   - The ADMIN TOKEN is recoverable. Losing it costs a restart with a new one;
//     nothing stored becomes unreadable. So it is still generated when absent —
//     refusing to start would be cost without benefit — but the generated value
//     goes to a file with mode 0600 and the log carries only the PATH.
//
//   - The GLOBAL ENCRYPTION KEY is not recoverable. It wraps every stored HMAC
//     key (auth.EncryptHMACKey) and, since ADR-0009, every stored S3 secret. A
//     key generated at startup is a different key on the next startup, and
//     ADR-0009 requires a secret that fails to decrypt to be treated as INVALID
//     and reconfigured. Auto-generation is therefore not a convenience here: it
//     is a data-loss generator that silently invalidates every tenant's
//     credentials on restart. The process REFUSES to start without it.
//
// Merely silencing the old log line would have made that worse, not better: the
// shouted "SAVE THIS KEY TO YOUR .ENV FILE OR ALL ENCRYPTED DATA WILL BE LOST
// ON RESTART!" was the only mitigation the auto-generating design had. Removing
// the mitigation while keeping the design is the wrong half to remove.
//
// Both generated values come from crypto/rand, not math/rand. math/rand is not
// a CSPRNG: its output stream is predictable from observed outputs, which is
// the wrong property for a credential. (This is NOT the classic fixed-seed
// case — since Go 1.20 the top-level math/rand functions are auto-seeded. The
// defect is the generator's class, not its seed.) math/rand remains correct in
// dispatch_retry.go, where it draws backoff jitter: jitter is not a secret.
const (
	// envGlobalEncryptionKey is the environment variable that carries the
	// operator-supplied global encryption key. It is REQUIRED.
	envGlobalEncryptionKey = "WA_API_GLOBAL_ENCRYPTION_KEY"

	// flagGlobalEncryptionKey is the command-line equivalent, named here so the
	// startup error can point at both channels without repeating a literal.
	flagGlobalEncryptionKey = "-globalencryptionkey"

	// envAdminToken is the environment variable that carries an
	// operator-supplied admin token.
	envAdminToken = "WA_API_ADMIN_TOKEN"

	// adminTokenFileName is the file a GENERATED admin token is written to,
	// inside the data directory. It exists so the operator has a channel to
	// read the token from that is not the log: a log line is collected,
	// forwarded to an aggregator and retained, and whoever reads it there gets
	// administrative access to the whole API.
	adminTokenFileName = "admin_token"

	// adminTokenFileMode is the permission the token file is created with:
	// owner read/write only. It is a single constant because both the creation
	// and the test that asserts the mode read it — two literals would be free
	// to drift apart.
	adminTokenFileMode os.FileMode = 0o600

	// generatedAdminTokenLength is the length, in characters, of a generated
	// admin token. It matches what the previous math/rand block produced, so
	// nothing downstream sees a format change.
	generatedAdminTokenLength = 32
)

// secretCharset is the alphabet a generated secret is drawn from. It is defined
// by reference to hmacKeyCharset (global_hmac_key.go) rather than repeated, so
// the two alphabets cannot drift: alphanumeric, so the value survives every
// transport an operator may paste it through without quoting.
const secretCharset = hmacKeyCharset

// adminTokenSource records where a resolved admin token came from, so a caller
// (and a test) can tell "the operator configured one" from "we generated one"
// without inspecting the token itself.
type adminTokenSource int

const (
	adminTokenFromFlag adminTokenSource = iota
	adminTokenFromEnv
	adminTokenGenerated
)

// String makes the source safe to put in a log field: it describes the token
// without revealing any part of it.
func (s adminTokenSource) String() string {
	switch s {
	case adminTokenFromFlag:
		return "command_line"
	case adminTokenFromEnv:
		return "environment"
	case adminTokenGenerated:
		return "generated"
	default:
		return "unknown"
	}
}

// encryptionKeySource records where the global encryption key came from. There
// is deliberately no "generated" member: generating one is the defect this file
// removes, and leaving the member around would invite it back.
type encryptionKeySource int

const (
	encryptionKeyFromFlag encryptionKeySource = iota
	encryptionKeyFromEnv
)

// String makes the source safe to put in a log field.
func (s encryptionKeySource) String() string {
	switch s {
	case encryptionKeyFromFlag:
		return "command_line"
	case encryptionKeyFromEnv:
		return "environment"
	default:
		return "unknown"
	}
}

// generateSecret draws `length` characters from crypto/rand.
//
// The draw uses rejection sampling rather than `b % len(charset)`: 256 is not a
// multiple of the 62-character alphabet, so a plain modulo would make the first
// 256%62 == 8 letters measurably likelier than the rest. Bytes at or above the
// largest multiple of the alphabet size that fits in a byte are discarded.
//
// This repeats the body of generateGlobalHMACKey (global_hmac_key.go) instead
// of sharing it, and that is deliberate: F156's structural test asserts that
// crypto/rand is used INSIDE that file, and a one-line delegation would remove
// the import and disarm the assertion that keeps math/rand out of it.
func generateSecret(length int) (string, error) {
	limit := byte(len(secretCharset) * (256 / len(secretCharset)))

	out := make([]byte, 0, length)
	buf := make([]byte, length)
	for len(out) < length {
		if _, err := cryptorand.Read(buf); err != nil {
			// Logged here AND propagated: the caller turns it into a Fatal,
			// and this line says WHICH startup step failed. No secret exists
			// yet, so there is nothing to leak.
			log.Error().Err(err).Int("requested_bytes", len(buf)).
				Msg("startup secret not generated: the entropy source failed")
			return "", fmt.Errorf("draw random bytes for a startup secret: %w", err)
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out = append(out, secretCharset[int(b)%len(secretCharset)])
			if len(out) == length {
				break
			}
		}
	}
	return string(out), nil
}

// resolveGlobalEncryptionKey decides which global encryption key the process
// will use: the command-line flag when set, else the environment variable. When
// neither is set it returns an error and the process must NOT start.
//
// The error names the exact variable that is missing, following the shape
// validateStackForMode already uses (cluster.go): say what was wrong, name the
// variable, and say why the degradation is not acceptable.
func resolveGlobalEncryptionKey(flagValue, envValue string) (string, encryptionKeySource, error) {
	if flagValue != "" {
		log.Info().Str("source", encryptionKeyFromFlag.String()).Msg("Global encryption key configured")
		return flagValue, encryptionKeyFromFlag, nil
	}
	if envValue != "" {
		log.Info().Str("source", encryptionKeyFromEnv.String()).Msg("Global encryption key configured")
		return envValue, encryptionKeyFromEnv, nil
	}
	return "", encryptionKeyFromEnv, fmt.Errorf(
		"%s is not set and is never generated: a key generated at startup would be a DIFFERENT key on the next startup, "+
			"and every HMAC key and S3 secret already stored would stop decrypting (ADR-0009 treats those as invalid and demands reconfiguration): "+
			"set %s (or %s) to a value you keep across restarts",
		envGlobalEncryptionKey, envGlobalEncryptionKey, flagGlobalEncryptionKey)
}

// writeAdminTokenFile writes a generated admin token into dir, with mode
// adminTokenFileMode, and returns the path.
//
// Any previous file is REMOVED before the new one is created rather than
// truncated: O_TRUNC on an existing path keeps that path's existing permission
// bits, so a file left behind as 0644 by an older build would silently stay
// world-readable. Creating fresh is the only way the mode constant is the sole
// authority over the result.
func writeAdminTokenFile(dir, token string) (string, error) {
	path := filepath.Join(dir, adminTokenFileName)

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		// Every failure on this path is logged with the PATH and propagated.
		// The path is what the operator has to act on (a read-only data
		// directory, a leftover file owned by another user), and it is not a
		// secret — the token itself never appears on any of these lines.
		log.Error().Err(err).Str("admin_token_file", path).
			Msg("admin token file not replaced: the previous file could not be removed")
		return "", fmt.Errorf("remove the previous admin token file %s: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, adminTokenFileMode)
	if err != nil {
		log.Error().Err(err).Str("admin_token_file", path).
			Msg("admin token file not created")
		return "", fmt.Errorf("create the admin token file %s: %w", path, err)
	}
	if _, err := f.WriteString(token); err != nil {
		// The handle is closed before returning: leaving it open on the error
		// path would leak a descriptor on every failed startup attempt.
		_ = f.Close()
		log.Error().Err(err).Str("admin_token_file", path).
			Msg("admin token file not written")
		return "", fmt.Errorf("write the admin token file %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		log.Error().Err(err).Str("admin_token_file", path).
			Msg("admin token file not closed: its contents may be incomplete")
		return "", fmt.Errorf("close the admin token file %s: %w", path, err)
	}
	return path, nil
}

// resolveAdminToken decides which admin token the process will use: the
// command-line flag when set, else the environment variable, else a freshly
// generated one written to a file under dir. It preserves the precedence the
// inline block in Main() had.
//
// It is a function of its inputs precisely so it can be tested — the block it
// replaced lived inside Main() and read package-level flag pointers, so no test
// could reach it.
func resolveAdminToken(flagValue, envValue, dir string) (string, adminTokenSource, error) {
	if flagValue != "" {
		log.Info().Str("source", adminTokenFromFlag.String()).Msg("Admin token configured")
		return flagValue, adminTokenFromFlag, nil
	}
	if envValue != "" {
		log.Info().Str("source", adminTokenFromEnv.String()).Msg("Admin token configured")
		return envValue, adminTokenFromEnv, nil
	}

	token, err := generateSecret(generatedAdminTokenLength)
	if err != nil {
		return "", adminTokenGenerated, err
	}
	path, err := writeAdminTokenFile(dir, token)
	if err != nil {
		return "", adminTokenGenerated, err
	}
	// The PATH is logged, never the token, and nothing that would narrow the
	// token down (prefix, hash) either. The operator reads the value from the
	// file, which only the owner can open.
	log.Warn().
		Str("source", adminTokenGenerated.String()).
		Str("admin_token_file", path).
		Msg("No " + envAdminToken + " provided, generated a random one. " +
			"Its value is not logged; read it from the file named in admin_token_file, " +
			"or set " + envAdminToken + " to a token of your own. " +
			"A generated token changes on every restart")
	return token, adminTokenGenerated, nil
}
