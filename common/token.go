package common

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// TokenEqual compares two tokens using SHA-256 digests and constant-time
// comparison, avoiding timing attacks and length side-channels.
func TokenEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// MaskToken redacts a token for logging/display: sk-ab...xyz
func MaskToken(t string) string {
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}

// RandomToken generates a URL-safe-ish random token with the given prefix.
func RandomToken(prefix string) string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return prefix + "fallback"
	}
	return prefix + hex.EncodeToString(b)
}
