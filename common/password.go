package common

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"hash"
	"strconv"
	"strings"
)

// Password hashing (admin UI login). Passwords are never stored in plaintext:
// only a PBKDF2-HMAC-SHA256 digest with a random per-account salt is persisted.
//
// The encoded form is a self-describing string so parameters can change over
// time without breaking existing hashes:
//
//	pbkdf2-sha256$<iterations>$<salt-b64>$<digest-b64>
//
// This is implemented on top of crypto/hmac + crypto/sha256 to stay compatible
// with the module's `go 1.22` directive (crypto/pbkdf2 landed later).

const (
	// pbkdf2Iterations is the default work factor. Tuned to stay well under
	// 100ms on commodity hardware while resisting offline brute force.
	pbkdf2Iterations = 210000
	pbkdf2SaltLen    = 16
	pbkdf2KeyLen     = 32
	pbkdf2Prefix     = "pbkdf2-sha256"
)

// pbkdf2 implements PBKDF2 (RFC 2898) over a hash constructor, using HMAC.
func pbkdf2(password, salt []byte, iter, keyLen int, h func() hash.Hash) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	u := make([]byte, hashLen)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		prf.Write(buf[:4])
		dk = prf.Sum(dk)
		t := dk[len(dk)-hashLen:]
		copy(u, t)

		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = u[:0]
			u = prf.Sum(u)
			for x := range u {
				t[x] ^= u[x]
			}
		}
	}
	return dk[:keyLen]
}

// HashPassword derives a storable PBKDF2 digest from a plaintext password.
// The returned string embeds the salt and iteration count.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password must not be empty")
	}
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLen, sha256.New)
	return strings.Join([]string{
		pbkdf2Prefix,
		strconv.Itoa(pbkdf2Iterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk),
	}, "$"), nil
}

// VerifyPassword reports whether password matches the encoded PBKDF2 digest.
// Comparison is constant time. Malformed encodings fail closed (return false).
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != pbkdf2Prefix {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2([]byte(password), salt, iter, len(want), sha256.New)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// IsPasswordHash reports whether s looks like an encoded PBKDF2 digest
// (used to distinguish a real hash from a masked/empty value).
func IsPasswordHash(s string) bool {
	parts := strings.Split(s, "$")
	return len(parts) == 4 && parts[0] == pbkdf2Prefix && parts[1] != "" &&
		parts[2] != "" && parts[3] != ""
}

// RandomPassword returns a URL-safe random password (base64 of 18 bytes).
// liapi never auto-generates credentials; this is a standalone utility (e.g.
// for scripts that provision a config).
func RandomPassword() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "changeme-" + hex.EncodeToString([]byte{1, 2, 3, 4})
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
