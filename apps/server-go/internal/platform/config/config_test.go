package config

import "testing"

func TestDefaultAdminCredentials(t *testing.T) {
	t.Setenv("APP_ENV", "local")
	t.Setenv("APP_ADMIN_USERNAME", "")
	t.Setenv("APP_ADMIN_PASSWORD", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminUsername != "admin" || c.AdminPassword != "12345678" {
		t.Fatal("unexpected default administrator credentials")
	}
}

func TestProductionAllowsDefaultBootstrapPassword(t *testing.T) {
	for key, value := range map[string]string{
		"APP_ENV": "production", "APP_ADMIN_PASSWORD": "", "MEDIA_DRIVER": "s3",
		"DB_DSN":             "postgres://test@db/fix_pro?sslmode=require",
		"WECHAT_MINI_APP_ID": "test-app", "WECHAT_MINI_APP_SECRET": "test-secret",
		"CUSTOMER_MOBILE_ENCRYPTION_KEY": "test-encryption", "CUSTOMER_MOBILE_HASH_KEY": "test-hash",
		"APP_ADMIN_BASIC_COMPAT": "false", "APP_ADMIN_COOKIE_SECURE": "true", "WORKER_DEV_TOKEN_ENABLED": "false",
		"APP_VERSION": "bootstrap-test", "GIT_COMMIT": "bootstrap-test", "LOG_INSTANCE_ID": "bootstrap-test", "LOG_FORMAT": "json",
	} {
		t.Setenv(key, value)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminPassword != DefaultAdminPassword {
		t.Fatal("production should support the default initial password")
	}
}

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
