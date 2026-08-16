package workforce

import "testing"

func TestValidWorkerMobile(t *testing.T) {
	for _, value := range []string{"13800138000", "15912345678"} {
		if !validWorkerMobile(value) {
			t.Fatalf("mobile %q should be valid", value)
		}
	}
	for _, value := range []string{"1380013800", "23800138000", "1380013800a"} {
		if validWorkerMobile(value) {
			t.Fatalf("mobile %q should be invalid", value)
		}
	}
}
