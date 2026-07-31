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
)

// GenerateHmacSignature generates HMAC-SHA256 signature for webhook payload.
func GenerateHmacSignature(payload []byte, encryptedHmacKey []byte, encryptionKey *string) (string, error) {
	if len(encryptedHmacKey) == 0 {
		return "", nil
	}

	// Decrypt HMAC key
	hmacKey, err := DecryptHMACKey(encryptedHmacKey, encryptionKey)
	if err != nil {
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
		return nil, fmt.Errorf("encryption key not configured")
	}

	block, err := aes.NewCipher([]byte(*encryptionKey))
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

	ciphertext := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return ciphertext, nil
}

// DecryptHMACKey decrypts HMAC key using AES-GCM with the provided encryption key.
func DecryptHMACKey(encryptedData []byte, encryptionKey *string) (string, error) {
	if encryptionKey == nil || *encryptionKey == "" {
		return "", fmt.Errorf("encryption key not configured")
	}

	block, err := aes.NewCipher([]byte(*encryptionKey))
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

	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
