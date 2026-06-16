package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	pbkdf2Iter    = 600_000 // OWASP 2023 floor for PBKDF2-HMAC-SHA256
	pbkdf2SaltLen = 16
	pbkdf2KeyLen  = 32
)

// hashPassword returns an encoded PBKDF2-HMAC-SHA256 hash of the form
// "pbkdf2-sha256$<iter>$<b64salt>$<b64key>". The cost is stored per hash so it
// can be raised later without invalidating existing rows.
//
// PBKDF2 keeps the template dependency-free (crypto/pbkdf2 is stdlib since Go
// 1.24). For stronger resistance to GPU/ASIC cracking, swap the body for
// golang.org/x/crypto/argon2 (argon2.IDKey) — that adds one runtime dependency.
func hashPassword(plain string) (string, error) {
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, plain, salt, pbkdf2Iter, pbkdf2KeyLen)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}
	enc := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, enc(salt), enc(key)), nil
}

// verifyPassword reports whether plain matches the encoded hash, comparing in
// constant time. It returns false (never an error) on a malformed encoding so
// callers can return a single uniform "invalid credentials" response.
func verifyPassword(encoded, plain string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
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
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, plain, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
