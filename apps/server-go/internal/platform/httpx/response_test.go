package httpx

import (
	"errors"
	"net/http/httptest"
	"testing"
)

func TestFailureHidesInternalError(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	Failure(w, r, errors.New("secret database error"))
	if w.Code != 500 {
		t.Fatalf("status=%d", w.Code)
	}
	if got := w.Body.String(); got == "" || contains(got, "secret") {
		t.Fatalf("unsafe response: %s", got)
	}
}
func contains(s, v string) bool {
	for i := 0; i+len(v) <= len(s); i++ {
		if s[i:i+len(v)] == v {
			return true
		}
	}
	return false
}
