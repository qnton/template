package auth

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := hashPassword("s3cr3t-passphrase")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if !verifyPassword(hash, "s3cr3t-passphrase") {
		t.Error("correct password failed to verify")
	}
	if verifyPassword(hash, "wrong-password") {
		t.Error("wrong password verified")
	}
	for _, bad := range []string{"", "garbage", "pbkdf2-sha256$x$y$z", "a$b$c$d$e"} {
		if verifyPassword(bad, "s3cr3t-passphrase") {
			t.Errorf("malformed hash %q must not verify", bad)
		}
	}
}
