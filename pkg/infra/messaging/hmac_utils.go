package messaging

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/rs/zerolog/log"
)

// newGCM e' o seam sobre cipher.NewGCM. Com um bloco AES ela nunca falha na
// pratica (o bloco tem sempre 16 bytes), o que deixaria os dois `if err != nil`
// abaixo permanentemente descobertos — a indirecao existe para que o teste
// possa exercitar esses caminhos sem que eles virem codigo morto por decreto.
var newGCM = cipher.NewGCM

// GenerateHmacSignature generates HMAC-SHA256 signature for webhook payload.
func GenerateHmacSignature(payload []byte, encryptedHmacKey []byte, encryptionKey *string) (string, error) {
	if len(encryptedHmacKey) == 0 {
		return "", nil
	}

	// Decrypt HMAC key
	hmacKey, err := DecryptHMACKey(encryptedHmacKey, encryptionKey)
	if err != nil {
		log.Error().Err(err).Int("payload_bytes", len(payload)).Int("encrypted_key_bytes", len(encryptedHmacKey)).Msg("failed to decrypt HMAC key while signing webhook payload")
		return "", fmt.Errorf("failed to decrypt HMAC key: %w", err)
	}

	// Generate HMAC
	h := hmac.New(sha256.New, []byte(hmacKey))
	h.Write(payload)

	return hex.EncodeToString(h.Sum(nil)), nil
}

// EncryptHMACKey encrypts a plaintext HMAC key using AES-GCM.
func EncryptHMACKey(plainText string, encryptionKey *string) ([]byte, error) {
	if encryptionKey == nil || *encryptionKey == "" {
		log.Error().Str("operation", "encrypt_hmac_key").Int("plaintext_bytes", len(plainText)).Msg("encryption key not configured")
		return nil, fmt.Errorf("encryption key not configured")
	}

	block, err := aes.NewCipher([]byte(*encryptionKey))
	if err != nil {
		log.Error().Err(err).Str("operation", "encrypt_hmac_key").Int("key_bytes", len(*encryptionKey)).Msg("failed to create AES cipher")
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := newGCM(block)
	if err != nil {
		log.Error().Err(err).Str("operation", "encrypt_hmac_key").Msg("failed to create GCM")
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Error().Err(err).Str("operation", "encrypt_hmac_key").Int("nonce_bytes", len(nonce)).Msg("failed to generate nonce")
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return ciphertext, nil
}

// DecryptHMACKey decrypts HMAC key using AES-GCM with the provided encryption key.
func DecryptHMACKey(encryptedData []byte, encryptionKey *string) (string, error) {
	if encryptionKey == nil || *encryptionKey == "" {
		log.Error().Str("operation", "decrypt_hmac_key").Int("ciphertext_bytes", len(encryptedData)).Msg("encryption key not configured")
		return "", fmt.Errorf("encryption key not configured")
	}

	block, err := aes.NewCipher([]byte(*encryptionKey))
	if err != nil {
		log.Error().Err(err).Str("operation", "decrypt_hmac_key").Int("key_bytes", len(*encryptionKey)).Msg("failed to create AES cipher")
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := newGCM(block)
	if err != nil {
		log.Error().Err(err).Str("operation", "decrypt_hmac_key").Msg("failed to create GCM")
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		log.Error().Str("operation", "decrypt_hmac_key").Int("ciphertext_bytes", len(encryptedData)).Int("nonce_bytes", nonceSize).Msg("ciphertext too short to contain a nonce")
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		log.Error().Err(err).Str("operation", "decrypt_hmac_key").Int("ciphertext_bytes", len(ciphertext)).Msg("failed to decrypt HMAC key")
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
