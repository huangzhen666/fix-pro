package auth

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("CorrectHorse!123")
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) < 9 || hash[:9] != "argon2id$" {
		t.Fatalf("hash must use Argon2id: %q", hash)
	}
	if NeedsPasswordRehash(hash) {
		t.Fatal("current Argon2id parameters should not require rehash")
	}
	if !VerifyPassword("CorrectHorse!123", hash) {
		t.Fatal("password should verify")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("wrong password verified")
	}
}

func TestLegacyPasswordHashCanVerifyAndRequiresRehash(t *testing.T) {
	salt := []byte("legacy-salt-1234")
	derived := pbkdf2SHA256([]byte("LegacyPassword!123"), salt, 150000, 32)
	legacy := fmt.Sprintf("pbkdf2_sha256$150000$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived))
	if !VerifyPassword("LegacyPassword!123", legacy) {
		t.Fatal("legacy password should still verify during migration")
	}
	if VerifyPassword("wrong-password", legacy) {
		t.Fatal("wrong legacy password verified")
	}
	if !NeedsPasswordRehash(legacy) {
		t.Fatal("legacy hash must require rehash")
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password should be rejected")
	}
}
