package httpx

import (
	"encoding/base64"
	"testing"
)

// FuzzUnmaskToken asserts the CSRF token parser never panics on arbitrary input
// and only ever returns nil or a real token of the expected length.
func FuzzUnmaskToken(f *testing.F) {
	f.Add("")
	f.Add("not-base64-@@@")
	f.Add(base64.RawURLEncoding.EncodeToString(make([]byte, 2*csrfTokenLen))) // correct length
	f.Add(base64.RawURLEncoding.EncodeToString(make([]byte, csrfTokenLen)))   // half length
	f.Add(base64.RawURLEncoding.EncodeToString(make([]byte, 10)))             // wrong length

	f.Fuzz(func(t *testing.T, s string) {
		out := unmaskToken(s) // must never panic
		if out != nil && len(out) != csrfTokenLen {
			t.Fatalf("unmaskToken returned a non-nil slice of len %d, want %d", len(out), csrfTokenLen)
		}
	})
}
