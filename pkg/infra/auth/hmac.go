// Package auth provides HMAC signing and AES-GCM encryption utilities
// extracted from root helpers.go (Phase 12c).
package auth

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

// GenerateHmacSignature creates an HMAC-SHA256 signature for a payload,
// using the encrypted HMAC key and encryption key for decryption.
func GenerateHmacSignature(payload, encryptedHmacKey, encryptionKey []byte) (string, error) {
	if len(encryptedHmacKey) == 0 {
		return "", nil
	}
	hmacKey, err := DecryptHMACKey(encryptedHmacKey, encryptionKey)
	if err != nil {
		log.Error().Err(err).
			Str("component", "auth.GenerateHmacSignature").
			Int("payload_bytes", len(payload)).
			Msg("assinatura HMAC nao gerada: chave hmac armazenada nao pode ser decriptada")
		return "", fmt.Errorf("failed to decrypt HMAC key: %w", err)
	}
	h := hmac.New(sha256.New, []byte(hmacKey))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// EncryptHMACKey encrypts plaintext using AES-GCM with the given key.
func EncryptHMACKey(plainText, encryptionKey string) ([]byte, error) {
	if encryptionKey == "" {
		log.Error().
			Str("component", "auth.EncryptHMACKey").
			Int("plaintext_bytes", len(plainText)).
			Msg("encriptacao de chave hmac recusada: chave de encriptacao global nao configurada")
		return nil, fmt.Errorf("encryption key not configured")
	}
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		log.Error().Err(err).
			Str("component", "auth.EncryptHMACKey").
			Int("key_bytes", len(encryptionKey)).
			Msg("encriptacao de chave hmac recusada: chave de encriptacao invalida para AES")
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Error().Err(err).
			Str("component", "auth.EncryptHMACKey").
			Int("nonce_bytes", len(nonce)).
			Msg("encriptacao de chave hmac abortada: fonte de entropia falhou ao gerar o nonce")
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(plainText), nil), nil
}

// DecryptHMACKey decrypts AES-GCM ciphertext using the given key.
func DecryptHMACKey(encryptedData, encryptionKey []byte) (string, error) {
	if len(encryptionKey) == 0 {
		log.Error().
			Str("component", "auth.DecryptHMACKey").
			Int("ciphertext_bytes", len(encryptedData)).
			Msg("decriptacao de chave hmac recusada: chave de encriptacao global nao configurada")
		return "", fmt.Errorf("encryption key not configured")
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		log.Error().Err(err).
			Str("component", "auth.DecryptHMACKey").
			Int("key_bytes", len(encryptionKey)).
			Msg("decriptacao de chave hmac recusada: chave de encriptacao invalida para AES")
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		log.Warn().
			Str("component", "auth.DecryptHMACKey").
			Int("ciphertext_bytes", len(encryptedData)).
			Int("nonce_bytes", nonceSize).
			Msg("decriptacao de chave hmac recusada: ciphertext menor que o nonce")
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := encryptedData[:nonceSize], encryptedData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		log.Warn().Err(err).
			Str("component", "auth.DecryptHMACKey").
			Int("ciphertext_bytes", len(encryptedData)).
			Msg("decriptacao de chave hmac falhou: tag GCM invalida (chave errada ou dado adulterado)")
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	return string(plaintext), nil
}
