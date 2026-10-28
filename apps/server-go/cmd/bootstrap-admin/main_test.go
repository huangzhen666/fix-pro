package main

import "testing"

func TestInitialPassword(t *testing.T) {
	for _, tc := range []struct {
		name, explicit, bootstrap, configured, want string
	}{
		{name: "cloud default", want: "12345678"},
		{name: "application configuration", configured: "Configured123", want: "Configured123"},
		{name: "bootstrap environment overrides application", bootstrap: "Bootstrap123", configured: "Configured123", want: "Bootstrap123"},
		{name: "flag overrides environment", explicit: " Explicit123 ", bootstrap: "Bootstrap123", configured: "Configured123", want: "Explicit123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := initialPassword(tc.explicit, tc.bootstrap, tc.configured); got != tc.want {
				t.Fatal("incorrect initial password selection")
			}
		})
	}
}
