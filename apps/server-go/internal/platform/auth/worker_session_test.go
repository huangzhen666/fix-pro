package auth

import (
	"context"
	"errors"
	"testing"
)

func TestWorkerPasswordPolicy(t *testing.T) {
	for _, value := range []string{"Abc12345", "FixPro@20260816", "abc123456789"} {
		if !hasLetterAndDigit(value) {
			t.Fatalf("password %q should contain a letter and a digit", value)
		}
	}
	for _, value := range []string{"123456789012", "abcdefghijklm"} {
		if hasLetterAndDigit(value) {
			t.Fatalf("password %q should fail the letter/digit policy", value)
		}
	}
}

func TestWorkerChangePasswordRejectsWeakPasswords(t *testing.T) {
	store := &WorkerSessionStore{}
	for _, password := range []string{"Abc1234", "12345678", "abcdefgh"} {
		err := store.ChangePassword(context.Background(), Principal{}, "", "OldPassword1", password)
		if !errors.Is(err, ErrWorkerPasswordTooWeak) {
			t.Fatalf("weak password should be rejected before accessing the database: %v", err)
		}
	}
}
