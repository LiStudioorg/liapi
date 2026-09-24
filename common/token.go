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

// HashToken returns the lowercase hex SHA-256 of a token. Only this form is
// persisted for device tokens.
func HashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// HashEqual compares a raw token against a stored hex hash in constant time.
func HashEqual(raw, hexHash string) bool {
	sum := sha256.Sum256([]byte(raw))
	got := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(hexHash)) == 1
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

// RequestID returns a short random hex id for log correlation.
func RequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}
