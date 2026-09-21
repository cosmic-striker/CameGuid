package repository

import (
	"strings"

	"camguid/internal/pkg/crypto"
)

// encPrefix marks values stored with AES-256-GCM encryption at rest. Values
// without the marker are treated as legacy plaintext (kept for backward
// compatibility with databases created before encryption was introduced).
const encPrefix = "enc1:"

// SecretCipher encrypts/decrypts string secrets stored in the database using a
// hex-encoded AES-256 key (CAMGUID_ENCRYPTION_KEY).
type SecretCipher struct {
	hexKey string
}

func NewSecretCipher(hexKey string) *SecretCipher {
	return &SecretCipher{hexKey: hexKey}
}

// EncryptSecret returns the encrypted form of s ("" stays ""). A non-empty
// result always carries the enc1: marker so readers can recognize it.
func (c *SecretCipher) EncryptSecret(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	enc, err := crypto.Encrypt(s, c.hexKey)
	if err != nil {
		return "", err
	}
	return encPrefix + enc, nil
}

// DecryptSecret decrypts a stored value. Non-marker (legacy plaintext) values
// pass through unchanged.
func (c *SecretCipher) DecryptSecret(stored string) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	return crypto.Decrypt(strings.TrimPrefix(stored, encPrefix), c.hexKey)
}

// strOrEmpty dereferences a string pointer, returning "" for nil.
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}