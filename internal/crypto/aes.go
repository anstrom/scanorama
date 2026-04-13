// Package crypto provides AES-256-GCM encryption for sensitive stored values.
// The encryption key is loaded from the SCANORAMA_SECRET_KEY environment
// variable (exactly 64 hex characters = 32 bytes). If the variable is absent,
// a random ephemeral key is generated and a warning is logged — credentials
// encrypted with an ephemeral key are lost on restart, so set the variable in
// production.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

const (
	keyEnvVar = "SCANORAMA_SECRET_KEY"
	keyBytes  = 32 // AES-256
)

var (
	globalKey  []byte
	globalOnce sync.Once
)

// key returns the encryption key, initializing it on first call.
func key() []byte {
	globalOnce.Do(func() {
		raw := os.Getenv(keyEnvVar)
		if raw != "" {
			decoded, err := hex.DecodeString(raw)
			if err == nil && len(decoded) == keyBytes {
				globalKey = decoded
				return
			}
			slog.Warn("crypto: SCANORAMA_SECRET_KEY is set but invalid (must be 64 hex chars); using ephemeral key")
		} else {
			slog.Warn("crypto: SCANORAMA_SECRET_KEY not set; using ephemeral key — credentials will be lost on restart")
		}

		ephemeral := make([]byte, keyBytes)
		if _, err := io.ReadFull(rand.Reader, ephemeral); err != nil {
			panic(fmt.Sprintf("crypto: failed to generate ephemeral key: %v", err))
		}
		globalKey = ephemeral
	})
	return globalKey
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a hex-encoded
// "nonce||ciphertext" string safe to store in a TEXT column.
// Returns "" for an empty plaintext (no-op, avoids storing encrypted empty strings).
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key())
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: rand nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a hex-encoded "nonce||ciphertext" string produced by Encrypt.
// Returns "" for an empty input.
func Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}

	data, err := hex.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("crypto: decode hex: %w", err)
	}

	block, err := aes.NewCipher(key())
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("crypto: ciphertext too short")
	}

	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt: %w", err)
	}

	return string(plaintext), nil
}
