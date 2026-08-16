package authorization

import "testing"

func TestDomainIsTenantScoped(t *testing.T) {
	if got := Domain(12); got != "org::12" {
		t.Fatalf("domain=%q", got)
	}
	if Domain(12) == Domain(13) {
		t.Fatal("different tenants must have different domains")
	}
}
