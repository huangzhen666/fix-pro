package auth

import "testing"

func TestWorkerPasswordPolicy(t *testing.T) {
	for _, value := range []string{"FixPro@20260816", "abc123456789"} {
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
