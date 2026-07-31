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
)

// GenerateHmacSignature creates an HMAC-SHA256 signature for a payload,
// using the encrypted HMAC key and encryption key for decryption.
func GenerateHmacSignature(payload, encryptedHmacKey, encryptionKey []byte) (string, error) {
	if len(encryptedHmacKey) == 0 {
		return "", nil
	}
	hmacKey, err := DecryptHMACKey(encryptedHmacKey, encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt HMAC key: %w", err)
	}
	h := hmac.New(sha256.New, []byte(hmacKey))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// EncryptHMACKey encrypts plaintext using AES-GCM with the given key.
func EncryptHMACKey(plainText, encryptionKey string) ([]byte, error) {
	if encryptionKey == "" {
		return nil, fmt.Errorf("encryption key not configured")
	}
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(plainText), nil), nil
}

// DecryptHMACKey decrypts AES-GCM ciphertext using the given key.
func DecryptHMACKey(encryptedData, encryptionKey []byte) (string, error) {
	if len(encryptionKey) == 0 {
		return "", fmt.Errorf("encryption key not configured")
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := encryptedData[:nonceSize], encryptedData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	return string(plaintext), nil
}
