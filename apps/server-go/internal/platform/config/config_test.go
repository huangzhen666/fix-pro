package config

import "testing"

func TestProductionGuards(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_ADMIN_PASSWORD", "change-me-in-production")
	if _, err := Load(); err == nil {
		t.Fatal("default production password must fail")
	}
	t.Setenv("APP_ADMIN_PASSWORD", "a-real-secret")
	t.Setenv("MEDIA_DRIVER", "local")
	if _, err := Load(); err == nil {
		t.Fatal("production local media must fail")
	}
	t.Setenv("MEDIA_DRIVER", "s3")
	t.Setenv("DB_DSN", "postgres://fixpro:secret@db/fix_pro?sslmode=disable")
	if _, err := Load(); err == nil {
		t.Fatal("production database without TLS must fail")
	}
}
