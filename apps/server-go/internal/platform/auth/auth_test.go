package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomerTokenOnlyWorksLocally(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, tc := range []struct {
		env  string
		want int
	}{{"local", 204}, {"production", 401}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer local-customer-1")
		w := httptest.NewRecorder()
		Customer(tc.env, next).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("env=%s status=%d want=%d", tc.env, w.Code, tc.want)
		}
	}
}

func TestAdminBasicAuth(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	Admin("admin", "secret", next).ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
}
