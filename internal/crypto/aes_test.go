// Package crypto — unit tests for AES-256-GCM encrypt/decrypt.
package crypto

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetKey resets the package-level singleton so tests that set different env
// vars can exercise the initialisation logic independently.
func resetKey() {
	globalKey = nil
	globalOnce = sync.Once{}
}

// testKey is a fixed 32-byte key encoded as 64 hex chars for deterministic tests.
const testKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// ── Encrypt / Decrypt round-trip ─────────────────────────────────────────────

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	t.Setenv(keyEnvVar, testKey)
	resetKey()
	t.Cleanup(resetKey)

	cases := []string{
		"public",
		"s3cr3t!community",
		"v3-auth-password-with-special-chars-åøæ",
		strings.Repeat("x", 256),
	}
	for _, plain := range cases {
		enc, err := Encrypt(plain)
		require.NoError(t, err, "Encrypt(%q)", plain)
		assert.NotEmpty(t, enc)
		assert.NotEqual(t, plain, enc, "ciphertext must differ from plaintext")

		got, err := Decrypt(enc)
		require.NoError(t, err, "Decrypt(Encrypt(%q))", plain)
		assert.Equal(t, plain, got)
	}
}

func TestEncrypt_EmptyString(t *testing.T) {
	t.Setenv(keyEnvVar, testKey)
	resetKey()
	t.Cleanup(resetKey)

	enc, err := Encrypt("")
	require.NoError(t, err)
	assert.Empty(t, enc, "Encrypt of empty string must return empty string")
}

func TestDecrypt_EmptyString(t *testing.T) {
	t.Setenv(keyEnvVar, testKey)
	resetKey()
	t.Cleanup(resetKey)

	got, err := Decrypt("")
	require.NoError(t, err)
	assert.Empty(t, got, "Decrypt of empty string must return empty string")
}

// ── Decrypt error paths ───────────────────────────────────────────────────────

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	t.Setenv(keyEnvVar, testKey)
	resetKey()
	t.Cleanup(resetKey)

	enc, err := Encrypt("original")
	require.NoError(t, err)

	// Flip the last hex byte to corrupt the GCM authentication tag.
	b, err := hex.DecodeString(enc)
	require.NoError(t, err)
	b[len(b)-1] ^= 0xff
	tampered := hex.EncodeToString(b)

	_, err = Decrypt(tampered)
	assert.Error(t, err, "tampered ciphertext must not decrypt successfully")
}

func TestDecrypt_InvalidHex(t *testing.T) {
	t.Setenv(keyEnvVar, testKey)
	resetKey()
	t.Cleanup(resetKey)

	_, err := Decrypt("not-valid-hex!!!")
	assert.Error(t, err)
}

func TestDecrypt_TooShort(t *testing.T) {
	t.Setenv(keyEnvVar, testKey)
	resetKey()
	t.Cleanup(resetKey)

	// Encode fewer bytes than the GCM nonce size (12 bytes).
	short := hex.EncodeToString([]byte{0x01, 0x02, 0x03})
	_, err := Decrypt(short)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

// ── Nonces are unique ─────────────────────────────────────────────────────────

func TestEncrypt_UniqueNonces(t *testing.T) {
	t.Setenv(keyEnvVar, testKey)
	resetKey()
	t.Cleanup(resetKey)

	enc1, err := Encrypt("same-input")
	require.NoError(t, err)
	enc2, err := Encrypt("same-input")
	require.NoError(t, err)

	assert.NotEqual(t, enc1, enc2, "each encryption must use a fresh random nonce")
}
