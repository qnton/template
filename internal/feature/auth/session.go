package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"
)

const (
	sessionCookieDev  = "session"
	sessionCookieProd = "__Host-session" // __Host- requires Secure + Path=/ + no Domain
	sessionTokenLen   = 32
	sessionTTL        = 7 * 24 * time.Hour
)

func sessionCookieName(prod bool) string {
	if prod {
		return sessionCookieProd
	}
	return sessionCookieDev
}

// newSessionToken returns a fresh opaque token (for the cookie) and its SHA-256
// (for the database). The raw token is never stored server-side, so a DB read
// cannot reconstruct a live session cookie.
func newSessionToken() (token string, hash []byte) {
	b := make([]byte, sessionTokenLen)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token)
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// setSessionCookie writes the hardened session cookie, mirroring csrf.go: the
// __Host- prefix and Secure flag in production, HttpOnly and SameSite=Lax always.
func setSessionCookie(w http.ResponseWriter, prod bool, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName(prod),
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   prod,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, prod bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName(prod),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   prod,
		SameSite: http.SameSiteLaxMode,
	})
}
